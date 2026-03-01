package moves_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	historyv1 "go.temporal.io/api/history/v1"
	updatev1 "go.temporal.io/api/update/v1"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
)

// TestUpdateWorkflowExecutionRequestParse verifies parsing of UpdateWorkflowExecutionRequest
func TestUpdateWorkflowExecutionRequestParse(t *testing.T) {
	move := &moves.UpdateWorkflowExecutionRequest{}

	// Valid request
	req := &historyservice.UpdateWorkflowExecutionRequest{
		NamespaceId: "test-namespace",
		Request: &workflowservicev1.UpdateWorkflowExecutionRequest{
			Request: &updatev1.Request{
				Meta: &updatev1.Meta{
					UpdateId: "test-update-123",
				},
				Input: &updatev1.Input{
					Name: "myUpdateHandler",
				},
			},
		},
	}

	err := move.Parse(req)
	require.NoError(t, err)
	require.Equal(t, req, move.Request)
	require.Equal(t, "UpdateWorkflowExecutionRequest", move.MoveType())

	// Invalid request - missing Meta
	invalidReq := &historyservice.UpdateWorkflowExecutionRequest{
		Request: &workflowservicev1.UpdateWorkflowExecutionRequest{
			Request: &updatev1.Request{},
		},
	}
	move2 := &moves.UpdateWorkflowExecutionRequest{}
	err = move2.Parse(invalidReq)
	require.Error(t, err)
}

// TestUpdateAdmittedParse verifies parsing of UpdateAdmitted
func TestUpdateAdmittedParse(t *testing.T) {
	move := &moves.UpdateAdmitted{}

	attrs := &historyv1.WorkflowExecutionUpdateAdmittedEventAttributes{
		Request: &updatev1.Request{
			Meta: &updatev1.Meta{
				UpdateId: "test-update-123",
			},
			Input: &updatev1.Input{
				Name: "myUpdateHandler",
			},
		},
	}

	err := move.Parse(attrs)
	require.NoError(t, err)
	require.Equal(t, attrs, move.Attributes)
	require.Equal(t, "UpdateAdmitted", move.MoveType())
}

// TestUpdateAcceptedParse verifies parsing of UpdateAccepted
func TestUpdateAcceptedParse(t *testing.T) {
	move := &moves.UpdateAccepted{}

	attrs := &historyv1.WorkflowExecutionUpdateAcceptedEventAttributes{
		ProtocolInstanceId: "proto-instance-123",
		AcceptedRequest: &updatev1.Request{
			Meta: &updatev1.Meta{
				UpdateId: "test-update-123",
			},
		},
	}

	err := move.Parse(attrs)
	require.NoError(t, err)
	require.Equal(t, attrs, move.Attributes)
	require.Equal(t, "UpdateAccepted", move.MoveType())
}

// TestUpdateCompletedParse verifies parsing of UpdateCompleted
func TestUpdateCompletedParse(t *testing.T) {
	move := &moves.UpdateCompleted{}

	attrs := &historyv1.WorkflowExecutionUpdateCompletedEventAttributes{
		Meta: &updatev1.Meta{
			UpdateId: "test-update-123",
		},
		AcceptedEventId: 5,
		Outcome: &updatev1.Outcome{
			Value: &updatev1.Outcome_Success{},
		},
	}

	err := move.Parse(attrs)
	require.NoError(t, err)
	require.Equal(t, attrs, move.Attributes)
	require.Equal(t, "UpdateCompleted", move.MoveType())
}

// TestUpdateRejectedParse verifies parsing of UpdateRejected
func TestUpdateRejectedParse(t *testing.T) {
	move := &moves.UpdateRejected{}

	attrs := &historyv1.WorkflowExecutionUpdateRejectedEventAttributes{
		ProtocolInstanceId: "proto-instance-123",
		RejectedRequest: &updatev1.Request{
			Meta: &updatev1.Meta{
				UpdateId: "test-update-123",
			},
		},
	}

	err := move.Parse(attrs)
	require.NoError(t, err)
	require.Equal(t, attrs, move.Attributes)
	require.Equal(t, "UpdateRejected", move.MoveType())
}
