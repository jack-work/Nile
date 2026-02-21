// Package wal implements a segmented, append-only write-ahead log with
// CRC32 integrity checks, cursor tracking, retention policies, and snapshots.
package wal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gluck/nile/pkg/store"
)

var (
	// ErrNoMessages is an alias for store.ErrNoMessages so callers can
	// use either wal.ErrNoMessages or store.ErrNoMessages interchangeably.
	ErrNoMessages = store.ErrNoMessages
	ErrClosed     = errors.New("wal: log is closed")
)

// Options configures the WAL.
type Options struct {
	MaxMessages int   // trigger retention when consumed count exceeds this
	MaxBytes    int64 // trigger retention when total log bytes exceed this
	SegmentSize int64 // bytes per segment before rolling to a new one
	MaxDepth    int   // max unprocessed messages (Phase 2: HTTP 429 when exceeded)
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		MaxMessages: 10000,
		MaxBytes:    10 * 1024 * 1024, // 10 MiB
		SegmentSize: 1024 * 1024,      // 1 MiB
	}
}

// Compile-time check: *Log implements store.Store.
var _ store.Store = (*Log)(nil)

// Log is a segmented, append-only write-ahead log.
type Log struct {
	mu       sync.Mutex
	dir      string // path to stream/ directory
	dataDir  string // parent of stream/ (the copt data dir)
	opts     Options
	segments []*segment // sorted by baseIndex
	active   *segment   // current writable segment
	nextIdx  uint64     // next index to assign on append
	cur      CursorState
	cursor   Cursor // persistence backend
	closed   bool

	// in-memory index: maps message offset -> segment index + record index within segment
	// rebuilt on recovery
	index []indexEntry
}

type indexEntry struct {
	segIdx    int // index into l.segments
	recordIdx int // record number within segment
}

// Open opens or creates a WAL in the given data directory.
// The stream/ subdirectory is used for segment files.
func Open(dataDir string, opts Options) (*Log, error) {
	streamDir := filepath.Join(dataDir, "stream")
	if err := os.MkdirAll(streamDir, 0755); err != nil {
		return nil, fmt.Errorf("wal: create stream dir: %w", err)
	}

	cursor := NewJSONCursor(dataDir)
	state, err := cursor.Load()
	if err != nil {
		return nil, fmt.Errorf("wal: load cursor: %w", err)
	}

	l := &Log{
		dir:     streamDir,
		dataDir: dataDir,
		opts:    opts,
		cur:     state,
		cursor:  cursor,
	}

	if err := l.recover(); err != nil {
		return nil, fmt.Errorf("wal: recovery: %w", err)
	}

	return l, nil
}

// Append adds a message to the log and returns its offset.
func (l *Log) Append(payload []byte) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return 0, ErrClosed
	}

	// Roll segment if needed
	if l.active == nil || l.active.size >= l.opts.SegmentSize {
		if err := l.rollSegment(); err != nil {
			return 0, fmt.Errorf("wal: roll segment: %w", err)
		}
	}

	offset := l.nextIdx
	if err := l.active.append(payload); err != nil {
		return 0, fmt.Errorf("wal: append: %w", err)
	}

	l.index = append(l.index, indexEntry{
		segIdx:    len(l.segments) - 1,
		recordIdx: int(offset - l.active.baseIndex),
	})
	l.nextIdx++

	return offset, nil
}

// NextUnprocessed returns the next message that hasn't been consumed.
// Returns ErrNoMessages if all messages have been processed.
// If caught up, re-scans the active segment and directory for records
// appended by external processes (e.g. nile send).
func (l *Log) NextUnprocessed() (uint64, []byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return 0, nil, ErrClosed
	}

	offset := l.cur.Consumed
	if offset >= l.nextIdx {
		// Re-scan for externally appended records before giving up
		if err := l.refresh(); err != nil {
			return 0, nil, fmt.Errorf("wal: refresh: %w", err)
		}
		if offset >= l.nextIdx {
			return 0, nil, ErrNoMessages
		}
	}

	entry := l.index[offset]
	seg := l.segments[entry.segIdx]
	records, err := seg.readAll()
	if err != nil {
		return 0, nil, fmt.Errorf("wal: read segment: %w", err)
	}

	if entry.recordIdx >= len(records) {
		return 0, nil, fmt.Errorf("wal: index inconsistency at offset %d", offset)
	}

	return offset, records[entry.recordIdx], nil
}

// MarkProcessed advances the consumed cursor past the given offset.
func (l *Log) MarkProcessed(offset uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	if offset+1 > l.cur.Consumed {
		l.cur.Consumed = offset + 1
	}
	return l.cursor.Save(l.cur)
}

// MarkPostProcessed records that post-processing is complete for an offset.
func (l *Log) MarkPostProcessed(offset uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	if offset > l.cur.PostProcessed {
		l.cur.PostProcessed = offset
	}
	return l.cursor.Save(l.cur)
}

// RetentionExceeded returns true if the log has exceeded its retention limits.
func (l *Log) RetentionExceeded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.retentionExceeded()
}

// Snapshot concatenates all segment files into a single snapshot file at dest,
// prefixed with a snapshot header (magic + version).
func (l *Log) Snapshot(dest string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	return l.snapshot(dest)
}

