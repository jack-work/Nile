# Observability

Nile integrates OpenTelemetry for traces and metrics from day one.

## Setup

The runtime initializes a `TracerProvider` and `MeterProvider` that write to:

- `<data-dir>/telemetry/traces.jsonl` (one span per line)
- `<data-dir>/telemetry/metrics.jsonl`

Logs go to stderr via `log/slog` with JSON formatting, captured by systemd journal.

## Traces

| Span | Attributes | Description |
|------|------------|-------------|
| `neb.init` | `copt.name` | Neb initialization handshake |
| `message.process` | `copt.name`, `message.offset` | Message delivery + neb processing |
| `retention.cycle` | `copt.name` | Full drain -> snapshot -> retain -> truncate |

Trace context (`trace_id`) is propagated through the message envelope `metadata` field for cross-copt tracing in Phase 2.

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nile.messages.received` | Counter | Messages appended to WAL |
| `nile.messages.processed` | Counter | Messages consumed by neb |
| `nile.messages.dead_lettered` | Counter | Messages sent to dead letter |
| `nile.message.duration_ms` | Histogram | Neb processing time |
| `nile.stream.depth` | Gauge | Unprocessed messages in WAL |
| `nile.stream.bytes` | Gauge | Total WAL size on disk |
| `nile.retention.triggered` | Counter | Retention cycles completed |

## Logs

`log/slog` with JSON handler. Trace context (`trace_id`, `span_id`) injected via OTel bridge. All lifecycle transitions, errors, and retries are logged.

## Future: OTLP Stack

Switch exporters via configuration (no code change):

```
--otel-endpoint grpc://localhost:4317
```

- **Tempo** for traces
- **Prometheus** for metrics
- **Loki** for logs (via Promtail or OTLP log exporter)
- **Grafana** for dashboards
