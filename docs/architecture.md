# Architecture

Nile is a framework for **durable, sandboxed, message-driven services**.

## Terminology

| Term | Meaning |
|------|---------|
| **Copt** | Deployment unit: a Nile stream runtime + its neb |
| **Neb** | The user's service process (implements `message()` + `retain()`) |
| **Divan** | Centralized service registry for copt discovery (Phase 2) |
| **Scattercast** | Decentralized broadcast discovery between copts (Phase 2) |

## Single Copt (Phase 1)

```
┌─── copt ──────────────────────────────────────┐
│                                               │
│  Nile Runtime (Go)           Neb (any lang)   │
│  ┌─────────────────┐        ┌─────────────┐  │
│  │ Append-Only WAL │        │ user code   │  │
│  │ - segments      │ stdio  │ message()   │  │
│  │ - retention     │◄──────►│ retain()    │  │
│  │ - snapshots     │ jsonrpc│             │  │
│  │                 │        │ (sandboxed  │  │
│  │ Lifecycle Mgr   │        │  via        │  │
│  │ - state machine │        │  Landlock)  │  │
│  │ - msg dispatch  │        │             │  │
│  └─────────────────┘        └─────────────┘  │
│                                               │
└───────────────────────────────────────────────┘
```

The runtime manages an append-only WAL, enforces retention, and delivers messages to the neb one at a time over stdio (JSON-RPC 2.0). The neb processes messages sequentially and can persist state.

## Inter-Copt Communication (Phase 2)

```
┌─── copt A ────┐         ┌─── copt B ────┐
│ runtime │ neb │  HTTP   │ runtime │ neb │
│         │  ───┼────────►│ (WAL)   │     │
└─────────┴─────┘         └─────────┴─────┘
        ▲                         ▲
        └──── divan / scattercast ─────┘
```

Neb A posts via HTTP to B's runtime, which appends to B's WAL. B's neb receives messages in order. Discovery via divan (centralized) or scattercast (decentralized).

## Technology

- **Core**: Go
- **Services**: Polyglot (any language that reads stdin/writes stdout)
- **Sandboxing**: Landlock (Linux LSM)
- **Deployment**: systemd user units, NixOS module, standalone binary
- **Observability**: OpenTelemetry (traces + metrics to file, OTLP-ready)

## Package Layout

```
pkg/wal/          Segmented append-only log with CRC32, snapshots, dead letters
pkg/transport/    Transport interface + stdio implementation
pkg/protocol/     JSON-RPC 2.0 types + message envelope
pkg/lifecycle/    State machine + message pump
pkg/sandbox/      Landlock wrapper for neb process
pkg/otel/         OpenTelemetry setup (traces, metrics)
pkg/registry/     Discovery interface + file-based registry
cmd/nile/         Runtime binary (run, install, status)
```
