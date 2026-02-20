package wal

import (
	"fmt"
	"os"
	"path/filepath"
)

// retentionExceeded checks whether the log has exceeded its configured limits.
func (l *Log) retentionExceeded() bool {
	if l.opts.MaxMessages > 0 && l.cur.Consumed > uint64(l.opts.MaxMessages) {
		return true
	}

	if l.opts.MaxBytes > 0 {
		var total int64
		for _, seg := range l.segments {
			total += seg.size
		}
		if total > l.opts.MaxBytes {
			return true
		}
	}

	return false
}

// DeadLetter writes a message payload to the dead letter segment in dead/.
// Called when a neb has exhausted retries for a message. The cursor is advanced
// past the message so the pump can continue.
func (l *Log) DeadLetter(offset uint64, payload []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	deadDir := filepath.Join(l.dataDir, "dead")
	if err := os.MkdirAll(deadDir, 0755); err != nil {
		return fmt.Errorf("wal: create dead dir: %w", err)
	}

	// Append to a single dead letter segment file
	path := filepath.Join(deadDir, "dead.wal")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("wal: open dead letter: %w", err)
	}
	defer f.Close()

	if _, err := encodeRecord(f, payload); err != nil {
		return fmt.Errorf("wal: write dead letter: %w", err)
	}
	if err := fdatasync(f); err != nil {
		return fmt.Errorf("wal: sync dead letter: %w", err)
	}

	// Advance cursor past this message
	if offset+1 > l.cur.Consumed {
		l.cur.Consumed = offset + 1
	}
	return l.cursor.Save(l.cur)
}

// ReadDeadLetters reads all dead-lettered message payloads.
func (l *Log) ReadDeadLetters() ([][]byte, error) {
	path := filepath.Join(l.dataDir, "dead", "dead.wal")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("wal: open dead letters: %w", err)
	}
	defer f.Close()

	seg := &segment{path: path}
	_ = seg // readAll opens its own file handle
	tmpSeg := &segment{path: path}
	return tmpSeg.readAll()
}
