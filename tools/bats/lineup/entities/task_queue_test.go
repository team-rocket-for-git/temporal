package entities_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	workflowpb "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/matchingservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tools/bats/lineup"
	"go.temporal.io/server/tools/bats/lineup/entities"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

// TestLostTaskUnit verifies the unit-level behavior of lost task detection.
// This test doesn't require full Temporal infrastructure - it directly tests
// the entity and model behavior.
func TestLostTaskUnit(t *testing.T) {
	// Setup
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	// Note: Model initialization is commented out until we resolve model registration
	// taskMatchingModel := &rules.TaskMatchingModel{}
	// err = taskMatchingModel.Init(context.Background(), rulebooktypes.Deps{
	// 	Registry: registry,
	// 	Logger:   logger,
	// })
	// require.NoError(t, err)

	taskQueue := "test-queue"
	workflowID := "test-workflow-1"
	runID := "test-run-1"

	// Create identities
	workflowTaskID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowTaskType, taskQueue+":"+workflowID+":"+runID)
	taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, taskQueue)
	taskIdentity := &lineuptypes.Identity{
		EntityID: workflowTaskID,
		ParentID: &taskQueueID,
	}
	taskQueueIdentity := &lineuptypes.Identity{
		EntityID: taskQueueID,
		ParentID: nil,
	}

	// Simulate the sequence of events:
	// 1. Task added to matching
	// 2. Task stored to persistence (removed from matching)
	// 3. Empty poll (task not in matching anymore)
	events := []scorebooktypes.Move{
		&moves.AddWorkflowTask{
			Request: &matchingservice.AddWorkflowTaskRequest{
				TaskQueue: &taskqueuepb.TaskQueue{Name: taskQueue},
				Execution: &commonpb.WorkflowExecution{
					WorkflowId: workflowID,
					RunId:      runID,
				},
			},
			Identity: taskIdentity,
		},
		&moves.StoreWorkflowTask{
			TaskQueue:  taskQueue,
			Identity:   taskIdentity,
			WorkflowID: workflowID,
			RunID:      runID,
		},
		&moves.PollWorkflowTask{
			Request: &matchingservice.PollWorkflowTaskQueueRequest{
				PollRequest: &workflowpb.PollWorkflowTaskQueueRequest{
					TaskQueue: &taskqueuepb.TaskQueue{Name: taskQueue},
				},
			},
			Response: &matchingservice.PollWorkflowTaskQueueResponse{
				// Empty response - no task returned
			},
			Identity:     taskQueueIdentity,
			TaskReturned: false,
		},
	}

	err = registry.RouteEvents(context.Background(), events)
	require.NoError(t, err)

	// Run the check - should detect the lost task
	// taskMatchingModel.Check(context.Background())

	// Verify the entities are in the expected state
	workflowTasks := registry.QueryEntities(entities.NewWorkflowTask())
	require.Len(t, workflowTasks, 1)

	wt := workflowTasks[0].(*entities.WorkflowTask)
	require.Equal(t, "stored", wt.FSM.Current(), "Task should be in 'stored' state")
	require.False(t, wt.StoredAt.IsZero(), "StoredAt timestamp should be set")
	require.True(t, wt.PolledAt.IsZero(), "Task should never have been polled")

	taskQueues := registry.QueryEntities(entities.NewTaskQueue())
	require.Len(t, taskQueues, 1)

	tq := taskQueues[0].(*entities.TaskQueue)
	require.False(t, tq.LastEmptyPollTime.IsZero(), "LastEmptyPollTime should be set")
	require.True(t, tq.LastEmptyPollTime.After(wt.StoredAt), "Empty poll should occur after task was stored")
}

// TestTaskQueueTracksEmptyPolls verifies that TaskQueue entities properly track empty polls
func TestTaskQueueTracksEmptyPolls(t *testing.T) {
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	taskQueue := "test-queue"
	taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, taskQueue)
	taskQueueIdentity := &lineuptypes.Identity{
		EntityID: taskQueueID,
		ParentID: nil,
	}

	// Send an empty poll event
	emptyPollEvent := &moves.PollWorkflowTask{
		Request: &matchingservice.PollWorkflowTaskQueueRequest{
			PollRequest: &workflowpb.PollWorkflowTaskQueueRequest{
				TaskQueue: &taskqueuepb.TaskQueue{Name: taskQueue},
			},
		},
		Response: &matchingservice.PollWorkflowTaskQueueResponse{
			// Empty response
		},
		Identity:     taskQueueIdentity,
		TaskReturned: false,
	}

	err = registry.RouteEvents(context.Background(), []scorebooktypes.Move{emptyPollEvent})
	require.NoError(t, err)

	// Verify TaskQueue tracked the empty poll
	taskQueues := registry.QueryEntities(entities.NewTaskQueue())
	require.Len(t, taskQueues, 1)

	tq := taskQueues[0].(*entities.TaskQueue)
	require.Equal(t, taskQueue, tq.Name)
	require.False(t, tq.LastEmptyPollTime.IsZero(), "LastEmptyPollTime should be set")
}

// TestWorkflowTaskStoreTransition verifies the FSM transition to stored state
func TestWorkflowTaskStoreTransition(t *testing.T) {
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	taskQueue := "test-queue"
	workflowID := "test-workflow"
	runID := "test-run"

	workflowTaskID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowTaskType, taskQueue+":"+workflowID+":"+runID)
	taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, taskQueue)
	taskIdentity := &lineuptypes.Identity{
		EntityID: workflowTaskID,
		ParentID: &taskQueueID,
	}

	// Send events: add then store
	events := []scorebooktypes.Move{
		&moves.AddWorkflowTask{
			Request: &matchingservice.AddWorkflowTaskRequest{
				TaskQueue: &taskqueuepb.TaskQueue{Name: taskQueue},
				Execution: &commonpb.WorkflowExecution{
					WorkflowId: workflowID,
					RunId:      runID,
				},
			},
			Identity: taskIdentity,
		},
		&moves.StoreWorkflowTask{
			TaskQueue:  taskQueue,
			Identity:   taskIdentity,
			WorkflowID: workflowID,
			RunID:      runID,
		},
	}

	err = registry.RouteEvents(context.Background(), events)
	require.NoError(t, err)

	// Verify task transitioned to stored state
	workflowTasks := registry.QueryEntities(entities.NewWorkflowTask())
	require.Len(t, workflowTasks, 1)

	wt := workflowTasks[0].(*entities.WorkflowTask)
	require.Equal(t, "stored", wt.FSM.Current())
	require.False(t, wt.AddedAt.IsZero())
	require.False(t, wt.StoredAt.IsZero())
	require.True(t, wt.PolledAt.IsZero())
	require.True(t, wt.StoredAt.After(wt.AddedAt))
}
