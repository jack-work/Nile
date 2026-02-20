package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/gluck/nile"

// Metrics holds all Nile metric instruments for a copt.
type Metrics struct {
	MessagesReceived     otelmetric.Int64Counter
	MessagesProcessed    otelmetric.Int64Counter
	MessagesDeadLettered otelmetric.Int64Counter
	MessageDurationMs    otelmetric.Float64Histogram
	StreamDepth          otelmetric.Int64Gauge
	StreamBytes          otelmetric.Int64Gauge
	RetentionTriggered   otelmetric.Int64Counter
}

// NewMetrics creates the metric instruments using the global MeterProvider.
func NewMetrics() *Metrics {
	meter := otel.Meter(instrumentationName)

	received, _ := meter.Int64Counter("nile.messages.received",
		otelmetric.WithDescription("Messages appended to WAL"),
	)
	processed, _ := meter.Int64Counter("nile.messages.processed",
		otelmetric.WithDescription("Messages consumed by neb"),
	)
	deadLettered, _ := meter.Int64Counter("nile.messages.dead_lettered",
		otelmetric.WithDescription("Messages sent to dead letter"),
	)
	durationMs, _ := meter.Float64Histogram("nile.message.duration_ms",
		otelmetric.WithDescription("Neb processing time in milliseconds"),
	)
	depth, _ := meter.Int64Gauge("nile.stream.depth",
		otelmetric.WithDescription("Unprocessed messages in WAL"),
	)
	streamBytes, _ := meter.Int64Gauge("nile.stream.bytes",
		otelmetric.WithDescription("Total WAL size on disk"),
	)
	retentionTriggered, _ := meter.Int64Counter("nile.retention.triggered",
		otelmetric.WithDescription("Retention cycles completed"),
	)

	return &Metrics{
		MessagesReceived:     received,
		MessagesProcessed:    processed,
		MessagesDeadLettered: deadLettered,
		MessageDurationMs:    durationMs,
		StreamDepth:          depth,
		StreamBytes:          streamBytes,
		RetentionTriggered:   retentionTriggered,
	}
}

// RecordReceived increments the messages received counter.
func (m *Metrics) RecordReceived(ctx context.Context) {
	m.MessagesReceived.Add(ctx, 1)
}

// RecordProcessed increments the messages processed counter.
func (m *Metrics) RecordProcessed(ctx context.Context) {
	m.MessagesProcessed.Add(ctx, 1)
}

// RecordDeadLettered increments the dead letter counter.
func (m *Metrics) RecordDeadLettered(ctx context.Context) {
	m.MessagesDeadLettered.Add(ctx, 1)
}

// RecordDuration records a message processing duration.
func (m *Metrics) RecordDuration(ctx context.Context, ms float64) {
	m.MessageDurationMs.Record(ctx, ms)
}

// RecordDepth records the current stream depth.
func (m *Metrics) RecordDepth(ctx context.Context, depth int64) {
	m.StreamDepth.Record(ctx, depth)
}

// RecordBytes records the current WAL size.
func (m *Metrics) RecordBytes(ctx context.Context, bytes int64) {
	m.StreamBytes.Record(ctx, bytes)
}

// RecordRetention increments the retention counter.
func (m *Metrics) RecordRetention(ctx context.Context) {
	m.RetentionTriggered.Add(ctx, 1)
}
