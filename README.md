# Nile

Durable, sandboxed, message-driven services.

Each deployment unit is a **copt** — a Nile stream runtime paired with a **neb** (your service). The runtime manages an append-only WAL, enforces retention, and delivers messages to the neb one at a time over stdio JSON-RPC. The neb implements `message()` and `retain()` in any language.

```
┌─── copt ──────────────────────────────────────┐
│                                               │
│  Nile Runtime (Go)           Neb (any lang)   │
│  ┌─────────────────┐        ┌─────────────┐  │
│  │ Append-Only WAL │        │ user code   │  │
│  │ - segments      │ stdio  │ message()   │  │
│  │ - retention     │◄──────►│ retain()    │  │
│  │ - snapshots     │ jsonrpc│             │  │
│  │                 │        │             │  │
│  │ Lifecycle Mgr   │        │ (sandboxed  │  │
│  │ - state machine │        │  via        │  │
│  │ - msg dispatch  │        │  Landlock)  │  │
│  └─────────────────┘        └─────────────┘  │
│                                               │
└───────────────────────────────────────────────┘
```

## Why

Services shouldn't worry about message ordering, delivery, persistence, or isolation. Nile handles all of that so your service is just a function: receive a message, do work, optionally persist state.

- **Durable**: Messages are written to a segmented, CRC32-checksummed WAL with `fdatasync`. They survive crashes.
- **Ordered**: One message at a time, in order. No concurrency in the neb.
- **Sandboxed**: Nebs run under Landlock — they can access their state directory but not the WAL or the rest of the filesystem.
- **Polyglot**: Nebs communicate over stdin/stdout JSON-RPC. Write them in anything.
- **Observable**: OpenTelemetry traces and metrics from day one.

## Quick start

```bash
# Enter the dev shell (requires Nix)
nix develop

# Run the interactive demo
nile-demo start
nile-demo send "hello world"
nile-demo output       # tail the neb's activity log
nile-demo log          # tail the WAL state
nile-demo runtime-log  # tail runtime stderr (JSON logs)
nile-demo reset        # stop + clear all data
```

Or run directly:

```bash
go build ./cmd/nile/
go build ./examples/counter-service/

./nile run my-copt --binary ./counter-service --data-dir /tmp/my-copt
# In another terminal:
./nile send --data-dir /tmp/my-copt my-copt "hello"
```

## Project layout

```
cmd/nile/              CLI: run, send, watch, install, status
pkg/wal/               Segmented write-ahead log
pkg/transport/         Transport interface + stdio implementation
pkg/protocol/          JSON-RPC 2.0 message types
pkg/lifecycle/         State machine + message pump
pkg/sandbox/           Neb process sandboxing (Landlock)
pkg/otel/              OpenTelemetry traces + metrics
examples/              Reference nebs (Go counter-service)
modules/               NixOS module
docs/                  Architecture, protocol spec, guides
```

## Phase 2 (planned)

Inter-copt communication: nebs post messages to other copts over HTTP, routed through the target's WAL. Discovery via a centralized **divan** or decentralized **scattercast**.

## Docs

See [`docs/`](docs/) for architecture, the neb protocol spec, WAL format, sandboxing, and a guide to writing nebs.
