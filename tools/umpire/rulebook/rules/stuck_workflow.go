package rules

import (
	"go.temporal.io/server/tools/umpire/rulebook"
	rulebooktypes "go.temporal.io/server/tools/umpire/rulebook/types"
	entity "go.temporal.io/server/tools/bats/lineup/entities"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// StuckWorkflowModel verifies that workflows that are started actually complete
// (i.e., receive a RespondWorkflowTaskCompleted response).
// It queries Workflow entities to find workflows that were started but never completed,
// indicating they may be stuck waiting for task completion.
type StuckWorkflowModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Threshold is how long to wait before reporting a workflow as stuck
	// Default: 30 seconds
	Threshold time.Duration
}

var _ rulebook.Model = (*StuckWorkflowModel)(nil)

func (m *StuckWorkflowModel) Name() string { return "stuckworkflow" }

func (m *StuckWorkflowModel) Init(_ context.Context, deps interface{}) error {
	// Type assert to the expected deps structure
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("stuckworkflow: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("stuckworkflow: logger is required")
	}
	if registry == nil {
		return errors.New("stuckworkflow: registry is required")
	}

	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.Threshold = 30 * time.Second

	return nil
}

func (m *StuckWorkflowModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow entities
	workflowEntities := m.Registry.QueryEntities(entity.NewWorkflow())

	// Check for "stuck" workflows - workflows that were started but never completed
	for _, e := range workflowEntities {
		wf, ok := e.(*entity.Workflow)
		if !ok {
			continue
		}

		// A workflow is "stuck" if:
		// 1. It's in "started" state (started but not completed)
		// 2. It was started more than the threshold time ago
		if wf.FSM.Current() == "started" && !wf.StartedAt.IsZero() {
			age := now.Sub(wf.StartedAt)
			if age > m.Threshold {
				reportKey := "stuck-workflow:" + wf.NamespaceID + ":" + wf.WorkflowID
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "workflow was started but did not receive task completion response",
						Tags: map[string]string{
							"namespace":  wf.NamespaceID,
							"workflowID": wf.WorkflowID,
							"state":      wf.FSM.Current(),
							"startedAt":  wf.StartedAt.Format(time.RFC3339),
							"lastSeenAt": wf.LastSeenAt.Format(time.RFC3339),
							"age":        age.String(),
						},
					})
					m.LastReported[reportKey] = now
				}
			}
		}
	}

	// Clean up old entries in LastReported (keep entries for 10 minutes)
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

func (m *StuckWorkflowModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *StuckWorkflowModel) Close(_ context.Context) error {
	return nil
}
