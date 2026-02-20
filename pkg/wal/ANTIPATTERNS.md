# Antipatterns: wal

## Active

### readAll() per NextUnprocessed() — O(N) re-read [Severity: High]

`NextUnprocessed()` calls `seg.readAll()` which opens the segment, decodes every record from the beginning, then returns only `records[entry.recordIdx]`. For a segment with 10k records called 10k times, this is O(N^2) total disk I/O.

**Fix**: Store byte offsets in `indexEntry` instead of record indices. Use `ReadAt`/`Seek` to read a single record directly. Alternatively, cache the decoded records per segment.

### In-memory index grows unboundedly [Severity: Medium]

`l.index` is a flat `[]indexEntry` indexed by absolute message offset. Between retention cycles it grows to hold one entry per message ever appended. For high-throughput copts this is a silent memory leak.

**Fix**: Use a ring buffer or offset-relative indexing. Or compact the index when segments are deleted.

### Recovery opens/closes/reconstructs segments fragily [Severity: Medium]

In `recover()`, segments are opened via `openSegment` (which gets file size), immediately closed, then `readAll()` is called (which re-opens the file by path). A new naked `&segment{}` struct is built with `file: nil`. This works by accident because `readAll` opens its own handle, but it's fragile — any future change to use `s.file` directly would panic.

**Fix**: Refactor recovery to either keep the read handle open or separate "read segment metadata" from "open segment for writing."

### No file locking — concurrent Open() is unsafe [Severity: High]

Two processes can `wal.Open()` the same data directory simultaneously. `cmdStatus` does this while `cmdRun` is active. Since `recover()` opens the last segment for writing, the status command could corrupt the active segment.

**Fix**: Use `flock` on a lock file in the data directory. Or make `cmdStatus` read-only (don't call `Open`, just parse segments directly).

### Snapshot leaves partial file on failure [Severity: Medium]

If `seg.copyTo()` fails mid-write, the partial snapshot file at `dest` is not removed. The retain directory accumulates corrupt snapshots.

**Fix**: Write to a temp file, then rename atomically. Remove temp on error.

### No directory fsync after snapshot creation [Severity: Low]

`f.Sync()` fsyncs file data but not the directory entry. On crash between file sync and directory sync, the snapshot may not appear in directory listings on recovery.

### Cursor .tmp file not cleaned on crash [Severity: Low]

`saveCursor()` writes to `.tmp` then renames. If the process crashes after write but before rename, the stale `.tmp` persists. Harmless (next save overwrites it) but untidy.

### Partial header write leaves segment dirty [Severity: Low]

`encodeRecord` does two `Write` calls (header + payload). If the header succeeds but payload fails, the segment has a dangling 8-byte header. Recovery handles this via `ErrTruncated` (safe), but the segment's `size` field diverges from actual file size since the error is returned before `size` is incremented.

## Resolved

*(none yet)*
