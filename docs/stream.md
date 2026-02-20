# Stream (WAL)

The WAL is a segmented, append-only log with CRC32 integrity checks.

## On-Disk Layout

```
/var/lib/nile/<copt>/
  stream/
    seg-000000.wal        # messages 0-N
    seg-000100.wal        # messages 100-199
    seg-000200.wal        # active segment
  dead/
    dead.wal              # dead-lettered messages
  cursor.json             # {"consumed": 150, "post_processed": 120}
  config.json             # runtime config
  state/                  # neb-owned persistent state
  retain/                 # snapshot files
  telemetry/              # traces.jsonl, metrics.jsonl
  run/                    # pid file
```

## Record Format

```
[length: uint32][crc32: uint32][payload: []byte]
```

Each record is 8 bytes of header plus the payload. CRC32 (IEEE) is computed over the payload and verified on read.

## Segment Files

Segments are named `seg-NNNNNN.wal` where `NNNNNN` is the zero-padded base index. New segments are created when the active segment exceeds `SegmentSize` bytes (default: 1 MiB).

## Snapshot Format

```
[magic: "NILE" (4 bytes)][version: uint32][segment data...]
```

Snapshots concatenate all segments into a single file, prefixed with a magic number and version for forward compatibility. Written to `retain/snap-<timestamp>.wal`.

`io.Copy` between `*os.File` uses `copy_file_range()` on Linux -- kernel-level copy, no userspace buffering. On CoW filesystems (btrfs), this is a near-instant reflink.

## Durability

- `os.OpenFile` with `O_APPEND|O_CREATE|O_WRONLY`
- `fdatasync()` after each append
- CRC32 per record, verified on recovery
- Cursor persistence via atomic rename (`cursor.json.tmp` -> `cursor.json`)
- Truncated records at segment end are silently discarded on recovery

## Cursor

The cursor tracks two offsets:
- `consumed`: next offset to deliver to the neb
- `post_processed`: highest offset where neb requested post-processing

Backed by a `Cursor` interface (default: `JSONCursor`). Swappable to a binary cursor if throughput demands it.

## Dead Letters

Messages that exhaust retry attempts are written to `dead/dead.wal` using the same record format. The cursor advances past dead-lettered messages so the pump continues.

## Retention

Retention is triggered when either limit is exceeded:
- `MaxMessages`: consumed message count > threshold
- `MaxBytes`: total WAL size on disk > threshold

The retention cycle: drain -> snapshot -> send retain to neb -> truncate (delete all segments, reset cursors).
