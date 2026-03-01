package catcher

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// Catcher receives and processes OTEL trace data from scout.
// It implements the TraceHandler interface that scout reports to.
type Catcher struct {
	logger   log.Logger
	mu       sync.RWMutex
	traces   []ptrace.Traces
	handlers []TraceProcessor
}

// TraceProcessor processes received traces.
// Multiple processors can be registered to handle traces differently
// (e.g., umpire for validation, storage for persistence).
type TraceProcessor interface {
	ProcessTraces(ctx context.Context, traces ptrace.Traces) error
}

// Config holds configuration for the Catcher.
type Config struct {
	Logger log.Logger
}

// New creates a new Catcher instance.
func New(cfg Config) *Catcher {
	if cfg.Logger == nil {
		panic("logger is required")
	}

	return &Catcher{
		logger:   cfg.Logger,
		traces:   make([]ptrace.Traces, 0),
		handlers: make([]TraceProcessor, 0),
	}
}

// AddTraces implements scout.TraceHandler interface.
// This is the entry point for OTEL data from scout.
func (c *Catcher) AddTraces(ctx context.Context, traces ptrace.Traces) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store traces for debugging/inspection
	c.traces = append(c.traces, traces)

	spanCount := 0
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			spanCount += ss.Spans().Len()
		}
	}

	c.logger.Debug("Catcher received traces",
		tag.NewInt("span_count", spanCount))

	// Forward to all registered processors
	var errs []error
	for _, handler := range c.handlers {
		if err := handler.ProcessTraces(ctx, traces); err != nil {
			c.logger.Error("Trace processor failed",
				tag.Error(err))
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to process traces: %v", errs)
	}

	return nil
}

// RegisterProcessor registers a trace processor to receive traces.
func (c *Catcher) RegisterProcessor(processor TraceProcessor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, processor)
}

// GetTraces returns all traces received so far (for debugging/testing).
func (c *Catcher) GetTraces() []ptrace.Traces {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]ptrace.Traces(nil), c.traces...)
}

// Clear removes all stored traces (useful for testing).
func (c *Catcher) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.traces = c.traces[:0]
}
