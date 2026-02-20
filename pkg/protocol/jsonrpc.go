// Package protocol defines JSON-RPC 2.0 message types for the Nile runtime <-> neb protocol.
package protocol

import (
	"encoding/json"
	"fmt"
)

const Version = "2.0"

// Request is a JSON-RPC 2.0 request from the runtime to the neb.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      uint64          `json:"id"`
}

// Response is a JSON-RPC 2.0 response from the neb to the runtime.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      uint64          `json:"id"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Methods
const (
	MethodInit     = "init"
	MethodMessage  = "message"
	MethodRetain   = "retain"
	MethodShutdown = "shutdown"
)

// InitParams is sent to the neb on startup.
type InitParams struct {
	Name   string            `json:"name"`
	Config map[string]string `json:"config,omitempty"`
}

// MessageParams delivers a single WAL message to the neb.
type MessageParams struct {
	Offset uint64 `json:"offset"`
	Data   string `json:"data"` // base64-encoded payload
}

// RetainParams tells the neb to process a snapshot.
type RetainParams struct {
	Snapshot string `json:"snapshot"`
}

// StatusResult is the standard neb response.
type StatusResult struct {
	Status      string `json:"status"`
	PostProcess bool   `json:"post_process,omitempty"`
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id uint64, method string, params interface{}) (*Request, error) {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return &Request{
		JSONRPC: Version,
		Method:  method,
		Params:  raw,
		ID:      id,
	}, nil
}

// Encode serializes a request to JSON bytes (no trailing newline).
func (r *Request) Encode() ([]byte, error) {
	return json.Marshal(r)
}

// DecodeResponse parses a JSON-RPC response.
func DecodeResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

// ParseResult unmarshals the result field into the given target.
func (r *Response) ParseResult(target interface{}) error {
	if r.Error != nil {
		return r.Error
	}
	return json.Unmarshal(r.Result, target)
}
