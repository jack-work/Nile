#!/usr/bin/env bash
# echo-service: minimal Nile neb that acknowledges every message.
# Communicates via newline-delimited JSON-RPC 2.0 over stdin/stdout.

while IFS= read -r line; do
  id=$(echo "$line" | jq -r '.id')
  method=$(echo "$line" | jq -r '.method // empty')

  case "$method" in
    init)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      ;;
    message)
      # Echo: just acknowledge
      offset=$(echo "$line" | jq -r '.params.offset')
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      echo "echo-service: processed message at offset $offset" >&2
      ;;
    retain)
      snapshot=$(echo "$line" | jq -r '.params.snapshot')
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      echo "echo-service: retain snapshot at $snapshot" >&2
      ;;
    shutdown)
      echo "{\"jsonrpc\":\"2.0\",\"result\":{\"status\":\"ok\"},\"id\":$id}"
      echo "echo-service: shutting down" >&2
      exit 0
      ;;
    *)
      echo "{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32601,\"message\":\"unknown method\"},\"id\":$id}"
      ;;
  esac
done
