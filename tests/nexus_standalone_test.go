package tests

import (
	"testing"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/chasm/lib/nexusoperation"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/protobuf/types/known/durationpb"
)

var nexusStandaloneOpts = []testcore.TestOption{
	testcore.WithDynamicConfig(dynamicconfig.EnableChasm, true),
	testcore.WithDynamicConfig(nexusoperation.ChasmNexusEnabled, true),
}

func TestStandaloneNexusOperation(t *testing.T) {
	t.Run("StartAndDescribe", func(t *testing.T) {
		env := testcore.NewEnv(t, nexusStandaloneOpts...)
		ctx := env.Context()

		operationID := testcore.RandomizeStr(t.Name())
		startResp := startAndValidateNexusOperation(env, operationID)

		descResp, err := env.FrontendClient().DescribeNexusOperationExecution(ctx, &workflowservice.DescribeNexusOperationExecutionRequest{
			Namespace:   env.Namespace().String(),
			OperationId: operationID,
			RunId:       startResp.RunId,
		})
		env.NoError(err)
		env.Equal(startResp.RunId, descResp.RunId)
		env.Equal(enumspb.NEXUS_OPERATION_EXECUTION_STATUS_RUNNING, descResp.GetInfo().GetStatus())
		env.NotEmpty(descResp.LongPollToken)
	})

	t.Run("IDConflictPolicy_Fail", func(t *testing.T) {
		env := testcore.NewEnv(t, nexusStandaloneOpts...)
		ctx := env.Context()

		operationID := testcore.RandomizeStr(t.Name())
		firstResp := startAndValidateNexusOperation(env, operationID)

		// Second start with different request ID should fail.
		_, err := env.FrontendClient().StartNexusOperationExecution(ctx, &workflowservice.StartNexusOperationExecutionRequest{
			Namespace:              env.Namespace().String(),
			OperationId:            operationID,
			Endpoint:               "test-endpoint",
			Service:                "test-service",
			Operation:              "test-operation",
			RequestId:              "different-request-id",
			ScheduleToCloseTimeout: durationpb.New(10 * time.Minute),
		})
		env.Error(err)

		// Second start with same request ID should return existing run.
		resp, err := env.FrontendClient().StartNexusOperationExecution(ctx, &workflowservice.StartNexusOperationExecutionRequest{
			Namespace:              env.Namespace().String(),
			OperationId:            operationID,
			Endpoint:               "test-endpoint",
			Service:                "test-service",
			Operation:              "test-operation",
			RequestId:              env.Tv().RequestID(),
			ScheduleToCloseTimeout: durationpb.New(10 * time.Minute),
		})
		env.NoError(err)
		env.Equal(firstResp.RunId, resp.RunId)
		env.False(resp.GetStarted())
	})

	t.Run("IDConflictPolicy_UseExisting", func(t *testing.T) {
		env := testcore.NewEnv(t, nexusStandaloneOpts...)
		ctx := env.Context()

		operationID := testcore.RandomizeStr(t.Name())
		firstResp := startAndValidateNexusOperation(env, operationID)

		resp, err := env.FrontendClient().StartNexusOperationExecution(ctx, &workflowservice.StartNexusOperationExecutionRequest{
			Namespace:              env.Namespace().String(),
			OperationId:            operationID,
			Endpoint:               "test-endpoint",
			Service:                "test-service",
			Operation:              "test-operation",
			RequestId:              "different-request-id",
			IdConflictPolicy:       enumspb.NEXUS_OPERATION_ID_CONFLICT_POLICY_USE_EXISTING,
			ScheduleToCloseTimeout: durationpb.New(10 * time.Minute),
		})
		env.NoError(err)
		env.Equal(firstResp.RunId, resp.RunId)
		env.False(resp.GetStarted())
	})
}

func startAndValidateNexusOperation(
	env *testcore.TestEnv,
	operationID string,
) *workflowservice.StartNexusOperationExecutionResponse {
	startResp, err := env.FrontendClient().StartNexusOperationExecution(env.Context(), &workflowservice.StartNexusOperationExecutionRequest{
		Namespace:              env.Namespace().String(),
		OperationId:            operationID,
		Endpoint:               "test-endpoint",
		Service:                "test-service",
		Operation:              "test-operation",
		RequestId:              env.Tv().RequestID(),
		ScheduleToCloseTimeout: durationpb.New(10 * time.Minute),
	})
	env.NoError(err)
	env.NotEmpty(startResp.GetRunId())
	env.True(startResp.GetStarted())
	return startResp
}
