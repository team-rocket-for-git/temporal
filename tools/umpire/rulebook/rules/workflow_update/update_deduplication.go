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

// UpdateDeduplicationModel verifies that workflow updates with the same ID are properly deduplicated.
//
// Property: For a given workflow execution and update ID, only one update should be processed.
// Multiple requests with the same update ID should return the same result without re-executing.
//
// From tests: TestScheduledSpeculativeWorkflowTask_DeduplicateID, TestStartedSpeculativeWorkflowTask_DeduplicateID
// Expected behavior:
// - Second update with same ID must be deduplicated
// - Both update requests should receive the same result
// - Only one update acceptance/completion event should be written to history
type UpdateDeduplicationModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
}

var _ rulebook.Model = (*UpdateDeduplicationModel)(nil)

func (m *UpdateDeduplicationModel) Name() string { return "updatededuplication" }

func (m *UpdateDeduplicationModel) Init(_ context.Context, deps interface{}) error {
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("updatededuplication: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("updatededuplication: logger is required")
	}
	if registry == nil {
		return errors.New("updatededuplication: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)

	return nil
}

func (m *UpdateDeduplicationModel) Check(_ context.Context) []rulebook.Violation {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	var violations []rulebook.Violation
	now := time.Now()

	// Get all workflow entities
	workflowEntities := m.Registry.QueryEntities(entity.NewWorkflow())

	// Track update IDs per workflow to detect duplicates
	updatesSeen := make(map[string]map[string]int) // workflowID -> updateID -> count

	for _, e := range workflowEntities {
		wf, ok := e.(*entity.Workflow)
		if !ok {
			continue
		}

		workflowKey := wf.WorkflowID
		if updatesSeen[workflowKey] == nil {
			updatesSeen[workflowKey] = make(map[string]int)
		}

		// TODO: Once WorkflowUpdate entity is implemented, query updates and check for duplicates
		// For now, this is a placeholder that will be populated when update tracking is added
		_ = updatesSeen
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

func (m *UpdateDeduplicationModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *UpdateDeduplicationModel) Close(_ context.Context) error {
	return nil
}
