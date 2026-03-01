package catch

import (
	"context"
	"testing"
	"time"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/require"
	omeskitchensink "github.com/temporalio/omes/loadgen/kitchensink"
	"go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/server/api/matchingservice/v1"
	"go.temporal.io/server/tools/umpire/pitcher"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
)

func TestWorkflow(t *testing.T) {
	ts := NewTestSuite(t)
	ts.Setup()

	ts.Run(t, "KitchenSinkWorkflow", func(ctx context.Context, t *testing.T, s *TestSuite) {
		// Register workflow and start worker
		taskQueue := s.TaskQueue()
		w := worker.New(s.SdkClient(), taskQueue, worker.Options{})
		RegisterWorkflows(w)
		require.NoError(t, w.Start())
		defer w.Stop()

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Build the play scenario with KitchenSinkWorkflow
		workflowInput := &omeskitchensink.WorkflowInput{
			InitialActions: []*omeskitchensink.ActionSet{
				{
					Actions: []*omeskitchensink.Action{
						{
							Variant: &omeskitchensink.Action_ReturnResult{
								ReturnResult: &omeskitchensink.ReturnResultAction{
									ReturnThis: &common.Payload{
										Data: []byte("test result"),
									},
								},
							},
						},
					},
				},
			},
		}
		play := pitcher.NewPlayBuilder().
			WithStartWorkflow(
				s.SdkClient(),
				client.StartWorkflowOptions{
					TaskQueue: taskQueue,
				},
				KitchenSinkWorkflow,
				workflowInput,
			).
			Build()

		// Execute the play
		workflowRun, err := s.Pitcher().Execute(ctx, play)
		require.NoError(t, err, "Should execute play")

		// Get workflow ID for querying moves
		workflowID := workflowRun.GetID()

		// Wait for completion
		err = workflowRun.Get(ctx, nil)
		require.NoError(t, err, "Workflow should complete successfully")

		// Verify StoreWorkflowTask moves were captured from gRPC
		scorebook := s.Umpire().Scorebook()
		workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
		storeTaskMoves := scorebook.QueryByType(workflowEntityID, &moves.StoreWorkflowTask{})
		t.Logf("Total StoreWorkflowTask moves for workflow %s: %d", workflowID, len(storeTaskMoves))
		require.GreaterOrEqual(t, len(storeTaskMoves), 1, "Expected at least 1 StoreWorkflowTask move")

		// Manually verify umpire check completes with no violations
		violations := s.Umpire().Check(ctx)
		require.Empty(t, violations, "Umpire check should complete with no violations")
		t.Logf("Umpire check completed successfully with %d violations", len(violations))
	})

	ts.Run(t, "WorkflowWithFaultInjection", func(ctx context.Context, t *testing.T, s *TestSuite) {
		// Register workflow and start worker
		taskQueue := s.TaskQueue()
		w := worker.New(s.SdkClient(), taskQueue, worker.Options{})
		RegisterWorkflows(w)
		require.NoError(t, w.Start())
		defer w.Stop()

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Generate unique workflow ID for matching
		workflowID := "test-workflow-" + uuid.New()

		// Build the play scenario with fault injection using KitchenSinkWorkflow
		workflowInput := &omeskitchensink.WorkflowInput{
			InitialActions: []*omeskitchensink.ActionSet{
				{
					Actions: []*omeskitchensink.Action{
						{
							Variant: &omeskitchensink.Action_ReturnResult{
								ReturnResult: &omeskitchensink.ReturnResultAction{
									ReturnThis: &common.Payload{
										Data: []byte("test result"),
									},
								},
							},
						},
					},
				},
			},
		}
		play := pitcher.NewPlayBuilder().
			WithFault(
				&matchingservice.AddWorkflowTaskRequest{},
				pitcher.FailPlay(serviceerror.NewResourceExhausted(enumspb.RESOURCE_EXHAUSTED_CAUSE_SYSTEM_OVERLOADED, "pitcher fault injection")),
				&pitcher.MatchCriteria{
					Entities: []lineuptypes.EntityID{
						lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID),
					},
				},
			).
			WithStartWorkflow(
				s.SdkClient(),
				client.StartWorkflowOptions{
					ID:        workflowID,
					TaskQueue: taskQueue,
				},
				KitchenSinkWorkflow,
				workflowInput,
			).
			Build()

		// Execute the play - the pitcher will set up faults and start the workflow
		workflowRun, err := s.Pitcher().Execute(ctx, play)
		require.NoError(t, err, "Should execute play despite fault injection")

		// Wait for completion - workflow should complete despite the initial fault
		// The system will retry the AddWorkflowTask call automatically
		err = workflowRun.Get(ctx, nil)
		require.NoError(t, err, "Workflow should complete successfully after automatic retry")

		// Verify StoreWorkflowTask moves were recorded via scorebook
		// With fault injection, we expect at least 2 attempts (1 failed, 1+ successful)
		scorebook := s.Umpire().Scorebook()
		workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
		storeTaskMoves := scorebook.QueryByType(workflowEntityID, &moves.StoreWorkflowTask{})
		t.Logf("Total StoreWorkflowTask moves for workflow %s: %d", workflowID, len(storeTaskMoves))
		require.GreaterOrEqual(t, len(storeTaskMoves), 2,
			"Expected at least 2 StoreWorkflowTask attempts (1 failed due to fault injection, 1+ successful)")

		// Manually verify umpire check completes with no violations
		violations := s.Umpire().Check(ctx)
		require.Empty(t, violations, "Umpire check should complete with no violations")
		t.Logf("Umpire check completed successfully with %d violations", len(violations))
	})
}
