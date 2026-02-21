package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gluck/nile/pkg/lifecycle"
	"github.com/gluck/nile/pkg/protocol"
	"github.com/gluck/nile/pkg/transport"
	"github.com/gluck/nile/pkg/wal"
)

// buildCounterService builds the counter-service example and returns its path.
func buildCounterService(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "counter-service")
	cmd := exec.Command("go", "build", "-o", bin, "../../examples/counter-service/")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build counter-service: %v", err)
	}
	return bin
}

func TestIntegrationMessageCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	counterBin := buildCounterService(t)
	dataDir := t.TempDir()

	stateDir := filepath.Join(dataDir, "state")
	retainDir := filepath.Join(dataDir, "retain")
	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(retainDir, 0755)

	opts := wal.Options{
		MaxMessages: 100000,
		MaxBytes:    10 * 1024 * 1024,
		SegmentSize: 1024 * 1024,
	}
	wlog, err := wal.Open(dataDir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}

	for i := 0; i < 5; i++ {
		wlog.Append([]byte(fmt.Sprintf("message-%d", i)))
	}

	cmd := exec.Command(counterBin)
	cmd.Env = []string{
		"NILE_STATE_DIR=" + stateDir,
		"NILE_RETAIN_DIR=" + retainDir,
	}
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start counter: %v", err)
	}

	tr := transport.NewStdio(stdin, stdout)
	mgr := lifecycle.New(lifecycle.Config{
		Name:      "test-counter",
		DataDir:   dataDir,
		Store:     wlog,
		Transport: tr,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(500 * time.Millisecond)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("lifecycle error: %v", err)
	}

	tr.Close()
	stdin.Close()
	cmd.Wait()
	wlog.Close()

	countData, err := os.ReadFile(filepath.Join(stateDir, "count"))
	if err != nil {
		t.Fatalf("read count: %v", err)
	}
	if string(countData) != "5" {
		t.Errorf("count: got %q, want %q", string(countData), "5")
	}
}

func TestIntegrationRetentionCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	counterBin := buildCounterService(t)
	dataDir := t.TempDir()

	stateDir := filepath.Join(dataDir, "state")
	retainDir := filepath.Join(dataDir, "retain")
	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(retainDir, 0755)

	opts := wal.Options{
		MaxMessages: 3,
		MaxBytes:    10 * 1024 * 1024,
		SegmentSize: 1024 * 1024,
	}
	wlog, err := wal.Open(dataDir, opts)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}

	for i := 0; i < 5; i++ {
		wlog.Append([]byte(fmt.Sprintf("retain-msg-%d", i)))
	}

	cmd := exec.Command(counterBin)
	cmd.Env = []string{
		"NILE_STATE_DIR=" + stateDir,
		"NILE_RETAIN_DIR=" + retainDir,
	}
	cmd.Stderr = os.Stderr

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()

	tr := transport.NewStdio(stdin, stdout)
	mgr := lifecycle.New(lifecycle.Config{
		Name:      "test-retain",
		DataDir:   dataDir,
		Store:     wlog,
		Transport: tr,
	})
	mgr.PollInterval = 10 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start()
	}()

	time.Sleep(1 * time.Second)
	mgr.Stop()

	if err := <-done; err != nil {
		t.Fatalf("lifecycle error: %v", err)
	}

	tr.Close()
	stdin.Close()
	cmd.Wait()
	wlog.Close()

	entries, err := os.ReadDir(retainDir)
	if err != nil {
		t.Fatalf("read retain dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one snapshot file in retain/")
	}

	countData, err := os.ReadFile(filepath.Join(stateDir, "count"))
	if err != nil {
		t.Fatalf("read count: %v", err)
	}
	if string(countData) != "4" {
		t.Errorf("count: got %q, want %q", string(countData), "4")
	}
}

func TestEchoServiceProtocol(t *testing.T) {
	req, err := protocol.NewRequest(1, protocol.MethodInit, protocol.InitParams{
		Name:   "test",
		Config: map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	data, err := req.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc: got %v", parsed["jsonrpc"])
	}
	if parsed["method"] != "init" {
		t.Errorf("method: got %v", parsed["method"])
	}

	req2, _ := protocol.NewRequest(2, protocol.MethodMessage, protocol.MessageParams{
		Offset: 42,
		Data:   "aGVsbG8=",
	})
	data2, _ := req2.Encode()

	var parsed2 map[string]interface{}
	json.Unmarshal(data2, &parsed2)
	params := parsed2["params"].(map[string]interface{})
	if params["offset"].(float64) != 42 {
		t.Errorf("offset: got %v", params["offset"])
	}
}
