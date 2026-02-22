// Package lifecycle implements the copt state machine and message pump.
package lifecycle

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	nileotel "github.com/gluck/nile/pkg/otel"
	"github.com/gluck/nile/pkg/protocol"
	"github.com/gluck/nile/pkg/store"
	"github.com/gluck/nile/pkg/transport"
)

var tracer = otel.Tracer("github.com/gluck/nile/pkg/lifecycle")

// Manager runs the copt lifecycle: initialization, message pump, retention, shutdown.
type Manager struct {
	name      string
	dataDir   string
	store     store.Store
	transport transport.Transport
	logger    *slog.Logger
	metrics   *nileotel.Metrics

	mu    sync.Mutex
	state State
	reqID uint64

	// stopCh signals the pump loop to stop
	stopCh   chan struct{}
	stopOnce sync.Once
	// doneCh is closed when the pump loop exits
	doneCh chan struct{}

	// Configuration
	PollInterval   time.Duration
	MessageTimeout time.Duration
	MaxRetries     int
}

// Config holds configuration for the lifecycle manager.
type Config struct {
	Name           string
	DataDir        string
	Store          store.Store
	Transport      transport.Transport
	Logger         *slog.Logger
	Metrics        *nileotel.Metrics
	MessageTimeout time.Duration // default: 60s
	MaxRetries     int           // default: 3
}

// New creates a new lifecycle manager.
func New(cfg Config) *Manager {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}

	timeout := cfg.MessageTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	retries := cfg.MaxRetries
	if retries == 0 {
		retries = 3
	}

	return &Manager{
		name:           cfg.Name,
		dataDir:        cfg.DataDir,
		store:          cfg.Store,
		transport:      cfg.Transport,
		logger:         logger,
		metrics:        cfg.Metrics,
		state:          StateCreated,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		PollInterval:   100 * time.Millisecond,
		MessageTimeout: timeout,
		MaxRetries:     retries,
	}
}

// State returns the current lifecycle state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// transition attempts a state change. Returns an error if invalid.
func (m *Manager) transition(to State) error {
	if !canTransition(m.state, to) {
		return fmt.Errorf("lifecycle: invalid transition %s -> %s", m.state, to)
	}
	m.logger.Info("state transition", "from", m.state.String(), "to", to.String())
	m.state = to
	return nil
}

func (m *Manager) nextID() uint64 {
	m.reqID++
	return m.reqID
}

// send builds and sends a JSON-RPC request, returning the parsed status result.
func (m *Manager) send(method string, params interface{}) (*protocol.StatusResult, error) {
	req, err := protocol.NewRequest(m.nextID(), method, params)
	if err != nil {
		return nil, err
	}

	resp, err := m.transport.Send(req)
	if err != nil {
		return nil, err
	}

	var result protocol.StatusResult
	if err := resp.ParseResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Start initializes the neb and runs the message pump. Blocks until Stop is called or an error occurs.
func (m *Manager) Start() error {
	ctx := context.Background()

	// Dear Agent, Maybe should go into it's own scope so we can use defer
	m.mu.Lock()
	if err := m.transition(StateStarting); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	_, span := tracer.Start(ctx, "neb.init", trace.WithAttributes(
		attribute.String("copt.name", m.name),
	))

	_, err := m.send(protocol.MethodInit, protocol.InitParams{Name: m.name})
	if err != nil {
		span.RecordError(err)
		span.End()
		m.mu.Lock()
		m.transition(StateFailed)
		m.mu.Unlock()
		return fmt.Errorf("lifecycle: init failed: %w", err)
	}
	span.End()
	m.logger.Info("neb init complete, starting pump")

	// Dear Agent, same here as above with the defer
	m.mu.Lock()
	if err := m.transition(StateIdle); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	return m.pump()
}

// Stop signals the pump to stop gracefully. Safe to call multiple times
// from multiple goroutines.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// Wait blocks until the pump loop has exited.
func (m *Manager) Wait() {
	<-m.doneCh
}

// pump is the core message processing loop.
func (m *Manager) pump() error {
	defer close(m.doneCh)

	ticker := time.NewTicker(m.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return m.shutdown()
		default:
		}

		m.mu.Lock()
		currentState := m.state
		m.mu.Unlock()

		if currentState != StateIdle {
			select {
			case <-m.stopCh:
				return m.shutdown()
			case <-ticker.C:
			}
			continue
		}

		offset, payload, err := m.store.NextUnprocessed()
		if err != nil && err != store.ErrNoMessages {
			m.logger.Error("failed to read next message", "error", err)
		}
		if err == store.ErrNoMessages {
			if m.store.RetentionExceeded() {
				if err := m.doRetention(); err != nil {
					return fmt.Errorf("lifecycle: retention: %w", err)
				}
			}

			// Record stream gauges while idle
			if m.metrics != nil {
				ctx := context.Background()
				m.metrics.RecordDepth(ctx, int64(m.store.Depth()))
				m.metrics.RecordBytes(ctx, m.store.TotalBytes())
			}

			select {
			case <-m.stopCh:
				return m.shutdown()
			case <-ticker.C:
				continue
			}
		}
		if err != nil {
			return fmt.Errorf("lifecycle: read message: %w", err)
		}

		m.logger.Info("dequeued message", "offset", offset, "payload_bytes", len(payload))

		if err := m.processMessage(offset, payload); err != nil {
			return fmt.Errorf("lifecycle: process message: %w", err)
		}

		if m.store.RetentionExceeded() {
			if err := m.doRetention(); err != nil {
				return fmt.Errorf("lifecycle: retention: %w", err)
			}
		}
	}
}

