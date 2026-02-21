// Package store defines the storage interface for the Nile message stream.
//
// The core invariant: messages are delivered to the neb one at a time,
// synchronously, in strict append order. The neb never sees concurrent
// messages, never needs locks, never observes out-of-order delivery.
// This is what makes the neb contract trivial to implement in any language.
//
// The Store interface abstracts the underlying storage backend. The default
// implementation is a segmented filesystem WAL (pkg/wal). Alternative
// backends (SQLite, embedded KV, PostgreSQL) can implement this interface
// to swap storage without changing the lifecycle manager.
package store

import "errors"

var ErrNoMessages = errors.New("store: no unprocessed messages")

// Store is the storage backend for a copt's message stream.
//
// All methods must be safe for concurrent use. Implementations must
// guarantee that NextUnprocessed returns messages in strict append order
// and never skips or reorders messages.
type Store interface {
	// Append adds a message to the stream and returns its offset.
	Append(payload []byte) (uint64, error)

	// NextUnprocessed returns the next message that hasn't been consumed.
	// Returns offset, payload, and error. Returns ErrNoMessages if caught up.
	NextUnprocessed() (uint64, []byte, error)

	// MarkProcessed advances the consumed cursor past the given offset.
	MarkProcessed(offset uint64) error

	// MarkPostProcessed records that post-processing is complete for an offset.
	MarkPostProcessed(offset uint64) error

	// RetentionExceeded returns true if the stream has exceeded its
	// configured retention limits (message count or byte size).
	RetentionExceeded() bool

	// Snapshot exports the current stream contents to a file at dest.
	// The neb receives this path via the retain() call.
	Snapshot(dest string) error

	// Truncate deletes all stream data and resets cursors.
	// Called after the neb's retain() completes successfully.
	Truncate() error

	// DeadLetter writes a failed message to the dead letter store and
	// advances the cursor past it so the pump can continue.
	DeadLetter(offset uint64, payload []byte) error

	// ReadDeadLetters returns all dead-lettered message payloads.
	ReadDeadLetters() ([][]byte, error)

	// Depth returns the number of unprocessed messages.
	Depth() uint64

	// TotalBytes returns the total storage size.
	TotalBytes() int64

	// Close releases all resources.
	Close() error
}
