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

// UpdateCompletionModel verifies that accepted workflow updates eventually complete or are explicitly rejected.
//
// Property: An update that reaches the ACCEPTED stage must eventually transition to either COMPLETED or explicitly fail.
// Updates should not remain in ACCEPTED state indefinitely without completing.
//
// From tests: TestEmptySpeculativeWorkflowTask_AcceptComplete, TestNotEmptySpeculativeWorkflowTask_AcceptComplete
// Expected behavior:
// - After acceptance, update should complete with a result
// - Poll requests should see the completion
// - History should contain both WorkflowExecutionUpdateAccepted and WorkflowExecutionUpdateCompleted events
//
// Violation conditions:
// - Update accepted but not completed after reasonable time (accounting for task timeout)
// - Update in limbo state (neither completing nor explicitly rejected)
type UpdateCompletionModel struct {
	Logger           log.Logger
	Registry         rulebooktypes.EntityRegistry
	Mu               sync.Mutex
	LastReported     map[string]time.Time
	AcceptedAt       map[string]time.Time // Track when updates were accepted
	CompletionWindow time.Duration        // How long to wait before flagging incomplete update
}

var _ rulebook.Model = (*UpdateCompletionModel)(nil)

func (m *UpdateCompletionModel) Name() string { return "updatecompletion" }

func (m *UpdateCompletionModel) Init(_ context.Context, deps interface{}) error {
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("updatecompletion: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("updatecompletion: logger is required")
	}
	if registry == nil {
		return errors.New("updatecompletion: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.AcceptedAt = make(map[string]time.Time)
	// Default: Allow 30 seconds for completion after acceptance (covers workflow task timeout)
	m.CompletionWindow = 30 * time.Second

	return nil
}

func (m *UpdateCompletionModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow entities
	workflowEntities := m.Registry.QueryEntities(entity.NewWorkflow())

	for _, e := range workflowEntities {
		wf, ok := e.(*entity.Workflow)
		if !ok {
			continue
		}

		// TODO: Once WorkflowUpdate entity is implemented:
		// 1. Query all updates for this workflow
		// 2. Find updates in ACCEPTED state
		// 3. Check if they've been accepted longer than CompletionWindow
		// 4. Report violation if update is stuck in ACCEPTED state
		//
		// Example violation:
		// updateKey := wf.WorkflowID + ":" + updateID
		// if acceptedTime, exists := m.AcceptedAt[updateKey]; exists {
		//     if now.Sub(acceptedTime) > m.CompletionWindow {
		//         violations = append(violations, rulebook.Violation{
		//             Model:   m.Name(),
		//             Message: "workflow update accepted but not completed within expected window",
		//             Tags: map[string]string{
		//                 "workflowID":     wf.WorkflowID,
		//                 "updateID":       updateID,
		//                 "acceptedAt":     acceptedTime.Format(time.RFC3339),
		//                 "timeInAccepted": now.Sub(acceptedTime).String(),
		//             },
		//         })
		//     }
		// }

		_ = wf // Suppress unused variable warning until WorkflowUpdate entity is implemented
	}

	// Clean up old tracked accepted updates
	cutoff := now.Add(-1 * time.Hour)
	for key, acceptedTime := range m.AcceptedAt {
		if acceptedTime.Before(cutoff) {
			delete(m.AcceptedAt, key)
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

func (m *UpdateCompletionModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *UpdateCompletionModel) Close(_ context.Context) error {
	return nil
}
