package wal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

const segmentPrefix = "seg-"
const segmentExt = ".wal"

// segment represents a single WAL segment file.
type segment struct {
	path      string
	baseIndex uint64 // first message index in this segment
	file      *os.File
	size      int64
}

// segmentName returns the filename for a segment starting at baseIndex.
func segmentName(baseIndex uint64) string {
	return fmt.Sprintf("%s%06d%s", segmentPrefix, baseIndex, segmentExt)
}

// parseSegmentName extracts the base index from a segment filename.
func parseSegmentName(name string) (uint64, bool) {
	if !strings.HasPrefix(name, segmentPrefix) || !strings.HasSuffix(name, segmentExt) {
		return 0, false
	}
	numStr := strings.TrimPrefix(name, segmentPrefix)
	numStr = strings.TrimSuffix(numStr, segmentExt)
	n, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// openSegment opens an existing segment file for reading.
func openSegment(path string, baseIndex uint64) (*segment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &segment{
		path:      path,
		baseIndex: baseIndex,
		file:      f,
		size:      info.Size(),
	}, nil
}

// createSegment creates a new segment file for writing.
func createSegment(dir string, baseIndex uint64) (*segment, error) {
	name := segmentName(baseIndex)
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &segment{
		path:      path,
		baseIndex: baseIndex,
		file:      f,
		size:      0,
	}, nil
}

// append writes a record to the segment and syncs to disk.
func (s *segment) append(payload []byte) error {
	n, err := encodeRecord(s.file, payload)
	if err != nil {
		return err
	}
	if err := fdatasync(s.file); err != nil {
		return err
	}
	s.size += int64(n)
	return nil
}

// close closes the segment file.
func (s *segment) close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// readAll reads all records from the segment. Returns payloads and any error.
// On ErrTruncated, returns records read so far with nil error (partial last record is discarded).
// On ErrCorruptRecord, returns records read so far and the error.
func (s *segment) readAll() ([][]byte, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records [][]byte
	for {
		payload, err := decodeRecord(f)
		if err == io.EOF {
			return records, nil
		}
		if err == ErrTruncated {
			// Partial write at end of segment; discard it
			return records, nil
		}
		if err != nil {
			return records, err
		}
		records = append(records, payload)
	}
}

// copyTo writes the entire segment file content to w.
func (s *segment) copyTo(w io.Writer) error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// listSegments returns sorted segment base indices found in dir.
func listSegments(dir string) ([]uint64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var indices []uint64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		idx, ok := parseSegmentName(e.Name())
		if ok {
			indices = append(indices, idx)
		}
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	return indices, nil
}

func fdatasync(f *os.File) error {
	return syscall.Fdatasync(int(f.Fd()))
}
