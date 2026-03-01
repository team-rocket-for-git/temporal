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

// RetryResilienceModel verifies that workflows complete successfully even when
// transient failures occur (e.g., from pitcher fault injection).
// It ensures the system's retry logic handles transient failures gracefully.
type RetryResilienceModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Threshold is how long to wait before reporting a workflow as stuck
	// after it has been started. Default: 30 seconds
	Threshold time.Duration
}

var _ rulebook.Model = (*RetryResilienceModel)(nil)

func (m *RetryResilienceModel) Name() string { return "retryresilience" }

func (m *RetryResilienceModel) Init(_ context.Context, deps interface{}) error {
	// Type assert to the expected deps structure
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("retryresilience: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("retryresilience: logger is required")
	}
	if registry == nil {
		return errors.New("retryresilience: registry is required")
	}

	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.Threshold = 30 * time.Second

	return nil
}

func (m *RetryResilienceModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow entities
	workflowEntities := m.Registry.QueryEntities(entity.NewWorkflow())

	// Check for workflows that have been started but not completed within threshold
	for _, e := range workflowEntities {
		wf, ok := e.(*entity.Workflow)
		if !ok {
			continue
		}

		// Skip workflows that haven't been started yet
		if wf.FSM.Current() == "created" {
			continue
		}

		// A workflow shows poor retry resilience if:
		// 1. It's in "started" state (started but not completed)
		// 2. It was started more than the threshold time ago
		// This indicates the workflow may be stuck despite retry mechanisms
		if wf.FSM.Current() == "started" && !wf.StartedAt.IsZero() {
			age := now.Sub(wf.StartedAt)
			if age > m.Threshold {
				reportKey := "retry-resilience:" + wf.NamespaceID + ":" + wf.WorkflowID
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "workflow did not complete within expected time despite retry mechanisms",
						Tags: map[string]string{
							"namespace":  wf.NamespaceID,
							"workflowID": wf.WorkflowID,
							"state":      wf.FSM.Current(),
							"startedAt":  wf.StartedAt.Format(time.RFC3339),
							"lastSeenAt": wf.LastSeenAt.Format(time.RFC3339),
							"age":        age.String(),
							"threshold":  m.Threshold.String(),
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

func (m *RetryResilienceModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	// Only report again if it's been at least 1 minute since last report
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *RetryResilienceModel) Close(_ context.Context) error {
	return nil
}
