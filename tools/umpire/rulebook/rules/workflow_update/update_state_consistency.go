package workflow_update

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	entity "go.temporal.io/server/tools/bats/lineup/entities"
	"go.temporal.io/server/tools/umpire/rulebook"
	rulebooktypes "go.temporal.io/server/tools/umpire/rulebook/types"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// UpdateStateConsistencyModel verifies that workflow update lifecycle stages follow valid state transitions.
//
// Property: Updates must follow valid state transition paths and never regress to earlier stages.
// The valid lifecycle is: UNSPECIFIED -> ADMITTED -> ACCEPTED -> COMPLETED (or REJECTED at any stage).
//
// From tests: All update tests implicitly verify state consistency through WaitPolicy checks
// Expected behavior:
// - Updates progress through lifecycle stages monotonically
// - Once ACCEPTED, cannot regress to ADMITTED
// - Once COMPLETED, state is immutable
// - Rejected updates don't transition to later stages
//
// Valid transitions:
//   UNSPECIFIED -> ADMITTED (update received by history service)
//   ADMITTED -> ACCEPTED (worker accepts update)
//   ACCEPTED -> COMPLETED (worker completes update with result)
//   Any -> REJECTED (worker or system rejects update)
//
// Invalid transitions (violations):
//   ACCEPTED -> ADMITTED (regression)
//   COMPLETED -> ACCEPTED (regression)
//   COMPLETED -> REJECTED (state change after completion)
//   REJECTED -> ACCEPTED (cannot un-reject)
type UpdateStateConsistencyModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Track update state history to detect invalid transitions
	UpdateStateHistory map[string][]updateStateTransition
}

type updateStateTransition struct {
	FromState string
	ToState   string
	Timestamp time.Time
}

var _ rulebook.Model = (*UpdateStateConsistencyModel)(nil)

func (m *UpdateStateConsistencyModel) Name() string { return "updatestateconsistency" }

func (m *UpdateStateConsistencyModel) Init(_ context.Context, deps interface{}) error {
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("updatestateconsistency: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("updatestateconsistency: logger is required")
	}
	if registry == nil {
		return errors.New("updatestateconsistency: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.UpdateStateHistory = make(map[string][]updateStateTransition)

	return nil
}

func (m *UpdateStateConsistencyModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow entities
	workflowEntities := m.Registry.QueryEntities(entity.NewWorkflow())

	// Define valid state transitions
	validTransitions := map[string]map[string]bool{
		"UNSPECIFIED": {"ADMITTED": true, "REJECTED": true},
		"ADMITTED":    {"ACCEPTED": true, "REJECTED": true},
		"ACCEPTED":    {"COMPLETED": true, "REJECTED": true},
		"COMPLETED":   {}, // Terminal state - no transitions allowed
		"REJECTED":    {}, // Terminal state - no transitions allowed
	}

	for _, e := range workflowEntities {
		wf, ok := e.(*entity.Workflow)
		if !ok {
			continue
		}

		// TODO: Once WorkflowUpdate entity is implemented:
		// 1. Query all updates for this workflow
		// 2. Check state transition history for each update
		// 3. Validate each transition against validTransitions map
		// 4. Report violations for invalid transitions
		//
		// Example violation detection:
		// for updateID, transitions := range updateStateTransitions {
		//     for i := 1; i < len(transitions); i++ {
		//         prev := transitions[i-1]
		//         curr := transitions[i]
		//
		//         // Check if transition is valid
		//         if validTo, exists := validTransitions[prev.ToState]; !exists || !validTo[curr.ToState] {
		//             violationKey := wf.WorkflowID + ":" + updateID + ":transition"
		//             if m.shouldReport(violationKey, now) {
		//                 violations = append(violations, rulebook.Violation{
		//                     Model:   m.Name(),
		//                     Message: "workflow update invalid state transition",
		//                     Tags: map[string]string{
		//                         "workflowID":  wf.WorkflowID,
		//                         "updateID":    updateID,
		//                         "fromState":   prev.ToState,
		//                         "toState":     curr.ToState,
		//                         "transitionAt": curr.Timestamp.Format(time.RFC3339),
		//                     },
		//                 })
		//                 m.LastReported[violationKey] = now
		//             }
		//         }
		//     }
		// }

		_ = wf // Suppress unused variable warning
		_ = validTransitions // Suppress unused variable warning
	}

	// Clean up old state history
	cutoff := now.Add(-1 * time.Hour)
	for key, transitions := range m.UpdateStateHistory {
		if len(transitions) > 0 && transitions[len(transitions)-1].Timestamp.Before(cutoff) {
			delete(m.UpdateStateHistory, key)
		}
	}

	// Clean up old reported violations
	cutoff = now.Add(-10 * time.Minute)
	for key, lastReport := range m.LastReported {
		if lastReport.Before(cutoff) {
			delete(m.LastReported, key)
		}
	}

	// Log violations
	for _, v := range violations {
		tags := []tag.Tag{tag.NewStringTag("model", v.Model)}
		for k, val := range v.Tags {
			tags = append(tags, tag.NewStringTag(k, val))
		}
		m.Logger.Warn(fmt.Sprintf("violation: %s", v.Message), tags...)
	}

	return violations
}

func (m *UpdateStateConsistencyModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *UpdateStateConsistencyModel) Close(_ context.Context) error {
	return nil
}
