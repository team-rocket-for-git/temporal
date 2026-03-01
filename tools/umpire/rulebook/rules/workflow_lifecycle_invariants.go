package rules

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

// WorkflowLifecycleInvariantsModel verifies that workflows follow correct lifecycle
// state transitions and maintain timestamp ordering invariants.
type WorkflowLifecycleInvariantsModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
}

var _ rulebook.Model = (*WorkflowLifecycleInvariantsModel)(nil)

func (m *WorkflowLifecycleInvariantsModel) Name() string { return "workflowlifecycleinvariants" }

func (m *WorkflowLifecycleInvariantsModel) Init(_ context.Context, deps interface{}) error {
	// Type assert to the expected deps structure
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("workflowlifecycleinvariants: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("workflowlifecycleinvariants: logger is required")
	}
	if registry == nil {
		return errors.New("workflowlifecycleinvariants: registry is required")
	}

	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)

	return nil
}

func (m *WorkflowLifecycleInvariantsModel) Check(_ context.Context) []rulebook.Violation {
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

		// Check timestamp ordering invariants
		if !wf.StartedAt.IsZero() && !wf.CompletedAt.IsZero() {
			// Invariant: StartedAt must be before CompletedAt
			if wf.StartedAt.After(wf.CompletedAt) {
				reportKey := "lifecycle-timestamp:" + wf.NamespaceID + ":" + wf.WorkflowID + ":started-after-completed"
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "workflow started timestamp is after completed timestamp",
						Tags: map[string]string{
							"namespace":   wf.NamespaceID,
							"workflowID":  wf.WorkflowID,
							"state":       wf.FSM.Current(),
							"startedAt":   wf.StartedAt.Format(time.RFC3339),
							"completedAt": wf.CompletedAt.Format(time.RFC3339),
						},
					})
					m.LastReported[reportKey] = now
				}
			}
		}

		// Check LastSeenAt invariants
		if !wf.StartedAt.IsZero() && !wf.LastSeenAt.IsZero() {
			// Invariant: LastSeenAt must be >= StartedAt
			if wf.LastSeenAt.Before(wf.StartedAt) {
				reportKey := "lifecycle-timestamp:" + wf.NamespaceID + ":" + wf.WorkflowID + ":lastseen-before-started"
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "workflow last seen timestamp is before started timestamp",
						Tags: map[string]string{
							"namespace":  wf.NamespaceID,
							"workflowID": wf.WorkflowID,
							"state":      wf.FSM.Current(),
							"startedAt":  wf.StartedAt.Format(time.RFC3339),
							"lastSeenAt": wf.LastSeenAt.Format(time.RFC3339),
						},
					})
					m.LastReported[reportKey] = now
				}
			}
		}

		// Check that completed workflows don't have events after completion
		if wf.FSM.Current() == "completed" && !wf.CompletedAt.IsZero() && !wf.LastSeenAt.IsZero() {
			// Invariant: LastSeenAt should not be significantly after CompletedAt
			// Allow a small grace period (1 second) for race conditions
			gracePeriod := 1 * time.Second
			if wf.LastSeenAt.Sub(wf.CompletedAt) > gracePeriod {
				reportKey := "lifecycle-timestamp:" + wf.NamespaceID + ":" + wf.WorkflowID + ":events-after-completion"
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "workflow received events after completion",
						Tags: map[string]string{
							"namespace":   wf.NamespaceID,
							"workflowID":  wf.WorkflowID,
							"state":       wf.FSM.Current(),
							"completedAt": wf.CompletedAt.Format(time.RFC3339),
							"lastSeenAt":  wf.LastSeenAt.Format(time.RFC3339),
							"delta":       wf.LastSeenAt.Sub(wf.CompletedAt).String(),
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

func (m *WorkflowLifecycleInvariantsModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	// Only report again if it's been at least 1 minute since last report
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *WorkflowLifecycleInvariantsModel) Close(_ context.Context) error {
	return nil
}
