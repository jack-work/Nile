package lifecycle

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gluck/nile/pkg/protocol"
	"github.com/gluck/nile/pkg/wal"
)

// mockTransport records calls and returns canned responses.
type mockTransport struct {
	mu       sync.Mutex
	calls    []string // method names
	closed   bool
	resultFn func(method string) (*protocol.StatusResult, error)
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		resultFn: func(method string) (*protocol.StatusResult, error) {
			return &protocol.StatusResult{Status: "ok"}, nil
		},
	}
}

func (m *mockTransport) Send(req *protocol.Request) (*protocol.Response, error) {
	m.mu.Lock()
	fn := m.resultFn
	m.calls = append(m.calls, req.Method)
	m.mu.Unlock()

	result, err := fn(req.Method)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(result)
	return &protocol.Response{
		JSONRPC: "2.0",
		Result:  data,
		ID:      req.ID,
	}, nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockTransport) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		from State
		to   State
		ok   bool
	}{
		{StateCreated, StateStarting, true},
		{StateCreated, StateIdle, false},
		{StateStarting, StateIdle, true},
		{StateStarting, StateFailed, true},
		{StateIdle, StateProcessing, true},
		{StateIdle, StateDraining, true},
		{StateIdle, StateStopping, true},
		{StateIdle, StateRetaining, false},
		{StateProcessing, StateIdle, true},
		{StateProcessing, StatePostProcessing, true},
		{StateDraining, StateRetaining, true},
		{StateRetaining, StateIdle, true},
		{StateStopping, StateStopped, true},
	}

	for _, tt := range tests {
		got := canTransition(tt.from, tt.to)
		if got != tt.ok {
			t.Errorf("%s -> %s: got %v, want %v", tt.from, tt.to, got, tt.ok)
		}
	}
}

func TestMessagePump(t *testing.T) {
	dir := t.TempDir()
	opts := wal.DefaultOptions()
	opts.MaxMessages = 100000

	wl, err := wal.Open(dir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer wl.Close()

	for i := 0; i < 5; i++ {
		wl.Append([]byte("test-msg"))
	}

	mt := newMockTransport()
	mgr := New(Config{
		Name:      "test",
		DataDir:   dir,
		Store:     wl,
		Transport: mt,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("pump error: %v", err)
	}

	calls := mt.getCalls()
	initCount := 0
	msgCount := 0
	shutdownCount := 0
	for _, c := range calls {
		switch c {
		case "init":
			initCount++
		case "message":
			msgCount++
		case "shutdown":
			shutdownCount++
		}
	}

	if initCount != 1 {
		t.Errorf("init calls: got %d, want 1", initCount)
	}
	if msgCount != 5 {
		t.Errorf("message calls: got %d, want 5", msgCount)
	}
	if shutdownCount != 1 {
		t.Errorf("shutdown calls: got %d, want 1", shutdownCount)
	}
}

func TestRetentionCycle(t *testing.T) {
	dir := t.TempDir()
	opts := wal.DefaultOptions()
	opts.MaxMessages = 3
	opts.SegmentSize = 1024 * 1024

	wl, err := wal.Open(dir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer wl.Close()

	for i := 0; i < 5; i++ {
		wl.Append([]byte("retain-test"))
	}

	mt := newMockTransport()
	mgr := New(Config{
		Name:      "test-retain",
		DataDir:   dir,
		Store:     wl,
		Transport: mt,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(500 * time.Millisecond)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("pump error: %v", err)
	}

	calls := mt.getCalls()
	retainCount := 0
	for _, c := range calls {
		if c == "retain" {
			retainCount++
		}
	}

	if retainCount == 0 {
		t.Error("expected at least one retain call")
	}
}

func TestPostProcessing(t *testing.T) {
	dir := t.TempDir()
	opts := wal.DefaultOptions()
	opts.MaxMessages = 100000

	wl, err := wal.Open(dir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer wl.Close()

	wl.Append([]byte("pp-test"))

	mt := newMockTransport()
	mt.resultFn = func(method string) (*protocol.StatusResult, error) {
		if method == "message" {
			return &protocol.StatusResult{Status: "ok", PostProcess: true}, nil
		}
		return &protocol.StatusResult{Status: "ok"}, nil
	}

	mgr := New(Config{
		Name:      "test-pp",
		DataDir:   dir,
		Store:     wl,
		Transport: mt,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(200 * time.Millisecond)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("pump error: %v", err)
	}

	calls := mt.getCalls()
	msgFound := false
	for _, c := range calls {
		if c == "message" {
			msgFound = true
		}
	}
	if !msgFound {
		t.Error("expected message call")
	}
}

func TestDeadLetterOnRetryExhaustion(t *testing.T) {
	dir := t.TempDir()
	opts := wal.DefaultOptions()
	opts.MaxMessages = 100000

	wl, err := wal.Open(dir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	defer wl.Close()

	wl.Append([]byte("will-fail"))
	wl.Append([]byte("will-succeed"))

	callCount := 0
	mt := newMockTransport()
	mt.resultFn = func(method string) (*protocol.StatusResult, error) {
		if method == "message" {
			mt.mu.Lock()
			c := callCount
			callCount++
			mt.mu.Unlock()
			// First message fails on all attempts (4 calls: 1 initial + 3 retries)
			if c < 4 {
				return nil, fmt.Errorf("neb error")
			}
		}
		return &protocol.StatusResult{Status: "ok"}, nil
	}

	mgr := New(Config{
		Name:       "test-deadletter",
		DataDir:    dir,
		Store:      wl,
		Transport:  mt,
		MaxRetries: 3,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(2 * time.Second)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("pump error: %v", err)
	}

	// First message should be dead-lettered
	dead, err := wl.ReadDeadLetters()
	if err != nil {
		t.Fatalf("read dead letters: %v", err)
	}
	if len(dead) != 1 {
		t.Fatalf("dead letters: got %d, want 1", len(dead))
	}
	if string(dead[0]) != "will-fail" {
		t.Errorf("dead letter: got %q, want %q", dead[0], "will-fail")
	}

	// Second message should have been processed
	calls := mt.getCalls()
	msgCount := 0
	for _, c := range calls {
		if c == "message" {
			msgCount++
		}
	}
	// 4 failed attempts + 1 successful = 5 message calls
	if msgCount < 5 {
		t.Errorf("message calls: got %d, want >= 5", msgCount)
	}
}
