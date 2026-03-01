package bats

import (
	"context"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/telemetry"
	"go.temporal.io/server/tools/umpire/pitcher"
)

// TestBase provides CATCH integration for test suites.
// Embed this in your test suite to get CATCH functionality.
type TestBase struct {
	catch *Catch
}

// SetupCatch initializes the CATCH system for testing.
// Call this in your test suite's SetupSuite or SetupTest.
//
// Example:
//
//	func (s *MyTestSuite) SetupSuite() {
//	    s.SetupCatch(s.Logger, catch.Config{
//	        EnableScout: true,
//	        EnableUmpire: true,
//	        EnablePitcher: true,
//	    })
//	}
func (tb *TestBase) SetupCatch(logger log.Logger, cfg Config) error {
	cfg.Logger = logger
	var err error
	tb.catch, err = New(cfg)
	return err
}

// GetCatch returns the CATCH instance.
func (tb *TestBase) GetCatch() *Catch {
	return tb.catch
}

// GetUmpire returns the Umpire instance.
func (tb *TestBase) GetUmpire() interface{} {
	if tb.catch == nil {
		return nil
	}
	return tb.catch.Umpire()
}

// GetPitcher returns the Pitcher instance.
func (tb *TestBase) GetPitcher() pitcher.Pitcher {
	if tb.catch == nil {
		return nil
	}
	return tb.catch.Pitcher()
}

// ConfigurePitcher configures pitcher with a play configuration.
// This is a convenience method for test setup.
func (tb *TestBase) ConfigurePitcher(targetType any, config pitcher.PlayConfig) {
	if tb.catch == nil || tb.catch.Pitcher() == nil {
		return
	}
	tb.catch.Pitcher().Configure(targetType, config)
}

// GetSpanExporters returns span exporters for test cluster configuration.
func (tb *TestBase) GetSpanExporters() map[telemetry.SpanExporterType]sdktrace.SpanExporter {
	if tb.catch == nil {
		return nil
	}
	return tb.catch.GetSpanExporters()
}

// CheckViolations validates properties and returns violations.
// Call this in TearDownTest to check for property violations.
func (tb *TestBase) CheckViolations(ctx context.Context) []interface{} {
	if tb.catch == nil {
		return nil
	}
	// Convert to generic slice for easier use
	violations := tb.catch.Check(ctx)
	result := make([]interface{}, len(violations))
	for i, v := range violations {
		result[i] = v
	}
	return result
}

// ResetCatch clears all CATCH state between tests.
// Call this in TearDownTest.
func (tb *TestBase) ResetCatch() {
	if tb.catch != nil {
		tb.catch.Reset()
	}
}

// ShutdownCatch cleanly shuts down CATCH.
// Call this in TearDownSuite.
func (tb *TestBase) ShutdownCatch(ctx context.Context) error {
	if tb.catch != nil {
		return tb.catch.Shutdown(ctx)
	}
	return nil
}

// WithCatch is a convenience function that sets up CATCH with default config.
// Returns span exporters that should be added to test cluster config.
//
// Example:
//
//	func (s *MyTestSuite) SetupSuite() {
//	    exporters := catch.WithCatch(s.Logger)
//	    s.testClusterConfig.SpanExporters = exporters
//	}
func WithCatch(logger log.Logger) map[telemetry.SpanExporterType]sdktrace.SpanExporter {
	c, err := New(Config{
		Logger:        logger,
		EnableScout:   true,
		EnableUmpire:  true,
		EnablePitcher: true,
	})
	if err != nil {
		panic(err)
	}
	return c.GetSpanExporters()
}
