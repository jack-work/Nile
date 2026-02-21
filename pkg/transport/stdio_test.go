package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/gluck/nile/pkg/protocol"
)

func TestStdioRoundTrip(t *testing.T) {
	// Simulate neb's stdin (we write) and stdout (we read)
	nebStdinR, nebStdinW := io.Pipe()
	nebStdoutR, nebStdoutW := io.Pipe()

	// Ensure the pipe writer closes even if the test fails early,
	// so the simulated neb goroutine can exit.
	t.Cleanup(func() {
		nebStdinW.Close()
		nebStdoutW.Close()
	})

	tr := NewStdio(nebStdinW, nebStdoutR)

	// Simulate a neb: read request from stdin, write response to stdout
	go func() {
		defer nebStdoutW.Close()
		buf := make([]byte, 4096)
		for {
			n, err := nebStdinR.Read(buf)
			if err != nil {
				return
			}
			// Parse request
			var req protocol.Request
			if err := json.Unmarshal(bytes.TrimSpace(buf[:n]), &req); err != nil {
				return
			}
			// Build response
			result, _ := json.Marshal(protocol.StatusResult{Status: "ok"})
			resp := protocol.Response{
				JSONRPC: "2.0",
				Result:  result,
				ID:      req.ID,
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			nebStdoutW.Write(data)
		}
	}()

	// Send init
	req, err := protocol.NewRequest(1, protocol.MethodInit, protocol.InitParams{
		Name: "test-copt",
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := tr.Send(req)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	var result protocol.StatusResult
	if err := resp.ParseResult(&result); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("status: got %q, want %q", result.Status, "ok")
	}

	// Send message
	req2, _ := protocol.NewRequest(2, protocol.MethodMessage, protocol.MessageParams{
		Offset: 42,
		Data:   "dGVzdA==",
	})
	resp2, err := tr.Send(req2)
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if resp2.ID != 2 {
		t.Errorf("response id: got %d, want 2", resp2.ID)
	}

	tr.Close()
	nebStdinW.Close()
}
