# Antipatterns: cmd/nile

## Active

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

### PID file write error silently ignored — resolved
Now logs a warning on write failure.

### ReadDeadLetters called after WAL Close / nil pointer — resolved
Moved `ReadDeadLetters()` inside the `if err == nil` block, before `wlog.Close()`.

### Signal goroutine orphaned on non-signal exit — resolved
Signal goroutine now loops: first signal triggers graceful shutdown, second signal forces `os.Exit(1)`. `signal.Stop(sigCh)` called after `mgr.Start()` returns to deregister.
