# Antipatterns: transport

## Active

### No read timeout — pump hangs forever if neb stalls [Severity: High]

`Stdio.Send()` calls `s.reader.Scan()` which blocks indefinitely. If the neb hangs between receiving a request and writing its response, the entire lifecycle manager goroutine is stuck forever. The manager's `stopCh` cannot interrupt this because the goroutine is blocked inside `Send()`, not in the pump's `select`.

**Fix**: Accept a `context.Context` in `Send()`. Use a deadline-aware reader or a goroutine with select on context cancellation. Alternatively, set a read deadline on the underlying pipe fd.

### Close() doesn't close underlying I/O [Severity: Medium]

`Close()` only sets `s.closed = true`. It does not close the writer or drain the scanner. Any goroutine blocked in `Scan()` when `Close()` is called remains blocked. Resources are not released.

**Fix**: Close the underlying writer in `Close()`, which will cause `Scan()` to return with an error (broken pipe on the neb side, EOF on ours).

### Mutex held across blocking I/O [Severity: Medium]

`Send()` holds `s.mu` across both the write and the blocking `Scan()`. Currently safe because the transport is always used synchronously from one goroutine, but the lock would deadlock if two goroutines ever called `Send()` concurrently. No documentation states the single-caller constraint.

**Fix**: Document the single-caller requirement, or split into separate write and read locks, or remove the mutex entirely since it's single-threaded.

## Resolved

*(none yet)*
