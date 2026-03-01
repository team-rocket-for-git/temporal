// Package pitcher provides fault injection capabilities for tests.
// It follows the interceptor pattern to inject failures, delays, and other
// chaos into RPC calls and persistence operations during testing.
//
// Pitcher is only active during tests and uses a global state pattern
// similar to testhooks.TestHooks.
package pitcher

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"time"

	sdkclient "go.temporal.io/sdk/client"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Pitcher makes plays (executes atomic actions like delay, fail, cancel).
// It is thread-safe and designed to be used as a global singleton.
type Pitcher interface {
	// MakePlay executes a play if matching criteria are met.
	// Returns the play that was made (for scorebook recording) and error to inject.
	// The targetType is a proto message type (e.g., &matchingservice.AddWorkflowTaskRequest{})
	// used to identify which RPC method is being intercepted.
	// The request is the actual gRPC request for matching criteria.
	// If a play matches, it is executed once and then deleted.
	MakePlay(ctx context.Context, targetType any, request any) (*Play, error)

	// Configure sets play configuration for a target RPC method.
	// The targetType should be a proto message type (e.g., &matchingservice.AddWorkflowTaskRequest{}).
	// Multiple plays can be configured for the same target - they will be matched in order.
	// Game configures which plays to make where using MatchCriteria.
	Configure(targetType any, config PlayConfig)

	// Execute runs a complete scenario play.
	// It sets up all faults, starts the workflow, and executes pitches sequentially.
	Execute(ctx context.Context, play *ScenarioPlay) (WorkflowRun, error)

	// Reset clears all configurations.
	Reset()
}

// WorkflowRun represents a running workflow execution
type WorkflowRun interface {
	// GetID returns the workflow ID
	GetID() string
	// GetRunID returns the workflow run ID
	GetRunID() string
	// Get waits for the workflow to complete and returns the result
	Get(ctx context.Context, valuePtr interface{}) error
}

// PlayConfig defines when to make a play.
// Each play matches ONCE and is then deleted. To match multiple times,
// configure multiple plays.
type PlayConfig struct {
	// Play is the action to take (delay, fail, timeout, etc.)
	Play Play

	// Match contains criteria for determining if this play applies to the current request.
	// The interceptor checks these fields against the request/context.
	// If all specified matchers match, the play is made and then deleted.
	// Empty/nil means match any request.
	Match *MatchCriteria
}

// MatchCriteria defines conditions that must be met for a play to apply.
// If the request matches any of the specified entity IDs, the criteria matches.
type MatchCriteria struct {
	// Entities is a list of entity IDs to match against.
	// If any entity ID in the list matches the request, the criteria matches (OR logic).
	// Empty list means match all requests.
	Entities []lineuptypes.EntityID
}

// Common error codes for fault injection
const (
	ErrorResourceExhausted = "RESOURCE_EXHAUSTED"
	ErrorDeadlineExceeded  = "DEADLINE_EXCEEDED"
	ErrorUnavailable       = "UNAVAILABLE"
	ErrorInternal          = "INTERNAL"
	ErrorCanceled          = "CANCELED"
	ErrorAborted           = "ABORTED"
)

// DelayParams specifies delay configuration
type DelayParams struct {
	Duration time.Duration // How long to delay
	Jitter   float64       // Random jitter (0.0-1.0)
}

// pitcherImpl implements the Pitcher interface
type pitcherImpl struct {
	mu      sync.RWMutex
	configs map[string][]*playState // target -> list of plays (matched in order)
	rand    *rand.Rand
}

// playState tracks execution state for a configured play
type playState struct {
	config PlayConfig
	mu     sync.Mutex
}

// Global pitcher instance (only active in tests)
var (
	globalPitcher Pitcher
	pitcherMu     sync.RWMutex
)

