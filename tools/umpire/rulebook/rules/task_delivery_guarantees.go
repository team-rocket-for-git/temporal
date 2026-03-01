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

// TaskDeliveryGuaranteesModel verifies that tasks added to task queues are
// eventually delivered (polled or stored) within a reasonable time.
// This property ensures no tasks are lost in the matching service.
type TaskDeliveryGuaranteesModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Threshold is how long to wait before reporting a task as stuck
	// after it has been added to a task queue. Default: 60 seconds
	Threshold time.Duration
}

var _ rulebook.Model = (*TaskDeliveryGuaranteesModel)(nil)

func (m *TaskDeliveryGuaranteesModel) Name() string { return "taskdeliveryguarantees" }

func (m *TaskDeliveryGuaranteesModel) Init(_ context.Context, deps interface{}) error {
	// Type assert to the expected deps structure
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("taskdeliveryguarantees: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("taskdeliveryguarantees: logger is required")
	}
	if registry == nil {
		return errors.New("taskdeliveryguarantees: registry is required")
	}

	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.Threshold = 60 * time.Second

	return nil
}

func (m *TaskDeliveryGuaranteesModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow task entities
	taskEntities := m.Registry.QueryEntities(entity.NewWorkflowTask())

	// Check for tasks that have been added but not delivered within threshold
	for _, e := range taskEntities {
		wt, ok := e.(*entity.WorkflowTask)
		if !ok {
			continue
		}

		// Skip tasks that haven't been added yet
		if wt.FSM.Current() == "created" {
			continue
		}

		// A task violates delivery guarantees if:
		// 1. It's in "added" state (added but not polled or stored)
		// 2. It was added more than the threshold time ago
		// This indicates the task may be lost or the matching service is not delivering it
		if wt.FSM.Current() == "added" && !wt.AddedAt.IsZero() {
			age := now.Sub(wt.AddedAt)
			if age > m.Threshold {
				reportKey := "task-delivery:" + wt.TaskQueue + ":" + wt.WorkflowID + ":" + wt.RunID
				if m.shouldReport(reportKey, now) {
					violations = append(violations, rulebook.Violation{
						Model:   m.Name(),
						Message: "task was not delivered within expected time",
						Tags: map[string]string{
							"taskQueue":  wt.TaskQueue,
							"workflowID": wt.WorkflowID,
							"runID":      wt.RunID,
							"state":      wt.FSM.Current(),
							"addedAt":    wt.AddedAt.Format(time.RFC3339),
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

func (m *TaskDeliveryGuaranteesModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	// Only report again if it's been at least 1 minute since last report
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *TaskDeliveryGuaranteesModel) Close(_ context.Context) error {
	return nil
}
