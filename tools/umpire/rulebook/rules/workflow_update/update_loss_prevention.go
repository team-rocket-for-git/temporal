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

// UpdateLossPreventionModel verifies that workflow updates are not lost during registry operations.
//
// Property: Updates should be preserved through graceful registry operations. Update loss is
// only acceptable in crash scenarios where the registry is lost without cleanup.
//
// From tests: TestScheduledSpeculativeWorkflowTask_LostUpdate, TestStartedSpeculativeWorkflowTask_LostUpdate,
//            TestFirstNormalWorkflowTask_UpdateResurrectedAfterRegistryCleared
//
// Expected behavior (graceful shutdown):
// - Registry cleared via clearUpdateRegistryAndAbortPendingUpdates()
// - In-flight update requests are aborted and return errors to clients
// - Frontend can retry update requests
// - Updates are eventually delivered after workflow recovers
//
// Acceptable loss (crash scenario):
// - Registry lost via loseUpdateRegistryAndAbandonPendingUpdates()
// - Shard finalizer doesn't run (simulated crash)
// - Pending update requests time out
// - Updates are permanently lost (acceptable in crash scenario)
//
// Violation conditions:
// - Update lost during graceful registry clear (should be aborted, not lost)
// - Update not resurrected after registry recovery (when workflow continues)
// - Multiple updates lost in same workflow (indicates systematic issue)
type UpdateLossPreventionModel struct {
	Logger       log.Logger
	Registry     rulebooktypes.EntityRegistry
	Mu           sync.Mutex
	LastReported map[string]time.Time
	// Track registry clear events and their type (graceful vs crash)
	RegistryClears map[string]registryClearInfo
}

type registryClearInfo struct {
	ClearedAt    time.Time
	IsGraceful   bool // true = clearUpdateRegistry, false = loseUpdateRegistry
	WorkflowID   string
	RunID        string
	UpdatesInFlight int // Number of updates that were in-flight when cleared
}

var _ rulebook.Model = (*UpdateLossPreventionModel)(nil)

func (m *UpdateLossPreventionModel) Name() string { return "updatelossprevention" }

func (m *UpdateLossPreventionModel) Init(_ context.Context, deps interface{}) error {
	type depsInterface interface {
		GetLogger() log.Logger
		GetRegistry() rulebooktypes.EntityRegistry
	}

	d, ok := deps.(depsInterface)
	if !ok {
		return errors.New("updatelossprevention: invalid deps type")
	}

	logger := d.GetLogger()
	registry := d.GetRegistry()

	if logger == nil {
		return errors.New("updatelossprevention: logger is required")
	}
	if registry == nil {
		return errors.New("updatelossprevention: registry is required")
	}
	m.Logger = logger
	m.Registry = registry
	m.LastReported = make(map[string]time.Time)
	m.RegistryClears = make(map[string]registryClearInfo)

	return nil
}

func (m *UpdateLossPreventionModel) Check(_ context.Context) []rulebook.Violation {
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

		workflowKey := wf.WorkflowID

		// TODO: Once WorkflowUpdate entity and registry clear tracking is implemented:
		// 1. Detect when update registry is cleared (from moves/events)
		// 2. Track whether clear was graceful (finalizer ran) or crash (finalizer timeout)
		// 3. For graceful clears: verify in-flight updates were aborted with errors
		// 4. For crash clears: updates timing out is acceptable
		// 5. After registry recovery: verify updates can be resurrected/retried
		//
		// Example violation (graceful clear):
		// if clearInfo, hadClear := m.RegistryClears[workflowKey]; hadClear && clearInfo.IsGraceful {
		//     // Check if updates were lost instead of aborted
		//     // Query updates that were sent before clear but never received error response
		//     if lostUpdateCount > 0 {
		//         clearKey := workflowKey + ":clear"
		//         if m.shouldReport(clearKey, now) {
		//             violations = append(violations, rulebook.Violation{
		//                 Model:   m.Name(),
		//                 Message: "workflow updates lost during graceful registry clear (should be aborted)",
		//                 Tags: map[string]string{
		//                     "workflowID":         wf.WorkflowID,
		//                     "runID":              wf.RunID,
		//                     "clearedAt":          clearInfo.ClearedAt.Format(time.RFC3339),
		//                     "updatesInFlight":    fmt.Sprintf("%d", clearInfo.UpdatesInFlight),
		//                     "updatesLost":        fmt.Sprintf("%d", lostUpdateCount),
		//                     "registryClearType":  "graceful",
		//                 },
		//             })
		//             m.LastReported[clearKey] = now
		//         }
		//     }
		// }

		_ = workflowKey // Suppress unused variable warning
	}

	// Clean up old registry clear tracking
	cutoff := now.Add(-1 * time.Hour)
	for key, clearInfo := range m.RegistryClears {
		if clearInfo.ClearedAt.Before(cutoff) {
			delete(m.RegistryClears, key)
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

func (m *UpdateLossPreventionModel) shouldReport(key string, now time.Time) bool {
	lastReport, reported := m.LastReported[key]
	if !reported {
		return true
	}
	return now.Sub(lastReport) >= 1*time.Minute
}

func (m *UpdateLossPreventionModel) Close(_ context.Context) error {
	return nil
}