// New creates a new Pitcher instance
func New() Pitcher {
	return &pitcherImpl{
		configs: make(map[string][]*playState),
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Get returns the global pitcher (nil if not in test mode)
func Get() Pitcher {
	pitcherMu.RLock()
	defer pitcherMu.RUnlock()
	return globalPitcher
}

// Set configures the global pitcher (test setup only)
func Set(p Pitcher) {
	pitcherMu.Lock()
	defer pitcherMu.Unlock()
	globalPitcher = p
}

// getTypeName extracts the fully qualified type name from a proto message.
// For example, *matchingservice.AddWorkflowTaskRequest -> "go.temporal.io/api/workflowservice/v1.AddWorkflowTaskRequest"
func getTypeName(targetType any) string {
	if targetType == nil {
		return ""
	}
	t := reflect.TypeOf(targetType)
	// Handle pointers
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// Return the full package path + type name
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.Name()
}

// MakePlay implements Pitcher.MakePlay
func (p *pitcherImpl) MakePlay(ctx context.Context, targetType any, request any) (*Play, error) {
	target := getTypeName(targetType)
	if target == "" {
		return nil, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	states, ok := p.configs[target]
	if !ok || len(states) == 0 {
		return nil, nil // no configuration for this target
	}

	// Find the first matching play
	for i, state := range states {
		if p.matchesCriteria(state.config.Match, request) {
			// Found a match - execute it and remove it from the list
			config := state.config

			// Remove this play from the list (one-time use)
			p.configs[target] = append(states[:i], states[i+1:]...)

			// If no more plays for this target, remove the target entirely
			if len(p.configs[target]) == 0 {
				delete(p.configs, target)
			}

			// Execute the play and return it (for scorebook recording)
			return p.executePlay(ctx, target, config)
		}
	}

	return nil, nil // no matching play found
}

// matchesCriteria checks if the request matches the criteria
func (p *pitcherImpl) matchesCriteria(criteria *MatchCriteria, request any) bool {
	// If no criteria specified, match anything
	if criteria == nil {
		return true
	}

	// If entities list is empty, match anything
	if len(criteria.Entities) == 0 {
		return true
	}

	// TODO: Implement matching logic based on request type and entity IDs.
	// The interceptor will need to extract entity IDs from the gRPC request
	// and compare against the Entities list.
	// For example:
	// - Extract WorkflowID from request and create EntityID for Workflow entity
	// - Extract TaskQueue from request and create EntityID for TaskQueue entity
	// - Compare extracted EntityIDs against criteria.Entities list
	// This will be implemented when we add typed interceptors.
	return false
}

// executePlay performs the actual play execution
func (p *pitcherImpl) executePlay(ctx context.Context, target string, config PlayConfig) (*Play, error) {
	// Set the target on the play (for scorebook recording)
	playToMake := config.Play
	playToMake.Target = target
	playToMake.Timestamp = time.Now()

	var err error
	switch playToMake.Type {
	case PlayFail:
		err = p.executeFail(playToMake)
	case PlayDelay:
		err = p.executeDelay(ctx, playToMake)
	case PlayTimeout:
		err = p.executeTimeout(ctx, playToMake)
	case PlayCancel:
		err = context.Canceled
	case PlayDrop:
		err = fmt.Errorf("pitcher: connection dropped")
	default:
		err = fmt.Errorf("pitcher: unknown play type %q for target %q", playToMake.Type, target)
	}

	return &playToMake, err
}

// executeFail returns a gRPC error
func (p *pitcherImpl) executeFail(pl Play) error {
	// If an error instance is provided (e.g., serviceerror), convert it to gRPC status
	if err, ok := pl.Params[ParamError].(error); ok {
		// Check if error has a Status() method (serviceerror types)
		type statusProvider interface {
			Status() *status.Status
		}
		if sp, ok := err.(statusProvider); ok {
			return sp.Status().Err()
		}
		// Otherwise return as-is
		return err
	}
	// Fallback if no error is specified
	return status.Error(codes.Internal, "pitcher made play: no error specified")
}

// executeDelay sleeps for the configured duration
func (p *pitcherImpl) executeDelay(ctx context.Context, pl Play) error {
	duration, ok := pl.Params[ParamDuration].(time.Duration)
	if !ok {
		duration = 100 * time.Millisecond // default delay
	}

	// Apply jitter if specified
	if jitter, ok := pl.Params[ParamJitter].(float64); ok && jitter > 0 {
		jitterAmount := float64(duration) * jitter * (p.rand.Float64() - 0.5) * 2
		duration += time.Duration(jitterAmount)
	}

	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// executeTimeout returns a deadline exceeded error after a delay
func (p *pitcherImpl) executeTimeout(ctx context.Context, pl Play) error {
	// First delay, then return timeout error
	if err := p.executeDelay(ctx, pl); err != nil {
		return err
	}
	return status.Error(codes.DeadlineExceeded, "pitcher made play: timeout")
}

// Configure implements Pitcher.Configure
func (p *pitcherImpl) Configure(targetType any, config PlayConfig) {
	target := getTypeName(targetType)
	if target == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Append to the list of plays for this target
	p.configs[target] = append(p.configs[target], &playState{
		config: config,
	})
}

// Reset implements Pitcher.Reset
func (p *pitcherImpl) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.configs = make(map[string][]*playState)
}

// Execute runs a complete scenario play by executing all pitches sequentially.
// Phase 1: Setup all faults
// Phase 2: Start workflow (if StartWorkflowPitch exists)
// Phase 3: Execute remaining pitches sequentially
func (p *pitcherImpl) Execute(ctx context.Context, play *ScenarioPlay) (WorkflowRun, error) {
	if play == nil {
		return nil, fmt.Errorf("scenario play cannot be nil")
	}

	var workflowRun sdkclient.WorkflowRun

	// Phase 1: Setup all faults first
	for _, pitch := range play.Pitches {
		if faultPitch, ok := pitch.(*FaultPitch); ok {
			// Configure the fault for this target type
			p.Configure(faultPitch.Target, PlayConfig{
				Play:  faultPitch.Fault,
				Match: faultPitch.Match,
			})
		}
	}

	// Phase 2: Start workflow
	for _, pitch := range play.Pitches {
		if startPitch, ok := pitch.(*StartWorkflowPitch); ok {
			var err error
			workflowRun, err = startPitch.Client.ExecuteWorkflow(
				ctx,
				startPitch.Options,
				startPitch.Workflow,
				startPitch.WorkflowInput,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to start workflow: %w", err)
			}
			break // Only one workflow start per play
		}
	}

	// Phase 3: Execute remaining pitches (client actions, server actions)
	for _, pitch := range play.Pitches {
		switch p := pitch.(type) {
		case *ClientActionPitch:
			// Execute client action
			// TODO: Implement client action execution
			_ = p
		case *ServerActionPitch:
			// Server actions are handled by the interceptor
			// TODO: Implement manual gRPC advancement
			_ = p
		}
	}

	if workflowRun == nil {
		return nil, fmt.Errorf("no workflow was started in the scenario")
	}

	return &workflowRunWrapper{run: workflowRun}, nil
}

// workflowRunWrapper wraps sdk client WorkflowRun to implement our WorkflowRun interface
type workflowRunWrapper struct {
	run sdkclient.WorkflowRun
}

func (w *workflowRunWrapper) GetID() string {
	return w.run.GetID()
}

func (w *workflowRunWrapper) GetRunID() string {
	return w.run.GetRunID()
}

func (w *workflowRunWrapper) Get(ctx context.Context, valuePtr interface{}) error {
	return w.run.Get(ctx, valuePtr)
}

// parseErrorCode converts string error codes to gRPC codes
func parseErrorCode(errorCode string) codes.Code {
	switch errorCode {
	case ErrorResourceExhausted:
		return codes.ResourceExhausted
	case ErrorDeadlineExceeded:
		return codes.DeadlineExceeded
	case ErrorUnavailable:
		return codes.Unavailable
	case ErrorInternal:
		return codes.Internal
	case ErrorCanceled:
		return codes.Canceled
	case ErrorAborted:
		return codes.Aborted
	default:
		return codes.Internal
	}
}
