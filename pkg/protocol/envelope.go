package protocol

import "time"

// Envelope wraps a message payload with routing and tracing metadata.
// The type field enables routing and versioning in Phase 2.
// Nebs ignore fields they don't understand.
type Envelope struct {
	ID       string            `json:"id"`
	Sender   string            `json:"sender,omitempty"`
	Type     string            `json:"type"`
	Payload  []byte            `json:"payload"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewEnvelope creates an envelope with sensible defaults.
func NewEnvelope(id string, payload []byte) *Envelope {
	return &Envelope{
		ID:      id,
		Type:    "default",
		Payload: payload,
		Metadata: map[string]string{
			"created_at": time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// SetTraceID sets the trace_id metadata for cross-copt tracing (Phase 2).
func (e *Envelope) SetTraceID(traceID string) {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata["trace_id"] = traceID
}

// TraceID returns the trace_id from metadata, or empty string if not set.
func (e *Envelope) TraceID() string {
	if e.Metadata == nil {
		return ""
	}
	return e.Metadata["trace_id"]
}
