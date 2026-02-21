package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Snapshot file header: [magic: 4 bytes "NILE"][version: uint32]
const (
	snapshotMagic      = "NILE"
	snapshotVersion    = 1
	snapshotHeaderSize = 8
)

var ErrInvalidSnapshot = errors.New("wal: invalid snapshot file")

// writeSnapshotHeader writes the magic bytes and version to w.
func writeSnapshotHeader(w io.Writer) error {
	var hdr [snapshotHeaderSize]byte
	copy(hdr[0:4], snapshotMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], snapshotVersion)
	_, err := w.Write(hdr[:])
	return err
}

// readSnapshotHeader reads and validates the snapshot header from r.
// Returns the version number.
func readSnapshotHeader(r io.Reader) (uint32, error) {
	var hdr [snapshotHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if string(hdr[0:4]) != snapshotMagic {
		return 0, fmt.Errorf("%w: bad magic", ErrInvalidSnapshot)
	}
	version := binary.LittleEndian.Uint32(hdr[4:8])
	return version, nil
}

// snapshot concatenates all segment files into a single snapshot file at dest,
// prefixed with a snapshot header. Uses io.Copy between *os.File which triggers
// copy_file_range on Linux (kernel-level copy, no userspace buffering; on CoW
// filesystems like btrfs this is a near-instant reflink).
func (l *Log) snapshot(dest string) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("wal: create snapshot dir: %w", err)
	}

	// Write to a temp file, then rename. This avoids leaving a partial
	// snapshot on disk if the write fails midway.
	tmp, err := os.CreateTemp(dir, ".snap-*.tmp")
	if err != nil {
		return fmt.Errorf("wal: create snapshot temp: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}

	if err := writeSnapshotHeader(tmp); err != nil {
		cleanup()
		return fmt.Errorf("wal: write snapshot header: %w", err)
	}

	for _, seg := range l.segments {
		if err := seg.copyTo(tmp); err != nil {
			cleanup()
			return fmt.Errorf("wal: copy segment %s: %w", seg.path, err)
		}
	}

	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("wal: sync snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("wal: close snapshot: %w", err)
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("wal: rename snapshot: %w", err)
	}
	return nil
}
