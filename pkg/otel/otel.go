// Package otel sets up OpenTelemetry tracing and metrics for a copt.
// Phase 1 uses file exporters (traces.jsonl, metrics.jsonl).
// Switching to OTLP gRPC exporters (Tempo, Prometheus) is a config change.
package otel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds setup parameters for the OTel providers.
type Config struct {
	CoptName string // copt name, used as service.name attribute
	DataDir  string // base data dir; telemetry/ subdir is created
}

// Providers holds the initialized OTel providers. Call Shutdown to flush and close.
type Providers struct {
	TracerProvider *trace.TracerProvider
	MeterProvider  *metric.MeterProvider

	traceFile  *os.File
	metricFile *os.File
}

// Setup initializes a TracerProvider and MeterProvider that write to
// <dataDir>/telemetry/traces.jsonl and metrics.jsonl.
func Setup(cfg Config) (*Providers, error) {
	telDir := filepath.Join(cfg.DataDir, "telemetry")
	if err := os.MkdirAll(telDir, 0755); err != nil {
		return nil, fmt.Errorf("otel: create telemetry dir: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("nile-"+cfg.CoptName),
			attribute.String("copt.name", cfg.CoptName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}

	// Traces
	traceFile, err := os.OpenFile(
		filepath.Join(telDir, "traces.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
	)
	if err != nil {
		return nil, fmt.Errorf("otel: open traces file: %w", err)
	}

	traceExp, err := stdouttrace.New(stdouttrace.WithWriter(traceFile))
	if err != nil {
		traceFile.Close()
		return nil, fmt.Errorf("otel: create trace exporter: %w", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
	)

	// Metrics
	metricFile, err := os.OpenFile(
		filepath.Join(telDir, "metrics.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
	)
	if err != nil {
		traceFile.Close()
		return nil, fmt.Errorf("otel: open metrics file: %w", err)
	}

	metricExp, err := stdoutmetric.New(stdoutmetric.WithWriter(metricFile))
	if err != nil {
		traceFile.Close()
		metricFile.Close()
		return nil, fmt.Errorf("otel: create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp)),
		metric.WithResource(res),
	)

	// Register globally
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	return &Providers{
		TracerProvider: tp,
		MeterProvider:  mp,
		traceFile:      traceFile,
		metricFile:     metricFile,
	}, nil
}

// Shutdown flushes exporters and closes files.
func (p *Providers) Shutdown(ctx context.Context) error {
	var firstErr error
	if err := p.TracerProvider.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.MeterProvider.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.traceFile.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.metricFile.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
