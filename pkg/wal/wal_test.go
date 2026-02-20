package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestRecordRoundTrip(t *testing.T) {
	payloads := [][]byte{
		[]byte("hello"),
		[]byte(""),
		[]byte("a longer payload with some data in it"),
		{0x00, 0xFF, 0x80},
	}

	var buf bytes.Buffer
	for _, p := range payloads {
		if _, err := encodeRecord(&buf, p); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}

	r := bytes.NewReader(buf.Bytes())
	for i, want := range payloads {
		got, err := decodeRecord(r)
		if err != nil {
			t.Fatalf("decode record %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("record %d: got %q, want %q", i, got, want)
		}
	}
}

func TestRecordCorrupt(t *testing.T) {
	var buf bytes.Buffer
	encodeRecord(&buf, []byte("test"))
	data := buf.Bytes()
	data[len(data)-1] ^= 0xFF
	r := bytes.NewReader(data)
	_, err := decodeRecord(r)
	if err != ErrCorruptRecord {
		t.Fatalf("expected ErrCorruptRecord, got %v", err)
	}
}

func TestOpenAppendRead(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.SegmentSize = 1024 * 1024

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	for i := 0; i < 10; i++ {
		offset, err := log.Append([]byte("msg"))
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if offset != uint64(i) {
			t.Errorf("offset: got %d, want %d", offset, i)
		}
	}

	for i := 0; i < 10; i++ {
		offset, data, err := log.NextUnprocessed()
		if err != nil {
			t.Fatalf("next %d: %v", i, err)
		}
		if offset != uint64(i) {
			t.Errorf("offset: got %d, want %d", offset, i)
		}
		if string(data) != "msg" {
			t.Errorf("data: got %q, want %q", data, "msg")
		}
		if err := log.MarkProcessed(offset); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}

	_, _, err = log.NextUnprocessed()
	if err != ErrNoMessages {
		t.Fatalf("expected ErrNoMessages, got %v", err)
	}
}

func TestSegmentRollover(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.SegmentSize = 50

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	for i := 0; i < 20; i++ {
		if _, err := log.Append([]byte("hello world")); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	indices, err := listSegments(filepath.Join(dir, "stream"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(indices) < 2 {
		t.Errorf("expected multiple segments, got %d", len(indices))
	}

	for i := 0; i < 20; i++ {
		offset, data, err := log.NextUnprocessed()
		if err != nil {
			t.Fatalf("next %d: %v", i, err)
		}
		if offset != uint64(i) {
			t.Errorf("offset: got %d, want %d", offset, i)
		}
		if string(data) != "hello world" {
			t.Errorf("data: got %q", data)
		}
		log.MarkProcessed(offset)
	}
}

func TestRecovery(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.SegmentSize = 100

	log1, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	for i := 0; i < 5; i++ {
		log1.Append([]byte("recover-test"))
	}
	for i := 0; i < 3; i++ {
		offset, _, _ := log1.NextUnprocessed()
		log1.MarkProcessed(offset)
	}
	log1.Close()

	log2, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer log2.Close()

	offset, data, err := log2.NextUnprocessed()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if offset != 3 {
		t.Errorf("offset: got %d, want 3", offset)
	}
	if string(data) != "recover-test" {
		t.Errorf("data: got %q", data)
	}
	if log2.NextIndex() != 5 {
		t.Errorf("nextIdx: got %d, want 5", log2.NextIndex())
	}
}

func TestRetention(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.MaxMessages = 5
	opts.MaxBytes = 0
	opts.SegmentSize = 1024 * 1024

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	if log.RetentionExceeded() {
		t.Error("retention should not be exceeded yet")
	}

	for i := 0; i < 6; i++ {
		log.Append([]byte("x"))
	}
	for i := 0; i < 6; i++ {
		offset, _, _ := log.NextUnprocessed()
		log.MarkProcessed(offset)
	}

	if !log.RetentionExceeded() {
		t.Error("retention should be exceeded")
	}
}

func TestSnapshotAndTruncate(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.SegmentSize = 50

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	for i := 0; i < 10; i++ {
		log.Append([]byte("snap-data"))
	}

	snapPath := filepath.Join(dir, "retain", "test-snap.wal")
	if err := log.Snapshot(snapPath); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	// Snapshot file should exist and be non-empty
	info, err := os.Stat(snapPath)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if info.Size() == 0 {
		t.Error("snapshot is empty")
	}

	// Verify snapshot header
	f, err := os.Open(snapPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer f.Close()
	version, err := readSnapshotHeader(f)
	if err != nil {
		t.Fatalf("read snapshot header: %v", err)
	}
	if version != snapshotVersion {
		t.Errorf("snapshot version: got %d, want %d", version, snapshotVersion)
	}

	// Truncate
	if err := log.Truncate(); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	if log.NextIndex() != 0 {
		t.Errorf("nextIdx after truncate: got %d, want 0", log.NextIndex())
	}
	_, _, err = log.NextUnprocessed()
	if err != ErrNoMessages {
		t.Errorf("expected ErrNoMessages after truncate, got %v", err)
	}

	offset, err := log.Append([]byte("after-truncate"))
	if err != nil {
		t.Fatalf("append after truncate: %v", err)
	}
	if offset != 0 {
		t.Errorf("offset after truncate: got %d, want 0", offset)
	}

	log.Close()
}

func TestByteRetention(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()
	opts.MaxMessages = 0
	opts.MaxBytes = 100
	opts.SegmentSize = 1024 * 1024

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	if log.RetentionExceeded() {
		t.Error("should not be exceeded yet")
	}

	for i := 0; i < 20; i++ {
		log.Append([]byte("some data that adds up"))
	}

	if !log.RetentionExceeded() {
		t.Errorf("should be exceeded, total bytes: %d", log.TotalBytes())
	}
}

func TestClosedLog(t *testing.T) {
	dir := tempDir(t)
	log, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	log.Close()

	if _, err := log.Append([]byte("x")); err != ErrClosed {
		t.Errorf("append on closed: got %v, want ErrClosed", err)
	}
	if _, _, err := log.NextUnprocessed(); err != ErrClosed {
		t.Errorf("next on closed: got %v, want ErrClosed", err)
	}
}

func TestDeadLetter(t *testing.T) {
	dir := tempDir(t)
	opts := DefaultOptions()

	log, err := Open(dir, opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	// Append messages
	log.Append([]byte("good-msg"))
	log.Append([]byte("bad-msg"))
	log.Append([]byte("next-msg"))

	// Process first
	offset, _, _ := log.NextUnprocessed()
	log.MarkProcessed(offset)

	// Dead letter the second
	offset2, payload2, _ := log.NextUnprocessed()
	if err := log.DeadLetter(offset2, payload2); err != nil {
		t.Fatalf("dead letter: %v", err)
	}

	// Next should be the third message
	offset3, data3, err := log.NextUnprocessed()
	if err != nil {
		t.Fatalf("next after dead letter: %v", err)
	}
	if offset3 != 2 {
		t.Errorf("offset: got %d, want 2", offset3)
	}
	if string(data3) != "next-msg" {
		t.Errorf("data: got %q, want %q", data3, "next-msg")
	}

	// Read dead letters
	dead, err := log.ReadDeadLetters()
	if err != nil {
		t.Fatalf("read dead letters: %v", err)
	}
	if len(dead) != 1 {
		t.Fatalf("dead letters: got %d, want 1", len(dead))
	}
	if string(dead[0]) != "bad-msg" {
		t.Errorf("dead letter: got %q, want %q", dead[0], "bad-msg")
	}
}

func TestDepth(t *testing.T) {
	dir := tempDir(t)
	log, err := Open(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()

	if log.Depth() != 0 {
		t.Errorf("initial depth: got %d, want 0", log.Depth())
	}

	for i := 0; i < 5; i++ {
		log.Append([]byte("msg"))
	}
	if log.Depth() != 5 {
		t.Errorf("depth after 5 appends: got %d, want 5", log.Depth())
	}

	offset, _, _ := log.NextUnprocessed()
	log.MarkProcessed(offset)
	if log.Depth() != 4 {
		t.Errorf("depth after 1 processed: got %d, want 4", log.Depth())
	}
}

func TestCursorInterface(t *testing.T) {
	dir := tempDir(t)
	c := NewJSONCursor(dir)

	// Load from empty
	state, err := c.Load()
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if state.Consumed != 0 || state.PostProcessed != 0 {
		t.Errorf("expected zero state, got %+v", state)
	}

	// Save and reload
	state.Consumed = 42
	state.PostProcessed = 10
	if err := c.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := c.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != state {
		t.Errorf("loaded %+v, want %+v", loaded, state)
	}
}

func TestSnapshotHeader(t *testing.T) {
	var buf bytes.Buffer

	if err := writeSnapshotHeader(&buf); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if buf.Len() != snapshotHeaderSize {
		t.Errorf("header size: got %d, want %d", buf.Len(), snapshotHeaderSize)
	}

	version, err := readSnapshotHeader(&buf)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if version != snapshotVersion {
		t.Errorf("version: got %d, want %d", version, snapshotVersion)
	}

	// Invalid magic
	bad := bytes.NewReader([]byte("BAAD\x01\x00\x00\x00"))
	_, err = readSnapshotHeader(bad)
	if err == nil {
		t.Error("expected error for bad magic")
	}
}