// Truncate deletes all segments and resets cursors. Called after retain() completes.
func (l *Log) Truncate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	// Close all segments
	for _, seg := range l.segments {
		seg.close()
	}

	// Delete all segment files
	indices, err := listSegments(l.dir)
	if err != nil {
		return fmt.Errorf("wal: list segments for truncate: %w", err)
	}
	for _, idx := range indices {
		path := filepath.Join(l.dir, segmentName(idx))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("wal: remove segment: %w", err)
		}
	}

	// Reset state
	l.segments = nil
	l.active = nil
	l.index = nil
	l.nextIdx = 0
	l.cur = CursorState{}

	return l.cursor.Save(l.cur)
}

// Depth returns the number of unprocessed messages in the WAL.
func (l *Log) Depth() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.nextIdx > l.cur.Consumed {
		return l.nextIdx - l.cur.Consumed
	}
	return 0
}

// TotalBytes returns the total size of all segments.
func (l *Log) TotalBytes() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	var total int64
	for _, seg := range l.segments {
		total += seg.size
	}
	return total
}

// NextIndex returns the next index that will be assigned on append.
func (l *Log) NextIndex() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.nextIdx
}

// Close closes the WAL.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	for _, seg := range l.segments {
		seg.close()
	}
	return nil
}

// refresh re-reads segments to discover records appended by external processes.
// Called when the pump is caught up and polling for new messages.
func (l *Log) refresh() error {
	// Check for new segment files
	indices, err := listSegments(l.dir)
	if err != nil {
		return err
	}

	// Track which segments we already know about
	known := make(map[uint64]bool)
	for _, seg := range l.segments {
		known[seg.baseIndex] = true
	}

	// Add any new segments
	for _, baseIdx := range indices {
		if known[baseIdx] {
			continue
		}
		path := filepath.Join(l.dir, segmentName(baseIdx))
		records, err := (&segment{path: path}).readAll()
		if err != nil {
			return fmt.Errorf("read new segment %s: %w", path, err)
		}

		segIdx := len(l.segments)
		info, _ := os.Stat(path)
		var size int64
		if info != nil {
			size = info.Size()
		}
		l.segments = append(l.segments, &segment{
			path:      path,
			baseIndex: baseIdx,
			size:      size,
		})

		for recIdx := range records {
			l.index = append(l.index, indexEntry{
				segIdx:    segIdx,
				recordIdx: recIdx,
			})
		}
		if count := uint64(len(records)); baseIdx+count > l.nextIdx {
			l.nextIdx = baseIdx + count
		}
	}

	// Re-read the last known segment to pick up appended records
	if len(l.segments) > 0 {
		lastSeg := l.segments[len(l.segments)-1]
		records, err := (&segment{path: lastSeg.path}).readAll()
		if err != nil {
			return fmt.Errorf("re-read segment %s: %w", lastSeg.path, err)
		}

		knownCount := int(l.nextIdx - lastSeg.baseIndex)
		if len(records) > knownCount {
			// New records in this segment
			segIdx := len(l.segments) - 1
			for recIdx := knownCount; recIdx < len(records); recIdx++ {
				l.index = append(l.index, indexEntry{
					segIdx:    segIdx,
					recordIdx: recIdx,
				})
			}
			l.nextIdx = lastSeg.baseIndex + uint64(len(records))

			// Update size
			info, _ := os.Stat(lastSeg.path)
			if info != nil {
				lastSeg.size = info.Size()
			}
		}
	}

	return nil
}

// rollSegment creates a new active segment.
func (l *Log) rollSegment() error {
	if l.active != nil {
		l.active.close()
	}

	seg, err := createSegment(l.dir, l.nextIdx)
	if err != nil {
		return err
	}

	l.segments = append(l.segments, seg)
	l.active = seg
	return nil
}

// recover replays all segments to rebuild the in-memory index.
func (l *Log) recover() error {
	indices, err := listSegments(l.dir)
	if err != nil {
		return err
	}

	if len(indices) == 0 {
		return nil
	}

	l.index = nil
	l.segments = nil

	for i, baseIdx := range indices {
		path := filepath.Join(l.dir, segmentName(baseIdx))
		seg, err := openSegment(path, baseIdx)
		if err != nil {
			return fmt.Errorf("open segment %s: %w", path, err)
		}
		seg.close()

		records, err := seg.readAll()
		if err != nil {
			return fmt.Errorf("read segment %s: %w", path, err)
		}

		segIdx := len(l.segments)
		l.segments = append(l.segments, &segment{
			path:      path,
			baseIndex: baseIdx,
			size:      seg.size,
		})

		for recIdx := range records {
			l.index = append(l.index, indexEntry{
				segIdx:    segIdx,
				recordIdx: recIdx,
			})
		}

		if count := uint64(len(records)); baseIdx+count > l.nextIdx {
			l.nextIdx = baseIdx + count
		}

		// Last segment becomes the active (writable) one
		if i == len(indices)-1 {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("reopen active segment: %w", err)
			}
			activeSeg := l.segments[segIdx]
			activeSeg.file = f
			l.active = activeSeg
		}
	}

	return nil
}
