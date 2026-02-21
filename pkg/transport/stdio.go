package transport

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gluck/nile/pkg/protocol"
)

// Stdio implements Transport over stdin/stdout pipes to a child process.
// Requests are written as newline-delimited JSON to the neb's stdin.
// Responses are read as newline-delimited JSON from the neb's stdout.
type Stdio struct {
	mu      sync.Mutex
	writer  io.Writer      // neb's stdin
	reader  *bufio.Scanner // neb's stdout
	closed  bool
	broken  bool           // set on timeout; scanner state is unrecoverable
	Timeout time.Duration  // 0 means no timeout
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

type scanResult struct {
	data []byte
	err  error
	eof  bool
}

// Send writes a JSON-RPC request and reads the response synchronously.
func (s *Stdio) Send(req *protocol.Request) (*protocol.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("transport: closed")
	}
	if s.broken {
		return nil, fmt.Errorf("transport: broken (previous timeout)")
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

	// Read response, with optional timeout
	result, err := s.readLine()
	if err != nil {
		return nil, err
	}

	resp, err := protocol.DecodeResponse(result)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	return resp, nil
}

// readLine reads the next line from the scanner. If Timeout is set and the
// read doesn't complete in time, the transport is marked broken (the scanner's
// internal buffer is in an indeterminate state after a timeout).
func (s *Stdio) readLine() ([]byte, error) {
	if s.Timeout <= 0 {
		// No timeout: block directly
		if !s.reader.Scan() {
			if err := s.reader.Err(); err != nil {
				return nil, fmt.Errorf("transport: read: %w", err)
			}
			return nil, fmt.Errorf("transport: neb closed stdout")
		}
		// Copy the bytes — scanner reuses the buffer
		return append([]byte(nil), s.reader.Bytes()...), nil
	}

	// With timeout: read in a goroutine
	ch := make(chan scanResult, 1)
	go func() {
		ok := s.reader.Scan()
		if !ok {
			ch <- scanResult{eof: true, err: s.reader.Err()}
			return
		}
		ch <- scanResult{data: append([]byte(nil), s.reader.Bytes()...)}
	}()

	select {
	case r := <-ch:
		if r.eof {
			if r.err != nil {
				return nil, fmt.Errorf("transport: read: %w", r.err)
			}
			return nil, fmt.Errorf("transport: neb closed stdout")
		}
		return r.data, nil
	case <-time.After(s.Timeout):
		// The goroutine is still blocked on Scan(). The scanner is now
		// unrecoverable — mark the transport as broken.
		s.broken = true
		return nil, fmt.Errorf("transport: neb response timeout (%v)", s.Timeout)
	}
}

// Close marks the transport as closed.
func (s *Stdio) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
