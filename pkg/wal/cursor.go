package wal

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Cursor tracks consumption progress through the WAL.
// Implementations must be safe for concurrent use from a single goroutine
// (the WAL holds a mutex around all cursor operations).
type Cursor interface {
	// Load reads the cursor state from persistent storage.
	Load() (CursorState, error)

	// Save persists the cursor state atomically.
	Save(state CursorState) error
}

// CursorState holds the consumption progress.
type CursorState struct {
	Consumed      uint64 `json:"consumed"`       // next offset to deliver
	PostProcessed uint64 `json:"post_processed"` // highest post-processed offset
}

// JSONCursor persists cursor state as a JSON file with atomic rename.
// Suitable for Phase 1 throughput. If this becomes a bottleneck, swap to
// a fixed-size binary cursor (16 bytes: two uint64s) with pwrite+fdatasync.
type JSONCursor struct {
	path string
}

// NewJSONCursor creates a cursor backed by a JSON file at dir/cursor.json.
func NewJSONCursor(dir string) *JSONCursor {
	return &JSONCursor{path: filepath.Join(dir, "cursor.json")}
}

func (c *JSONCursor) Load() (CursorState, error) {
	var state CursorState
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(data, &state)
	return state, err
}

func (c *JSONCursor) Save(state CursorState) error {
	data, err := json.Marshal(&state)
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
