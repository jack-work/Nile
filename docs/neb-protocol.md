# Neb Protocol

Communication between the runtime and neb uses **newline-delimited JSON-RPC 2.0** over stdio.

## Transport

- Runtime writes to neb's **stdin**
- Neb writes to **stdout**
- Neb **stderr** goes to the runtime's stderr (visible in journald)
- Strictly synchronous: one request, one response, then next

## Methods

### `init`

Sent once on startup.

```json
→ {"jsonrpc":"2.0","method":"init","params":{"name":"my-copt","config":{}},"id":1}
← {"jsonrpc":"2.0","result":{"status":"ok"},"id":1}
```

### `message`

Delivers a single WAL message. Payload is base64-encoded.

```json
→ {"jsonrpc":"2.0","method":"message","params":{"offset":42,"data":"aGVsbG8="},"id":2}
← {"jsonrpc":"2.0","result":{"status":"ok","post_process":true},"id":2}
```

Set `post_process: true` to record that this message needs post-processing.

### `retain`

Sent when retention triggers. The neb should process the snapshot.

```json
→ {"jsonrpc":"2.0","method":"retain","params":{"snapshot":"/var/lib/nile/my-copt/retain/snap-001.wal"},"id":3}
← {"jsonrpc":"2.0","result":{"status":"ok"},"id":3}
```

### `shutdown`

Sent on graceful shutdown. Neb should clean up and exit.

```json
→ {"jsonrpc":"2.0","method":"shutdown","params":{},"id":4}
← {"jsonrpc":"2.0","result":{"status":"ok"},"id":4}
```

## Error Responses

```json
← {"jsonrpc":"2.0","error":{"code":-32601,"message":"unknown method"},"id":5}
```

## Idempotency Requirement

Nile provides **at-least-once delivery**. If the runtime crashes between receiving the neb's response and persisting the cursor, the message is redelivered on restart. **Nebs must be idempotent.** This is a deliberate tradeoff -- exactly-once would require 2PC.

## Retry and Dead Letter

If the neb returns an error or the transport fails, the runtime retries up to `maxRetries` (default: 3) with exponential backoff (100ms, 200ms, 400ms). After exhausting retries, the message is written to `dead/dead.wal` and the cursor advances.

## Message Envelope (Phase 2)

Messages can be wrapped in an extensible envelope:

```json
{
  "id": "msg-uuid",
  "sender": "copt-name",
  "type": "default",
  "payload": "...",
  "metadata": {
    "trace_id": "abc123",
    "created_at": "2026-02-20T00:00:00Z"
  }
}
```

The `type` field enables routing. The `metadata` map is open-ended. Nebs ignore fields they don't understand.
