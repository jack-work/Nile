# Copt Runtime

The runtime manages the copt lifecycle: neb spawning, message delivery, retention, and shutdown.

## Lifecycle States

```
Created -> Starting -> Idle -> Processing -> [PostProcessing] -> Idle
                        |                                         |
                        +-- retention limit --> Draining -> Retaining -> Idle
                        |
                        +-- shutdown --> Stopping -> Stopped
                        +-- crash --> Failed -> (restart)
```

## Message Pump

The core loop:

1. Check for stop signal
2. Read next unprocessed message from WAL
3. If no messages: check retention, then poll
4. Send `message` to neb via JSON-RPC
5. On success: mark processed, advance cursor
6. On failure: retry with exponential backoff, then dead-letter
7. Check retention limits
8. Repeat

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `--message-timeout` | 60s | Neb response timeout |
| `--max-retries` | 3 | Retries before dead-letter |
| `--max-messages` | 10000 | Retention: max consumed messages |
| `--max-bytes` | 10 MiB | Retention: max log bytes |
| `--segment-size` | 1 MiB | Bytes per WAL segment |
| `--max-depth` | 0 | Max unprocessed messages (Phase 2: HTTP 429) |

## Retention Cycle

1. **Drain**: Stop delivering new messages
2. **Snapshot**: Concatenate all segments into `retain/snap-<timestamp>.wal`
3. **Retain**: Send snapshot path to neb via `retain` method
4. **Truncate**: Delete all segments, reset cursors

## Dead Letter Flow

1. Message delivery fails
2. Retry up to `maxRetries` with backoff (100ms, 200ms, 400ms...)
3. After exhausting retries: write to `dead/dead.wal`, advance cursor
4. Pump continues with next message

## Process Management

- PID file written to `run/nile.pid`
- Handles SIGTERM/SIGINT for graceful shutdown
- Sends `shutdown` method to neb before exiting
- Neb stderr forwarded to runtime stderr (journald)
