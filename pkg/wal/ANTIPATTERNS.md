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

### No directory fsync after snapshot creation [Severity: Low]

`f.Sync()` fsyncs file data but not the directory entry. On crash between file sync and directory sync, the snapshot may not appear in directory listings on recovery.

### Cursor .tmp file not cleaned on crash [Severity: Low]

`saveCursor()` writes to `.tmp` then renames. If the process crashes after write but before rename, the stale `.tmp` persists. Harmless (next save overwrites it) but untidy.

### No maximum record size — corrupt segment can OOM [Severity: Medium]

`decodeRecord()` allocates `make([]byte, length)` where `length` is read from the segment file with no upper bound. A corrupt or malicious segment with `length=0xFFFFFFFF` attempts a 4 GiB allocation.

**Fix**: Enforce a maximum payload size and return `ErrRecordTooLarge`.

## Resolved

### Snapshot leaves partial file on failure — resolved
Snapshot now writes to a temp file and renames atomically. On any error, the temp file is cleaned up. No partial snapshots left on disk.

### No maximum record size — resolved
Added `maxRecordPayload` (64 MiB) constant and `ErrRecordTooLarge`. `decodeRecord()` rejects records exceeding this limit before allocating.

### Concurrent append corruption via split writes — resolved in 1bc7992c7249
`encodeRecord` previously did two `Write()` calls (header then payload). When multiple processes appended concurrently to an O_APPEND fd, the writes could interleave (e.g., header A, header B, payload A, payload B), producing corrupt records. Fixed by combining header + payload into a single buffer and issuing one `Write()` call, which is atomic under O_APPEND for writes ≤ PIPE_BUF (4096 bytes on Linux). Records larger than PIPE_BUF are still theoretically vulnerable but in practice are safe on local filesystems.
