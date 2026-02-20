# Sandboxing

Nile uses Landlock (Linux LSM) to restrict neb filesystem access.

## Landlock Policy

The runtime applies Landlock restrictions before exec'ing the neb:

| Path | Access | Purpose |
|------|--------|---------|
| `/nix/store` | Read-only | Dependencies |
| `state/` | Read-write | Neb persistent state |
| `retain/` | Read-write | Snapshot files |
| `stream/` | **Not accessible** | WAL is runtime-only |

Uses `landlock.V5.BestEffort()` -- applies the strongest available restrictions without failing on older kernels. The effective Landlock ABI version is logged on startup.

## Why stdio

- Inherited from parent process (zero config)
- Works perfectly with Landlock (no socket paths to allow)
- Natural backpressure (pipe buffer)
- Simplest possible implementation

stdio file descriptors are inherited through exec -- Landlock doesn't restrict pre-opened fds.

## Environment

The neb runs with a minimal environment:

```
PATH=/usr/bin:/bin
NILE_STATE_DIR=/var/lib/nile/<name>/state
NILE_RETAIN_DIR=/var/lib/nile/<name>/retain
NILE_NETWORK=1  (if network access enabled)
```

## systemd Defense in Depth

The NixOS module adds systemd sandboxing alongside Landlock:

- `ProtectSystem=strict`
- `ProtectHome=true`
- `PrivateTmp=true`
- `NoNewPrivileges=true`
- Explicit `ReadWritePaths` and `ReadOnlyPaths`

## Extra Paths

Configure additional paths via the NixOS module:

```nix
services.nile.copts.my-service = {
  sandbox.extraReadPaths = [ "/etc/my-config" ];
  sandbox.extraWritePaths = [ "/var/log/my-service" ];
  sandbox.network = true;  # allow network sockets
};
```
