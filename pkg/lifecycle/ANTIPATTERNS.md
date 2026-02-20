# Antipatterns: lifecycle

## Active

### State machine is partially decorative [Severity: High]

`shutdown()` directly assigns `m.state = StateStopping` and `m.state = StateStopped`, bypassing `transition()` and its validity checks. Multiple `transition()` return values are silently discarded (lines 121, 229, 237, 284, 297 in manager.go). The state machine can be violated without any error surfacing.

**Fix**: Make `transition()` the only way to change state. Panic or log-fatal on invalid transitions (they indicate logic bugs). Never discard the error.

### Neb not shut down on send error [Severity: Medium]

If `m.send(MethodMessage, ...)` returns an error in `processMessage()`, the function returns the error, which propagates up through `pump()` and exits. `shutdown()` is never called. The neb process is left running with its stdin pipe open until the OS closes the fd on process exit.

**Fix**: Always call `shutdown()` in a defer or in the error path of `pump()`.

### Polling loop instead of event notification [Severity: Low]

The pump uses a ticker to poll for new messages. This is correct and leak-free, but polling is inherently less efficient than event notification (e.g., inotify on the stream directory or a channel from `Append()`).

**Fix**: Long-term: replace polling with inotify/channel notification when new messages are appended.

### transition() errors silently swallowed [Severity: Medium]

At least 5 call sites discard the error from `transition()`. This means the manager can continue operating in an undefined state without any indication.

### MarkProcessed before PostProcessing complete [Severity: Medium]

`MarkProcessed` is called unconditionally after the neb responds, even when `PostProcess` is true. If the process crashes between `MarkProcessed` and `MarkPostProcessed`, the post-processing cursor is never updated for that offset. The semantics of the two cursors are undocumented.

### Shutdown send error discarded [Severity: Low]

`m.send(MethodShutdown, nil)` in `shutdown()` ignores both the error and result. If the neb is already dead, this fails silently. Callers can't distinguish clean shutdown from shutdown-where-neb-already-died.

## Resolved

### Polling loop with time.After leak — resolved
Replaced `time.After(m.PollInterval)` and `time.Sleep` with a single `time.Ticker`. No more timer goroutine leaks.

### State stuck in StateRetaining on truncate failure — resolved
Added `transition(StateFailed)` before returning truncation errors in `doRetention()`.
