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

// SpeculativeTaskRollbackModel verifies proper rollback behavior for speculative workflow tasks.
//
// Property: When a speculative workflow task fails or is rejected, its associated updates
// must be properly rolled back and not written to history as accepted/completed.
//
// From tests: TestSpeculativeWorkflowTask_Fail, TestRunningWorkflowTask_NewEmptySpeculativeWorkflowTask_Rejected
// Expected behavior:
// - Failed speculative tasks should not have their updates persisted to history
// - Metrics should track commits vs rollbacks correctly
// - Updates in rejected speculative tasks should be retried or explicitly failed
//
// Rollback occurs when:
// - Worker explicitly fails the workflow task
// - Workflow task times out (ScheduleToStart or StartToClose)
// - Speculative task is rejected due to concurrent normal task
// - Worker connection is lost during task processing
//
// Commit occurs when:
// - Worker successfully completes the speculative task
// - Update is accepted and completed within the task
type SpeculativeTaskRollbackModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Track speculative tasks and their outcomes
	SpeculativeTaskOutcomes map[string]string // taskID -> "commit" or "rollback"
}

var _ rulebook.Model = (*SpeculativeTaskRollbackModel)(nil)

func (m *SpeculativeTaskRollbackModel) Name() string { return "speculativetaskrollback" }

func (m *SpeculativeTaskRollbackModel) Init(_ context.Context, deps interface{}) error {
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("speculativetaskrollback: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("speculativetaskrollback: logger is required")
	}
	if registry == nil {
		return errors.New("speculativetaskrollback: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.SpeculativeTaskOutcomes = make(map[string]string)

	return nil
}

func (m *SpeculativeTaskRollbackModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow task entities
	workflowTaskEntities := m.Registry.QueryEntities(entity.NewWorkflowTask())

	for _, e := range workflowTaskEntities {
		wt, ok := e.(*entity.WorkflowTask)
		if !ok {
			continue
		}

		// TODO: Once WorkflowTask entity tracks speculative status and rollback:
		// 1. Identify speculative workflow tasks (IsSpeculative flag)
		// 2. Check if task was rolled back (RolledBackAt timestamp)
		// 3. Verify that updates in rolled-back tasks are not in history
		// 4. Track rollback count vs commit count metrics
		//
		// Example violation:
		// if wt.IsSpeculative && wt.RolledBackAt.IsZero() && wt.FailedAt.IsZero() {
		//     // Task failed but wasn't properly rolled back
		//     taskKey := wt.WorkflowID + ":" + wt.TaskQueue
		//     if m.shouldReport(taskKey, now) {
		//         violations = append(violations, rulebook.Violation{
		//             Model:   m.Name(),
		//             Message: "speculative workflow task failed but rollback not recorded",
		//             Tags: map[string]string{
		//                 "workflowID": wt.WorkflowID,
		//                 "taskQueue":  wt.TaskQueue,
		//             },
		//         })
		//         m.LastReported[taskKey] = now
		//     }
		// }

		_ = wt // Suppress unused variable warning until speculative tracking is implemented
	}

	// Clean up old reported violations
	cutoff := now.Add(-10 * time.Minute)
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

func (m *SpeculativeTaskRollbackModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *SpeculativeTaskRollbackModel) Close(_ context.Context) error {
	return nil
}
