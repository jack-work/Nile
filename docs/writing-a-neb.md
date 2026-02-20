# Writing a Neb

A neb is any executable that speaks JSON-RPC 2.0 over stdin/stdout.

## Protocol

1. Read a JSON line from **stdin**
2. Parse the `method` field
3. Process the request
4. Write a JSON response to **stdout** (one line, newline-terminated)
5. Log to **stderr** (captured by journald)

## Environment Variables

| Variable | Description |
|----------|-------------|
| `NILE_STATE_DIR` | Read-write directory for persistent state |
| `NILE_RETAIN_DIR` | Directory containing retain snapshots |

## Bash Example

```bash
#!/usr/bin/env bash
while IFS= read -r line; do
  id=$(echo "$line" | jq -r '.id')
  method=$(echo "$line" | jq -r '.method // empty')

  case "$method" in
    init)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      ;;
    message)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      ;;
    retain)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      ;;
    shutdown)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      exit 0
      ;;
  esac
done
```

## Go Example

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type request struct {
    JSONRPC string          `json:"jsonrpc"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
    ID      uint64          `json:"id"`
}

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

    for scanner.Scan() {
        var req request
        json.Unmarshal(scanner.Bytes(), &req)

        switch req.Method {
        case "init", "message", "retain":
            respond(req.ID, map[string]string{"status": "ok"})
        case "shutdown":
            respond(req.ID, map[string]string{"status": "ok"})
            os.Exit(0)
        }
    }
}

func respond(id uint64, result interface{}) {
    resp := map[string]interface{}{
        "jsonrpc": "2.0",
        "result":  result,
        "id":      id,
    }
    data, _ := json.Marshal(resp)
    fmt.Println(string(data))
}
```

## Python Example

```python
#!/usr/bin/env python3
import json, sys

for line in sys.stdin:
    req = json.loads(line)
    resp = {"jsonrpc": "2.0", "result": {"status": "ok"}, "id": req["id"]}
    print(json.dumps(resp), flush=True)
    if req.get("method") == "shutdown":
        break
```

## Key Rules

1. **Be idempotent.** Messages may be redelivered after a crash.
2. **Respond to every request.** The runtime waits synchronously.
3. **Use `NILE_STATE_DIR` for state.** Other paths may be sandboxed.
4. **Don't access `stream/`.** The WAL is runtime-only.
5. **Exit on `shutdown`.** Clean up resources and exit.
6. **Set `post_process: true`** in the message response if you need the runtime to track post-processing progress.
