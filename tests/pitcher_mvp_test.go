package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.temporal.io/server/tests/testcore"
	"go.temporal.io/server/tools/umpire/pitcher"
)

type PitcherMVPSuite struct {
	testcore.FunctionalTestBase
}

func TestPitcherMVP(t *testing.T) {
	suite.Run(t, new(PitcherMVPSuite))
}

// TestPitcherIntegration verifies that pitcher fault injection works end-to-end
func (s *PitcherMVPSuite) TestPitcherIntegration() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure pitcher: fail first AddWorkflowTask call
	s.ConfigurePitcher("matchingservice.AddWorkflowTask", pitcher.PlayConfig{
		Play:  pitcher.FailPlay(serviceerror.NewResourceExhausted(enumspb.RESOURCE_EXHAUSTED_CAUSE_SYSTEM_OVERLOADED, "test error")),
		Match: &pitcher.MatchCriteria{
			// No criteria means match first request
		},
	})

	// Register and start worker
	w := worker.New(s.SdkClient(), s.TaskQueue(), worker.Options{})
	w.RegisterWorkflow(SimpleWorkflow)
	s.NoError(w.Start())
	defer w.Stop()

	// Start a simple workflow
	workflowID := "pitcher-mvp-test"
	we, err := s.SdkClient().ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.TaskQueue(),
	}, SimpleWorkflow)
	s.NoError(err)

	// Despite the injected failure, the workflow should complete successfully
	// due to retry logic
	err = we.Get(ctx, nil)
	s.NoError(err, "Workflow should complete despite transient matching failure")
}

// TestPitcherReset verifies that pitcher state is reset between tests
func (s *PitcherMVPSuite) TestPitcherReset() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// This test should have no pitcher configuration from the previous test
	// The workflow should complete without any injected failures

	w := worker.New(s.SdkClient(), s.TaskQueue(), worker.Options{})
	w.RegisterWorkflow(SimpleWorkflow)
	s.NoError(w.Start())
	defer w.Stop()

	workflowID := "pitcher-reset-test"
	we, err := s.SdkClient().ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.TaskQueue(),
	}, SimpleWorkflow)
	s.NoError(err)

	err = we.Get(ctx, nil)
	s.NoError(err, "Workflow should complete without any failures")
}

// SimpleWorkflow is a minimal workflow for testing
func SimpleWorkflow(ctx workflow.Context) error {
	return nil
}
