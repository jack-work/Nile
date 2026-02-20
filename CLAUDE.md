# Nile

Durable, sandboxed, message-driven services. Each deployment unit is a **copt** (Nile stream runtime + **neb** user service). The runtime manages an append-only WAL, enforces retention, and delivers messages to the neb one at a time over stdio JSON-RPC.

## Project layout

```
cmd/nile/          CLI: run, install, status
pkg/wal/           Segmented write-ahead log
pkg/transport/     Transport interface + stdio implementation
pkg/protocol/      JSON-RPC 2.0 message types
pkg/lifecycle/     State machine + message pump
pkg/sandbox/       Neb process sandboxing (Landlock stub)
modules/           NixOS module
templates/         Service template for `nix flake init`
examples/          Echo (bash) and counter (Go) nebs
```

## Build & test

```bash
go build ./cmd/nile/
go test ./...
```

## Note-keeping strategy

This project keeps **antipattern documentation inline with the code**. Each package that has known issues, design debts, or latent footguns maintains a `ANTIPATTERNS.md` file in its directory. These are living documents -- update them when:

- You discover a new antipattern or design concern
- You fix an existing antipattern (move it to a "Resolved" section with a brief note)
- You introduce a known shortcut that should be revisited later

The antipattern files serve three purposes:

1. **Onboarding**: new contributors immediately see what's fragile and why
2. **Prioritization**: the severity ratings help decide what to fix next
3. **Institutional memory**: prevent re-introducing patterns that were already identified as problematic

### Antipattern file format

```markdown
# Antipatterns: <package name>

## Active

### <Short title> [Severity: High/Medium/Low]
<Description of the problem, why it's bad, what the fix looks like.>

## Resolved

### <Short title> — resolved in <commit/date>
<What was done.>
```

### When working on this codebase

- Before modifying a package, **read its ANTIPATTERNS.md first**
- After making changes, check if any antipatterns were resolved or introduced
- Use the `/note` skill to quickly add observations during development
- Prefer fixing antipatterns opportunistically when working in the area rather than dedicated cleanup sprints

## Key design decisions

- **Stdio JSON-RPC**: zero-config, works with Landlock, natural backpressure
- **Polling not inotify**: simpler, portable -- documented as an antipattern to revisit
- **In-memory index**: fast reads at the cost of memory -- documented as a concern
- **Offsets reset on truncate**: not globally monotone -- by design for simplicity
