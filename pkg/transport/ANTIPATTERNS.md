# Antipatterns: transport

## Active

### Close() doesn't close underlying I/O [Severity: Medium]

`Close()` only sets `s.closed = true`. It does not close the writer or drain the scanner. Any goroutine blocked in `Scan()` when `Close()` is called remains blocked. Resources are not released.

**Fix**: Close the underlying writer in `Close()`, which will cause `Scan()` to return with an error (broken pipe on the neb side, EOF on ours).

### Mutex held across blocking I/O [Severity: Medium]

`Send()` holds `s.mu` across both the write and the blocking `Scan()`. Currently safe because the transport is always used synchronously from one goroutine, but the lock would deadlock if two goroutines ever called `Send()` concurrently. No documentation states the single-caller constraint.

**Fix**: Document the single-caller requirement, or split into separate write and read locks, or remove the mutex entirely since it's single-threaded.

## Resolved

### No read timeout — resolved
Added `Timeout` field to `Stdio`. When set, `readLine()` reads in a goroutine with a deadline. On timeout, the transport is marked `broken` (scanner state is unrecoverable after a timeout). `cmd/nile/main.go` wires `--message-timeout` to `tr.Timeout`.
