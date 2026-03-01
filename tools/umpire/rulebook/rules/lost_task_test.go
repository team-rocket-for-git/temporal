package rules_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/looplab/fsm"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/tools/bats/lineup/entities"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/rulebook/rules"
	rulebooktypes "go.temporal.io/server/tools/umpire/rulebook/types"
)

func TestLostTaskModel_NoIssues(t *testing.T) {
	logger := log.NewNoopLogger()
	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{},
	}

	m := &rules.LostTaskModel{}
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

func TestLostTaskModel_DetectsLostTask(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a workflow task that was stored to persistence but never polled
	now := time.Now()
	lostTask := &entities.WorkflowTask{
		TaskQueue:  "test-queue",
		WorkflowID: "test-workflow",
		RunID:      "test-run",
		AddedAt:    now.Add(-30 * time.Second),
		StoredAt:   now.Add(-20 * time.Second),
		// PolledAt is zero - task was never polled
	}
	lostTask.FSM = fsm.NewFSM(
		"stored", // In stored state
		fsm.Events{
			{Name: "add", Src: []string{"created"}, Dst: "added"},
			{Name: "store", Src: []string{"added"}, Dst: "stored"},
			{Name: "poll", Src: []string{"stored"}, Dst: "polled"},
		},
		fsm.Callbacks{},
	)

	// Create a task queue that had an empty poll after the task was stored
	taskQueue := &entities.TaskQueue{
		Name:              "test-queue",
		LastEmptyPollTime: now.Add(-10 * time.Second), // After task was stored
	}

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{lostTask, taskQueue},
	}

	m := &rules.LostTaskModel{}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should detect the lost task
	violations := m.Check(context.Background())
	require.Len(t, violations, 1)
	require.Equal(t, "losttask", violations[0].Model)
	require.Contains(t, violations[0].Message, "stored to persistence but never polled")

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestLostTaskModel_IgnoresPolledTasks(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a task that was stored AND polled
	now := time.Now()
	polledTask := &entities.WorkflowTask{
		TaskQueue:  "test-queue",
		WorkflowID: "test-workflow",
		RunID:      "test-run",
		AddedAt:    now.Add(-30 * time.Second),
		StoredAt:   now.Add(-20 * time.Second),
		PolledAt:   now.Add(-15 * time.Second), // Was polled
	}
	polledTask.FSM = fsm.NewFSM(
		"polled",
		fsm.Events{
			{Name: "add", Src: []string{"created"}, Dst: "added"},
			{Name: "store", Src: []string{"added"}, Dst: "stored"},
			{Name: "poll", Src: []string{"stored"}, Dst: "polled"},
		},
		fsm.Callbacks{},
	)

	taskQueue := &entities.TaskQueue{
		Name:              "test-queue",
		LastEmptyPollTime: now.Add(-10 * time.Second),
	}

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{polledTask, taskQueue},
	}

	m := &rules.LostTaskModel{}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should NOT detect issue since task was polled
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

func TestLostTaskModel_IgnoresTasksNotStored(t *testing.T) {
	captureLogger := &capturingLogger{}

	// Create a task that's still in "added" state (not stored yet)
	now := time.Now()
	addedTask := &entities.WorkflowTask{
		TaskQueue:  "test-queue",
		WorkflowID: "test-workflow",
		RunID:      "test-run",
		AddedAt:    now.Add(-30 * time.Second),
		// Not stored yet
	}
	addedTask.FSM = fsm.NewFSM(
		"added", // Still in added state
		fsm.Events{
			{Name: "add", Src: []string{"created"}, Dst: "added"},
			{Name: "store", Src: []string{"added"}, Dst: "stored"},
		},
		fsm.Callbacks{},
	)

	taskQueue := &entities.TaskQueue{
		Name:              "test-queue",
		LastEmptyPollTime: now.Add(-10 * time.Second),
	}

	mockRegistry := &mockEntityRegistry{
		entities: []interface{}{addedTask, taskQueue},
	}

	m := &rules.LostTaskModel{}
	err := m.Init(context.Background(), rulebooktypes.Deps{
		Registry: mockRegistry,
		Logger:   captureLogger,
	})
	require.NoError(t, err)

	// Run check - should NOT detect issue since task wasn't stored
	violations := m.Check(context.Background())
	require.Empty(t, violations)

	err = m.Close(context.Background())
	require.NoError(t, err)
}

// mockEntityRegistry is a simple mock for testing
type mockEntityRegistry struct {
	entities []interface{}
}

func (m *mockEntityRegistry) QueryEntities(_ interface{ Type() lineuptypes.EntityType }) []interface{} {
	return m.entities
}

// capturingLogger captures error logs for testing
type capturingLogger struct {
	mu     sync.Mutex
	errors []string
}

func (c *capturingLogger) Debug(msg string, tags ...tag.Tag)  {}
func (c *capturingLogger) Info(msg string, tags ...tag.Tag)   {}
func (c *capturingLogger) Warn(msg string, tags ...tag.Tag)   {}
func (c *capturingLogger) DPanic(msg string, tags ...tag.Tag) {}
func (c *capturingLogger) Panic(msg string, tags ...tag.Tag)  {}

func (c *capturingLogger) Error(msg string, tags ...tag.Tag) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, msg)
}

func (c *capturingLogger) Fatal(msg string, tags ...tag.Tag) {
	c.Error(msg, tags...)
}

func (c *capturingLogger) HasErrorContaining(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, err := range c.errors {
		if strings.Contains(err, substr) {
			return true
		}
	}
	return false
}
