package rules_test

import (
	"context"
	"testing"
	"time"

	"github.com/looplab/fsm"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tools/bats/lineup/entities"
	"go.temporal.io/server/tools/umpire/rulebook/rules"
	rulebooktypes "go.temporal.io/server/tools/umpire/rulebook/types"
)

func TestStuckWorkflowModel_NoIssues(t *testing.T) {
	logger := log.NewNoopLogger()
	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   logger,
	})
	require.NoError(t, err)

	// Check with no entities - should not fail
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestStuckWorkflowModel_DetectsStuckWorkflow(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a workflow that was started but never completed
	now := time.Now()
	stuckWorkflow := &entities.Workflow{
		WorkflowID:  "test-workflow",
		NamespaceID: "default",
		StartedAt:   now.Add(-60 * time.Second), // Started 60 seconds ago
		LastSeenAt:  now.Add(-60 * time.Second),
	}
	stuckWorkflow.FSM = fsm.NewFSM(
		"started", // In started state, never completed
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{stuckWorkflow},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should detect the stuck workflow
	violations := m.Check(context.Background())
	require.Len(t, violations, 1)
	require.Equal(t, "stuckworkflow", violations[0].Model)
	require.Contains(t, violations[0].Message, "did not receive task completion response")
	require.Equal(t, "test-workflow", violations[0].Tags["workflowID"])
	require.Equal(t, "default", violations[0].Tags["namespace"])

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestStuckWorkflowModel_IgnoresCompletedWorkflows(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a workflow that was started and completed
	now := time.Now()
	completedWorkflow := &entities.Workflow{
		WorkflowID:  "test-workflow",
		NamespaceID: "default",
		StartedAt:   now.Add(-60 * time.Second),
		CompletedAt: now.Add(-30 * time.Second), // Completed 30 seconds ago
		LastSeenAt:  now.Add(-30 * time.Second),
	}
	completedWorkflow.FSM = fsm.NewFSM(
		"completed", // In completed state
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{completedWorkflow},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should NOT detect issue since workflow is completed
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestStuckWorkflowModel_IgnoresRecentlyStartedWorkflows(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a workflow that was started recently but hasn't completed yet
	now := time.Now()
	recentWorkflow := &entities.Workflow{
		WorkflowID:  "test-workflow",
		NamespaceID: "default",
		StartedAt:   now.Add(-10 * time.Second), // Started only 10 seconds ago
		LastSeenAt:  now.Add(-10 * time.Second),
	}
	recentWorkflow.FSM = fsm.NewFSM(
		"started", // In started state
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{recentWorkflow},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should NOT detect issue since workflow is recently started
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestStuckWorkflowModel_ThresholdBehavior(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a workflow that was started just before the threshold (won't trigger)
	now := time.Now()
	recentWorkflow := &entities.Workflow{
		WorkflowID:  "test-workflow",
		NamespaceID: "default",
		StartedAt:   now.Add(-29 * time.Second), // Just under the threshold
		LastSeenAt:  now.Add(-29 * time.Second),
	}
	recentWorkflow.FSM = fsm.NewFSM(
		"started",
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{recentWorkflow},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should NOT detect issue since it's below threshold
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestStuckWorkflowModel_MultipleWorkflows(t *testing.T) {
	captureLogger := &capturingLogger{}

	now := time.Now()

	// Create multiple workflows with different states
	stuckWorkflow1 := &entities.Workflow{
		WorkflowID:  "workflow-1",
		NamespaceID: "default",
		StartedAt:   now.Add(-60 * time.Second),
		LastSeenAt:  now.Add(-60 * time.Second),
	}
	stuckWorkflow1.FSM = fsm.NewFSM(
		"started",
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	completedWorkflow := &entities.Workflow{
		WorkflowID:  "workflow-2",
		NamespaceID: "default",
		StartedAt:   now.Add(-60 * time.Second),
		CompletedAt: now.Add(-30 * time.Second),
		LastSeenAt:  now.Add(-30 * time.Second),
	}
	completedWorkflow.FSM = fsm.NewFSM(
		"completed",
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	stuckWorkflow2 := &entities.Workflow{
		WorkflowID:  "workflow-3",
		NamespaceID: "default",
		StartedAt:   now.Add(-90 * time.Second),
		LastSeenAt:  now.Add(-90 * time.Second),
	}
	stuckWorkflow2.FSM = fsm.NewFSM(
		"started",
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{stuckWorkflow1, completedWorkflow, stuckWorkflow2},
	}

	m := &rules.StuckWorkflowModel{Threshold: 30 * time.Second}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should detect 2 stuck workflows
	violations := m.Check(context.Background())
	require.Len(t, violations, 2)

	// Check that we detected the right workflows
	workflowIDs := make(map[string]bool)
	for _, v := range violations {
		workflowIDs[v.Tags["workflowID"]] = true
	}
	require.True(t, workflowIDs["workflow-1"])
	require.True(t, workflowIDs["workflow-3"])
	require.False(t, workflowIDs["workflow-2"])

	err = m.Close(context.Background())
	require.NoError(t, err)
}
