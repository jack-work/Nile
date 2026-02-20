# CLI Reference

## `nile run <name>`

Run a copt: spawn the neb, process messages.

```bash
nile run my-copt \
  --binary ./my-service \
  --data-dir /var/lib/nile/my-copt \
  --max-messages 10000 \
  --max-bytes 10485760 \
  --segment-size 1048576 \
  --message-timeout 60 \
  --max-retries 3 \
  --max-depth 0
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--binary` | (required) | Path to neb binary |
| `--data-dir` | `/var/lib/nile/<name>` | Data directory |
| `--max-messages` | 10000 | Retention: max consumed messages |
| `--max-bytes` | 10485760 | Retention: max log bytes |
| `--segment-size` | 1048576 | Bytes per WAL segment |
| `--message-timeout` | 60 | Neb response timeout (seconds) |
| `--max-retries` | 3 | Retries before dead-letter |
| `--max-depth` | 0 | Max unprocessed messages (0 = unlimited) |

## `nile install <name>`

Generate a systemd user unit and print enable instructions.

```bash
nile install my-copt \
  --binary ./my-service \
  --max-messages 10000
```

Creates `~/.config/systemd/user/nile-my-copt.service` and prints:

```
systemctl --user daemon-reload
systemctl --user enable --now nile-my-copt.service
```

## `nile status <name>`

Show copt status (running/stopped), PID, WAL info.

```bash
nile status my-copt
```

Output:

```
copt my-copt: running (pid 12345)
  next index: 42
  depth: 3
  total bytes: 4096
  dead letters: 1
```

## Examples

### Dev: run standalone

```bash
nile run echo-test --binary ./examples/echo-service/echo.sh
```

### Production: install as systemd service

```bash
nile install counter --binary /opt/counter-service --max-messages 5000
systemctl --user daemon-reload
systemctl --user enable --now nile-counter.service
```