// processMessage sends a message to the neb with retry logic.
// On exhausting retries, the message is dead-lettered.
func (m *Manager) processMessage(offset uint64, payload []byte) error {
	ctx := context.Background()
	ctx, span := tracer.Start(ctx, "message.process", trace.WithAttributes(
		attribute.String("copt.name", m.name),
		attribute.Int64("message.offset", int64(offset)),
		attribute.Int("message.payload_bytes", len(payload)),
	))
	defer span.End()

	m.mu.Lock()
	if err := m.transition(StateProcessing); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	encoded := base64.StdEncoding.EncodeToString(payload)

	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= m.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms...
			backoff := time.Duration(100<<uint(attempt-1)) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-m.stopCh:
				m.mu.Lock()
				m.transition(StateIdle)
				m.mu.Unlock()
				return nil
			}
			m.logger.Warn("retrying message", "offset", offset, "attempt", attempt, "last_error", lastErr)
		}

		m.logger.Debug("sending message to neb", "offset", offset, "attempt", attempt, "data_b64", encoded)

		result, err := m.send(protocol.MethodMessage, protocol.MessageParams{
			Offset: offset,
			Data:   encoded,
		})
		if err != nil {
			lastErr = err
			m.logger.Warn("neb send failed", "offset", offset, "attempt", attempt, "error", err)
			continue
		}

		// Success
		elapsed := time.Since(start)
		m.logger.Info("message processed", "offset", offset, "status", result.Status, "elapsed_ms", elapsed.Milliseconds())
		span.SetAttributes(
			attribute.String("neb.status", result.Status),
			attribute.Int64("message.elapsed_ms", elapsed.Milliseconds()),
		)
		if m.metrics != nil {
			m.metrics.RecordProcessed(ctx)
			m.metrics.RecordDuration(ctx, float64(elapsed.Milliseconds()))
		}

		if err := m.store.MarkProcessed(offset); err != nil {
			return err
		}
		m.logger.Debug("cursor advanced", "offset", offset, "consumed", offset+1)

		if result.PostProcess {
			m.mu.Lock()
			m.transition(StatePostProcessing)
			m.mu.Unlock()

			if err := m.store.MarkPostProcessed(offset); err != nil {
				return err
			}
		}

		m.mu.Lock()
		m.transition(StateIdle)
		m.mu.Unlock()

		return nil
	}

	// Exhausted retries: dead-letter the message
	span.RecordError(lastErr)
	m.logger.Error("dead-lettering message", "offset", offset, "error", lastErr, "retries", m.MaxRetries)

	if m.metrics != nil {
		m.metrics.RecordDeadLettered(ctx)
	}

	if err := m.store.DeadLetter(offset, payload); err != nil {
		return fmt.Errorf("dead letter: %w", err)
	}

	m.mu.Lock()
	m.transition(StateIdle)
	m.mu.Unlock()

	return nil
}

// doRetention performs the drain -> snapshot -> retain -> truncate cycle.
func (m *Manager) doRetention() error {
	ctx := context.Background()
	_, span := tracer.Start(ctx, "retention.cycle", trace.WithAttributes(
		attribute.String("copt.name", m.name),
	))
	defer span.End()

	m.mu.Lock()
	if err := m.transition(StateDraining); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	m.logger.Info("retention: starting snapshot")

	retainDir := filepath.Join(m.dataDir, "retain")
	snapPath := filepath.Join(retainDir, fmt.Sprintf("snap-%d.wal", time.Now().UnixNano()))

	if err := m.store.Snapshot(snapPath); err != nil {
		span.RecordError(err)
		return fmt.Errorf("snapshot: %w", err)
	}

	m.mu.Lock()
	if err := m.transition(StateRetaining); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	m.logger.Info("retention: sending retain to neb")
	_, err := m.send(protocol.MethodRetain, protocol.RetainParams{Snapshot: snapPath})
	if err != nil {
		span.RecordError(err)
		m.mu.Lock()
		m.transition(StateFailed)
		m.mu.Unlock()
		return fmt.Errorf("retain call: %w", err)
	}

	if err := m.store.Truncate(); err != nil {
		m.mu.Lock()
		m.transition(StateFailed)
		m.mu.Unlock()
		return fmt.Errorf("truncate: %w", err)
	}

	if m.metrics != nil {
		m.metrics.RecordRetention(ctx)
	}

	m.logger.Info("retention: complete, log truncated")

	m.mu.Lock()
	m.transition(StateIdle)
	m.mu.Unlock()

	return nil
}

// shutdown sends a shutdown request to the neb.
func (m *Manager) shutdown() error {
	m.mu.Lock()
	if m.state != StateIdle && m.state != StateStopping {
		m.state = StateStopping
	} else {
		m.transition(StateStopping)
	}
	m.mu.Unlock()

	m.logger.Info("shutting down neb")
	m.send(protocol.MethodShutdown, nil)

	m.mu.Lock()
	m.state = StateStopped
	m.mu.Unlock()

	return nil
}
