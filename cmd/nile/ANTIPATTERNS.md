# Antipatterns: cmd/nile

## Active

### PID file write error silently ignored [Severity: Low]

`os.WriteFile(pidFile, ...)` return value is not checked. If the write fails, `cmdStatus` incorrectly reports the copt as not running.

### cmdStatus opens WAL concurrently with running copt [Severity: High]

`cmdStatus` calls `wal.Open()` on the same data directory that `cmdRun` has open. Since there's no file locking, `recover()` opens the last segment for writing, potentially corrupting it.

**Fix**: Add a read-only WAL open mode, or just parse cursor.json directly for status info.

### No timeout on neb Wait() [Severity: Low]

`nebCmd.Wait()` blocks indefinitely. If the neb hangs on shutdown, the runtime hangs too.

**Fix**: Use a context with timeout, or send SIGKILL after a grace period.

## Resolved

### CLI arg parsing panics on missing values — resolved
Added `requireValue()` bounds check before accessing `args[i]` for all flags.

### Numeric flag parse errors silently produce zero — resolved
Added `parseInt()` and `parseInt64()` helpers that exit with a clear error message on bad input.
