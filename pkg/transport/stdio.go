package transport

import (
	"bufio"
	"fmt"
	"io"
	"sync"

	"github.com/gluck/nile/pkg/protocol"
)

// Stdio implements Transport over stdin/stdout pipes to a child process.
// Requests are written as newline-delimited JSON to the neb's stdin.
// Responses are read as newline-delimited JSON from the neb's stdout.
type Stdio struct {
	mu     sync.Mutex
	writer io.Writer    // neb's stdin
	reader *bufio.Scanner // neb's stdout
	closed bool
}

// NewStdio creates a new stdio transport.
// w is the neb's stdin (we write to it), r is the neb's stdout (we read from it).
func NewStdio(w io.Writer, r io.Reader) *Stdio {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max line
	return &Stdio{
		writer: w,
		reader: scanner,
	}
}

// Send writes a JSON-RPC request and reads the response synchronously.
func (s *Stdio) Send(req *protocol.Request) (*protocol.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("transport: closed")
	}

	data, err := req.Encode()
	if err != nil {
		return nil, fmt.Errorf("transport: encode request: %w", err)
	}

	// Write request + newline
	data = append(data, '\n')
	if _, err := s.writer.Write(data); err != nil {
		return nil, fmt.Errorf("transport: write: %w", err)
	}

	// Read response line
	if !s.reader.Scan() {
		if err := s.reader.Err(); err != nil {
			return nil, fmt.Errorf("transport: read: %w", err)
		}
		return nil, fmt.Errorf("transport: neb closed stdout")
	}

	resp, err := protocol.DecodeResponse(s.reader.Bytes())
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	return resp, nil
}

// Close marks the transport as closed.
func (s *Stdio) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
