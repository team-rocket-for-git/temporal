package bats

import (
	"context"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/telemetry"
	"go.temporal.io/server/tools/umpire"
	"go.temporal.io/server/tools/umpire/catcher"
	"go.temporal.io/server/tools/umpire/pitcher"
)

// Catch is the complete CATCH system that brings together:
// - Scout: Observable event instrumentation (OTEL)
// - Catcher: Receives OTEL traces
// - Umpire: Validates properties
// - Pitcher: Fault injection
type Catch struct {
	logger   log.Logger
	catcher  *catcher.Catcher
	umpire   *umpire.Umpire
	pitcher  pitcher.Pitcher
	exporter sdktrace.SpanExporter
}

// Config holds configuration for the complete CATCH system.
type Config struct {
	Logger log.Logger

	// EnableScout enables OTEL instrumentation via scout
	EnableScout bool

	// EnableUmpire enables property validation
	EnableUmpire bool

	// EnablePitcher enables fault injection
	EnablePitcher bool
}

// New creates a new complete CATCH system with all components integrated.
// This is the recommended way to set up CATCH for testing.
func New(cfg Config) (*Catch, error) {
	if cfg.Logger == nil {
		panic("logger is required")
	}

	c := &Catch{
		logger: cfg.Logger,
	}

	// Initialize Catcher (always needed if Scout is enabled)
	if cfg.EnableScout {
		c.catcher = catcher.New(catcher.Config{
			Logger: cfg.Logger,
		})

		// Create Scout span exporter that reports to catcher
		c.exporter = NewSpanExporter(c.catcher)
	}

	// Initialize Umpire
	if cfg.EnableUmpire {
		var err error
		c.umpire, err = umpire.New(umpire.Config{
			Logger: cfg.Logger,
		})
		if err != nil {
			return nil, err
		}

		// Set global umpire for use in tests
		umpire.Set(c.umpire)
	}

	// Initialize Pitcher
	if cfg.EnablePitcher {
		c.pitcher = pitcher.New()
		// Set global pitcher for use in interceptors
		pitcher.Set(c.pitcher)
	}

	return c, nil
}

// SpanExporter returns the OTEL span exporter that should be registered
// with the test cluster's telemetry configuration.
func (c *Catch) SpanExporter() sdktrace.SpanExporter {
	return c.exporter
}

// Umpire returns the Umpire instance for property validation.
func (c *Catch) Umpire() *umpire.Umpire {
	return c.umpire
}

// Pitcher returns the Pitcher instance for fault injection.
func (c *Catch) Pitcher() pitcher.Pitcher {
	return c.pitcher
}

// Catcher returns the Catcher instance that receives OTEL traces.
func (c *Catch) Catcher() *catcher.Catcher {
	return c.catcher
}

// Check validates all registered properties and returns any violations.
// This should be called at the end of a test.
func (c *Catch) Check(ctx context.Context) []interface{} {
	if c.umpire == nil {
		return nil
	}
	violations := c.umpire.Check(ctx)
	// Convert to interface{} slice for easier use
	result := make([]interface{}, len(violations))
	for i, v := range violations {
		result[i] = v
	}
	return result
}

// Reset clears all state (catcher traces, pitcher config, etc.).
// Call this between tests to ensure isolation.
func (c *Catch) Reset() {
	if c.catcher != nil {
		c.catcher.Clear()
	}
	if c.pitcher != nil {
		c.pitcher.Reset()
	}
}

// Shutdown cleanly shuts down all CATCH components.
func (c *Catch) Shutdown(ctx context.Context) error {
	if c.exporter != nil {
		if err := c.exporter.Shutdown(ctx); err != nil {
			c.logger.Error("Failed to shutdown scout exporter", tag.Error(err))
		}
	}
	return nil
}

// GetSpanExporters returns a map of span exporters for test cluster configuration.
// This is a convenience method for FunctionalTestBase integration.
func (c *Catch) GetSpanExporters() map[telemetry.SpanExporterType]sdktrace.SpanExporter {
	if c.exporter == nil {
		return nil
	}
	return map[telemetry.SpanExporterType]sdktrace.SpanExporter{
		"catch": c.exporter,
	}
}
