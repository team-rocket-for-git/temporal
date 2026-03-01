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

// LostTaskModel verifies that tasks stored to persistence are successfully polled.
// It queries WorkflowTask entities to find tasks that have been stored to persistence
// but never polled, indicating they were lost from the matching service's in-memory state.
type LostTaskModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
}

var _ rulebook.Model = (*LostTaskModel)(nil)

func (m *LostTaskModel) Name() string { return "losttask" }

func (m *LostTaskModel) Init(_ context.Context, deps interface{}) error {
	// Type assert to the expected deps structure
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("losttask: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("losttask: logger is required")
	}
	if registry == nil {
		return errors.New("losttask: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)

	return nil
}

func (m *LostTaskModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow tasks and task queues
	workflowTaskEntities := m.Registry.QueryEntities(entity.NewWorkflowTask())
	taskQueueEntities := m.Registry.QueryEntities(entity.NewTaskQueue())

	// Check for "lost" tasks - tasks that were stored to persistence but never polled
	for _, e := range workflowTaskEntities {
		wt, ok := e.(*entity.WorkflowTask)
		if !ok {
			continue
		}

		// A task is "lost" if:
		// 1. It's in "stored" state (removed from matching, persisted to database)
		// 2. It was never successfully polled
		if wt.FSM.Current() == "stored" && wt.PolledAt.IsZero() {
			// Check if there was an empty poll after it was stored
			for _, queueEntity := range taskQueueEntities {
				tq, ok := queueEntity.(*entity.TaskQueue)
				if !ok || tq.Name != wt.TaskQueue {
					continue
				}

				// If there was an empty poll after the task was stored, it's definitely lost
				if !tq.LastEmptyPollTime.IsZero() && tq.LastEmptyPollTime.After(wt.StoredAt) {
					reportKey := "lost-task:" + wt.TaskQueue + ":" + wt.WorkflowID + ":" + wt.RunID
					if m.shouldReport(reportKey, now) {
						violations = append(violations, rulebook.Violation{
							Model:   m.Name(),
							Message: "workflow task was stored to persistence but never polled (lost from matching)",
							Tags: map[string]string{
								"taskQueue":     wt.TaskQueue,
								"workflowID":    wt.WorkflowID,
								"runID":         wt.RunID,
								"state":         wt.FSM.Current(),
								"addedAt":       wt.AddedAt.Format(time.RFC3339),
								"storedAt":      wt.StoredAt.Format(time.RFC3339),
								"emptyPollAt":   tq.LastEmptyPollTime.Format(time.RFC3339),
								"ageWhenStored": wt.StoredAt.Sub(wt.AddedAt).String(),
							},
						})
						m.LastReported[reportKey] = now
					}
					break
				}
			}
		}
	}

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

func (m *LostTaskModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *LostTaskModel) Close(_ context.Context) error {
	return nil
}
