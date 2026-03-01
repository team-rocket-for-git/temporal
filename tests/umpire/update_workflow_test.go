package catch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.temporal.io/server/tools/bats/lineup/entities"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
)

// SimpleUpdateWorkflow is a minimal workflow that handles updates
func SimpleUpdateWorkflow(ctx workflow.Context) (string, error) {
	// Set update handler
	err := workflow.SetUpdateHandler(ctx, "test-update", func(ctx workflow.Context, message string) (string, error) {
		return "updated: " + message, nil
	})
	if err != nil {
		return "", err
	}

	// Wait for signal to complete
	var done bool
	workflow.GetSignalChannel(ctx, "finish").Receive(ctx, &done)

	return "workflow completed", nil
}

func TestWorkflowWithUpdate(t *testing.T) {
	ts := NewTestSuite(t)
	ts.Setup()

	ts.Run(t, "WorkflowWithUpdate", func(ctx context.Context, t *testing.T, s *TestSuite) {
		// Register workflow and start worker
		taskQueue := s.TaskQueue()
		w := worker.New(s.SdkClient(), taskQueue, worker.Options{})
		w.RegisterWorkflow(SimpleUpdateWorkflow)
		require.NoError(t, w.Start())
		defer w.Stop()

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Start the workflow
		workflowID := "test-workflow-update-" + t.Name()
		workflowRun, err := s.SdkClient().ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: taskQueue,
		}, SimpleUpdateWorkflow)
		require.NoError(t, err, "Should start workflow")

		// Wait a bit for workflow to start and set up update handler
		time.Sleep(500 * time.Millisecond)

		// Send an update to the workflow
		updateHandle, err := s.SdkClient().UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
			WorkflowID:   workflowRun.GetID(),
			RunID:        workflowRun.GetRunID(),
			UpdateName:   "test-update",
			Args:         []interface{}{"hello"},
			WaitForStage: client.WorkflowUpdateStageCompleted,
		})
		require.NoError(t, err, "Should send update")

		// Get update result
		var updateResult string
		err = updateHandle.Get(ctx, &updateResult)
		require.NoError(t, err, "Should get update result")
		require.Equal(t, "updated: hello", updateResult)

		// Signal workflow to complete
		err = s.SdkClient().SignalWorkflow(ctx, workflowRun.GetID(), workflowRun.GetRunID(), "finish", true)
		require.NoError(t, err, "Should signal workflow")

		// Wait for workflow completion
		var result string
		err = workflowRun.Get(ctx, &result)
		require.NoError(t, err, "Workflow should complete successfully")
		require.Equal(t, "workflow completed", result)

		// Give umpire time to process events
		time.Sleep(500 * time.Millisecond)

		// Verify UpdateWorkflowExecutionRequest moves were captured
		scorebook := s.Umpire().Scorebook()
		workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
		updateMoves := scorebook.QueryByType(workflowEntityID, &moves.UpdateWorkflowExecutionRequest{})
		t.Logf("Total UpdateWorkflowExecutionRequest moves for workflow %s: %d", workflowID, len(updateMoves))
		require.GreaterOrEqual(t, len(updateMoves), 1, "Expected at least 1 UpdateWorkflowExecutionRequest move")

		// Query all moves for this workflow to see what we captured
		allMoves := scorebook.QueryByID(workflowEntityID)
		t.Logf("Total moves for workflow %s: %d", workflowID, len(allMoves))
		for _, move := range allMoves {
			t.Logf("  Move type: %s", move.MoveType())
		}

		// Verify WorkflowUpdate entities were created
		updateEntities := s.Umpire().Registry().QueryEntities(&entities.WorkflowUpdate{})
		t.Logf("Total WorkflowUpdate entities: %d", len(updateEntities))

		if len(updateEntities) > 0 {
			wu := updateEntities[0].(*entities.WorkflowUpdate)
			t.Logf("WorkflowUpdate: updateID=%s, workflowID=%s, state=%s, handler=%s",
				wu.UpdateID, wu.WorkflowID, wu.FSM.Current(), wu.HandlerName)
			require.Equal(t, "test-update", wu.HandlerName)
		}

		// Manually verify umpire check completes with no violations
		violations := s.Umpire().Check(ctx)
		require.Empty(t, violations, "Umpire check should complete with no violations")
		t.Logf("Umpire check completed successfully with %d violations", len(violations))
	})
}
