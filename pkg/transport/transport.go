// Package transport defines the interface for communication between the runtime and neb.
package transport

import "github.com/gluck/nile/pkg/protocol"

// Transport sends requests to the neb and receives responses.
type Transport interface {
	// Send sends a request and waits for the response.
	Send(req *protocol.Request) (*protocol.Response, error)

	// Close shuts down the transport.
	Close() error
}
