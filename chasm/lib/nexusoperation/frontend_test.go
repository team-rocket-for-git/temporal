package nexusoperation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
)

func TestIsStandaloneNexusOperationEnabled(t *testing.T) {
	for _, tc := range []struct {
		name          string
		enabled       bool
		chasmEnabled  bool
		chasmNexus    bool
		expectEnabled bool
	}{
		{"all enabled", true, true, true, true},
		{"nexus disabled", false, true, true, false},
		{"chasm disabled", true, false, true, false},
		{"chasm nexus disabled", true, true, false, false},
		{"all disabled", false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &frontendHandler{
				config: &Config{
					Enabled:           func() bool { return tc.enabled },
					ChasmEnabled:      func(string) bool { return tc.chasmEnabled },
					ChasmNexusEnabled: func(string) bool { return tc.chasmNexus },
				},
			}
			require.Equal(t, tc.expectEnabled, h.IsStandaloneNexusOperationEnabled("test-ns"))
		})
	}
}

func TestValidateStartNexusOperationExecutionRequest(t *testing.T) {
	config := &Config{
		MaxIDLengthLimit:       func() int { return 20 },
		MaxServiceNameLength:   func(string) int { return 20 },
		MaxOperationNameLength: func(string) int { return 20 },
	}

	for _, tc := range []struct {
		name   string
		mutate func(*workflowservice.StartNexusOperationExecutionRequest)
		errMsg string
	}{
		{
			name: "valid request",
		},
		{
			name:   "empty namespace",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) { r.Namespace = "" },
			errMsg: "namespace is required",
		},
		{
			name:   "empty operation id",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) { r.OperationId = "" },
			errMsg: "operation_id is required",
		},
		{
			name: "operation id exceeds length limit",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) {
				r.OperationId = "this-operation-id-is-too-long"
			},
			errMsg: "operation_id exceeds length limit",
		},
		{
			name:   "empty endpoint",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) { r.Endpoint = "" },
			errMsg: "endpoint is required",
		},
		{
			name:   "empty service",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) { r.Service = "" },
			errMsg: "service is required",
		},
		{
			name: "service name exceeds length limit",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) {
				r.Service = "this-service-name-is-too-long"
			},
			errMsg: "service name exceeds length limit",
		},
		{
			name:   "empty operation",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) { r.Operation = "" },
			errMsg: "operation is required",
		},
		{
			name: "operation name exceeds length limit",
			mutate: func(r *workflowservice.StartNexusOperationExecutionRequest) {
				r.Operation = "this-operation-name-is-too-long"
			},
			errMsg: "operation name exceeds length limit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validReq := &workflowservice.StartNexusOperationExecutionRequest{
				Namespace:   "default",
				OperationId: "operation-id",
				Endpoint:    "endpoint",
				Service:     "service",
				Operation:   "operation",
			}
			if tc.mutate != nil {
				tc.mutate(validReq)
			}
			err := validateStartNexusOperationExecutionRequest(validReq, config)
			if tc.errMsg != "" {
				var invalidArgErr *serviceerror.InvalidArgument
				require.ErrorAs(t, err, &invalidArgErr)
				require.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateDescribeNexusOperationExecutionRequest(t *testing.T) {
	config := &Config{
		MaxIDLengthLimit: func() int { return 20 },
	}

	for _, tc := range []struct {
		name   string
		mutate func(*workflowservice.DescribeNexusOperationExecutionRequest)
		errMsg string
	}{
		{
			name: "valid request",
		},
		{
			name:   "empty namespace",
			mutate: func(r *workflowservice.DescribeNexusOperationExecutionRequest) { r.Namespace = "" },
			errMsg: "namespace is required",
		},
		{
			name:   "empty operation id",
			mutate: func(r *workflowservice.DescribeNexusOperationExecutionRequest) { r.OperationId = "" },
			errMsg: "operation_id is required",
		},
		{
			name: "operation id exceeds length limit",
			mutate: func(r *workflowservice.DescribeNexusOperationExecutionRequest) {
				r.OperationId = "this-operation-id-is-too-long"
			},
			errMsg: "operation_id exceeds length limit",
		},
		{
			name: "run id exceeds length limit",
			mutate: func(r *workflowservice.DescribeNexusOperationExecutionRequest) {
				r.RunId = "this-run-id-is-too-long!!"
			},
			errMsg: "run_id exceeds length limit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validReq := &workflowservice.DescribeNexusOperationExecutionRequest{
				Namespace:   "default",
				OperationId: "operation-id",
			}
			if tc.mutate != nil {
				tc.mutate(validReq)
			}
			err := validateDescribeNexusOperationExecutionRequest(validReq, config)
			if tc.errMsg != "" {
				var invalidArgErr *serviceerror.InvalidArgument
				require.ErrorAs(t, err, &invalidArgErr)
				require.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
