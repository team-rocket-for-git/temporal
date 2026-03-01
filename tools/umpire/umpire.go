package umpire

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/tools/bats/lineup"
	"go.temporal.io/server/tools/umpire/rulebook"
	"go.temporal.io/server/tools/umpire/rulebook/rules"
	"go.temporal.io/server/tools/umpire/rulebook/rules/workflow_update"
	rulebooktypes "go.temporal.io/server/tools/umpire/rulebook/types"
	"go.temporal.io/server/tools/umpire/scorebook"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

// Global umpire instance (only active in tests)
var (
	globalUmpire *Umpire
	umpireMu     sync.RWMutex
)

// Umpire provides an in-memory telemetry verification engine that can be
// embedded into tests or used as a standalone service. It receives OTLP traces,
// converts them to events, routes them to entities, and runs pluggable verification
// models that query entities and return violations when invariants are violated.
type Umpire struct {
	logger   log.Logger
	registry *lineup.Registry
	importer *scorebook.Importer
	rulebook *rulebook.Rulebook
	scorebook *scorebook.Scorebook
}

// Config holds configuration for an Umpire instance.
type Config struct {
	// Logger for umpire output.
	Logger log.Logger
}

// New creates a new Umpire instance with the given configuration.
func New(cfg Config) (*Umpire, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("umpire: logger is required")
	}

	// Initialize registry with in-memory storage.
	registry, err := lineup.NewEntityRegistry(cfg.Logger, "")
	if err != nil {
		return nil, fmt.Errorf("umpire: failed to create registry: %w", err)
	}

	// TODO: Register default entities
	// lineup.RegisterDefaultEntities(registry)

	// Initialize importer.
	importer := scorebook.NewImporter()

	// Initialize scorebook for test queries.
	sb := scorebook.NewScorebook()

	// Initialize rulebook.
	rb := rulebook.NewRulebook()

	// Register all available models
	rb.Register("losttask", func() rulebook.Model { return &rules.LostTaskModel{} })
	rb.Register("stuckworkflow", func() rulebook.Model { return &rules.StuckWorkflowModel{} })
	rb.Register("retryresilience", func() rulebook.Model { return &rules.RetryResilienceModel{} })
	rb.Register("taskdeliveryguarantees", func() rulebook.Model { return &rules.TaskDeliveryGuaranteesModel{} })
	rb.Register("workflowlifecycleinvariants", func() rulebook.Model { return &rules.WorkflowLifecycleInvariantsModel{} })

	// Register workflow update models
	rb.Register("updatededuplication", func() rulebook.Model { return &workflow_update.UpdateDeduplicationModel{} })
	rb.Register("updatecompletion", func() rulebook.Model { return &workflow_update.UpdateCompletionModel{} })
	rb.Register("speculativetaskrollback", func() rulebook.Model { return &workflow_update.SpeculativeTaskRollbackModel{} })
	rb.Register("updatelossprevention", func() rulebook.Model { return &workflow_update.UpdateLossPreventionModel{} })
	rb.Register("updatestateconsistency", func() rulebook.Model { return &workflow_update.UpdateStateConsistencyModel{} })

	// Initialize all registered models.
	if err := rb.InitModels(context.Background(), nil, rulebooktypes.Deps{
		Registry: registry,
		Logger:   cfg.Logger,
	}); err != nil {
		return nil, fmt.Errorf("umpire: failed to initialize models: %w", err)
	}

	cfg.Logger.Info("umpire initialized",
		tag.NewInt("numModels", len(rb.Models())),
	)

	return &Umpire{
		logger:   cfg.Logger,
		registry: registry,
		importer: importer,
		rulebook: rb,
		scorebook: sb,
	}, nil
}

// Close releases resources held by the umpire.
func (w *Umpire) Close() error {
	// Close models.
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()

	if err := w.rulebook.Close(closeCtx); err != nil {
		w.logger.Warn("umpire: error closing models", tag.Error(err))
	}

	// Close registry and database.
	if err := w.registry.Close(); err != nil {
		w.logger.Warn("umpire: error closing registry", tag.Error(err))
	}

	w.logger.Info("umpire closed")
	return nil
}

// AddTraces adds a batch of traces to the umpire. It converts spans to events
// and routes them to entities.
// NOTE: RPC calls are now recorded directly via the gRPC interceptor (RecordMove).
// This method is primarily for non-RPC OTEL events like "workflow locked/unlocked".
func (w *Umpire) AddTraces(ctx context.Context, td ptrace.Traces) error {
	// Create an iterator that yields all spans in the trace batch.
	spanIter := func(yield func(ptrace.Span) bool) {
		resourceSpans := td.ResourceSpans()
		for i := 0; i < resourceSpans.Len(); i++ {
			rs := resourceSpans.At(i)
			scopeSpans := rs.ScopeSpans()
			for j := 0; j < scopeSpans.Len(); j++ {
				ss := scopeSpans.At(j)
				spans := ss.Spans()
				for k := 0; k < spans.Len(); k++ {
					span := spans.At(k)
					w.logger.Debug("umpire: processing span",
						tag.NewStringTag("spanName", span.Name()),
						tag.NewStringTag("spanKind", span.Kind().String()),
						tag.NewStringTag("traceID", span.TraceID().String()),
						tag.NewStringTag("spanID", span.SpanID().String()),
					)
					if !yield(span) {
						return
					}
				}
			}
		}
	}

	// Import spans to events.
	events := w.importer.ImportSpans(spanIter)
	w.logger.Debug("umpire: imported events", tag.NewInt("numEvents", len(events)))

	// NOTE: We don't add RPC moves to history here anymore - they're recorded
	// directly by the gRPC interceptor via RecordMove for immediate availability.
	// OTEL events are still routed to entities for verification models.

	// Route events to entities.
	if err := w.registry.RouteEvents(ctx, events); err != nil {
		w.logger.Warn("umpire: failed to route events", tag.Error(err))
	}

	return nil
}

// Check runs all model Check() methods and returns detected violations.
// This should be called explicitly from tests to verify invariants.
func (w *Umpire) Check(ctx context.Context) []rulebook.Violation {
	return w.rulebook.Check(ctx)
}

// Scorebook returns the scorebook for querying moves in tests.
func (w *Umpire) Scorebook() *scorebook.Scorebook {
	return w.scorebook
}

// Registry returns the entity registry for querying entities in tests.
func (w *Umpire) Registry() *lineup.Registry {
	return w.registry
}

// RecordMove records a move from a gRPC interceptor.
// This is the primary way moves are recorded - directly from gRPC calls.
func (w *Umpire) RecordMove(ctx context.Context, request any) {
	// Convert the gRPC request to a move
	move := w.importer.ImportRequest(request)
	if move == nil {
		// No parser for this request type, skip
		return
	}

	// Add to scorebook for test querying
	w.scorebook.Add(move)

	// Route to entities for verification models
	if err := w.registry.RouteEvents(ctx, []scorebooktypes.Move{move}); err != nil {
		w.logger.Warn("umpire: failed to route move from gRPC", tag.Error(err))
	}
}

// Get returns the global umpire (nil if not in test mode)
func Get() *Umpire {
	umpireMu.RLock()
	defer umpireMu.RUnlock()
	return globalUmpire
}

// Set configures the global umpire (test setup only)
func Set(u *Umpire) {
	umpireMu.Lock()
	defer umpireMu.Unlock()
	globalUmpire = u
}
