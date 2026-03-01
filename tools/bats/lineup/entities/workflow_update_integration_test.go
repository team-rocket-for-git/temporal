package entities_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	commonv1 "go.temporal.io/api/common/v1"
	historyv1 "go.temporal.io/api/history/v1"
	updatev1 "go.temporal.io/api/update/v1"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tools/bats/lineup"
	"go.temporal.io/server/tools/bats/lineup/entities"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

// TestWorkflowUpdateEntityIntegration verifies the WorkflowUpdate entity correctly processes moves.
func TestWorkflowUpdateEntityIntegration(t *testing.T) {
	// Setup
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	updateID := "test-update-123"
	workflowID := "test-workflow-456"
	handlerName := "myUpdateHandler"

	// Create identity for the update
	updateEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowUpdateType, updateID)
	workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
	updateIdentity := &lineuptypes.Identity{
		EntityID: updateEntityID,
		ParentID: &workflowEntityID,
	}

	// Simulate update lifecycle: Request -> Admitted -> Accepted -> Completed
	events := []scorebooktypes.Move{
		// 1. UpdateWorkflowExecutionRequest
		&moves.UpdateWorkflowExecutionRequest{
			Request: &historyservice.UpdateWorkflowExecutionRequest{
				NamespaceId: "test-namespace",
				Request: &workflowservicev1.UpdateWorkflowExecutionRequest{
					WorkflowExecution: &commonv1.WorkflowExecution{
						WorkflowId: workflowID,
					},
					Request: &updatev1.Request{
						Meta: &updatev1.Meta{
							UpdateId: updateID,
						},
						Input: &updatev1.Input{
							Name: handlerName,
						},
					},
				},
			},
			Identity: updateIdentity,
		},
		// 2. UpdateAdmitted
		&moves.UpdateAdmitted{
			Attributes: &historyv1.WorkflowExecutionUpdateAdmittedEventAttributes{
				Request: &updatev1.Request{
					Meta: &updatev1.Meta{
						UpdateId: updateID,
					},
					Input: &updatev1.Input{
						Name: handlerName,
					},
				},
			},
			Identity: updateIdentity,
		},
		// 3. UpdateAccepted
		&moves.UpdateAccepted{
			Attributes: &historyv1.WorkflowExecutionUpdateAcceptedEventAttributes{
				ProtocolInstanceId: "proto-instance-123",
				AcceptedRequest: &updatev1.Request{
					Meta: &updatev1.Meta{
						UpdateId: updateID,
					},
				},
			},
			Identity: updateIdentity,
		},
		// 4. UpdateCompleted
		&moves.UpdateCompleted{
			Attributes: &historyv1.WorkflowExecutionUpdateCompletedEventAttributes{
				Meta: &updatev1.Meta{
					UpdateId: updateID,
				},
				AcceptedEventId: 5,
				Outcome: &updatev1.Outcome{
					Value: &updatev1.Outcome_Success{},
				},
			},
			Identity: updateIdentity,
		},
	}

	// Route events through registry
	err = registry.RouteEvents(context.Background(), events)
	require.NoError(t, err)

	// Query WorkflowUpdate entities
	updateEntities := registry.QueryEntities(entities.NewWorkflowUpdate())
	require.Len(t, updateEntities, 1, "Should have exactly one WorkflowUpdate entity")

	// Verify entity state
	wu := updateEntities[0].(*entities.WorkflowUpdate)
	require.Equal(t, updateID, wu.UpdateID)
	require.Equal(t, workflowID, wu.WorkflowID)
	require.Equal(t, handlerName, wu.HandlerName)
	require.Equal(t, "completed", wu.FSM.Current())
	require.Equal(t, "success", wu.Outcome)

	// Verify timestamps
	require.False(t, wu.AdmittedAt.IsZero(), "AdmittedAt should be set")
	require.False(t, wu.AcceptedAt.IsZero(), "AcceptedAt should be set")
	require.False(t, wu.CompletedAt.IsZero(), "CompletedAt should be set")
	require.True(t, wu.RejectedAt.IsZero(), "RejectedAt should not be set")

	// Verify timestamp ordering
	require.True(t, wu.AcceptedAt.After(wu.AdmittedAt), "AcceptedAt should be after AdmittedAt")
	require.True(t, wu.CompletedAt.After(wu.AcceptedAt), "CompletedAt should be after AcceptedAt")

	// Verify deduplication tracking
	require.Equal(t, 1, wu.RequestCount, "Should have received 1 request")
	require.False(t, wu.FirstSeenAt.IsZero(), "FirstSeenAt should be set")
}

// TestWorkflowUpdateDeduplication verifies duplicate update requests are tracked.
func TestWorkflowUpdateDeduplication(t *testing.T) {
	// Setup
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	updateID := "test-update-dedup"
	workflowID := "test-workflow-dedup"

	// Create identity
	updateEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowUpdateType, updateID)
	workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
	updateIdentity := &lineuptypes.Identity{
		EntityID: updateEntityID,
		ParentID: &workflowEntityID,
	}

	// Send the same update request 3 times (simulating deduplication)
	for i := 0; i < 3; i++ {
		event := &moves.UpdateWorkflowExecutionRequest{
			Request: &historyservice.UpdateWorkflowExecutionRequest{
				NamespaceId: "test-namespace",
				Request: &workflowservicev1.UpdateWorkflowExecutionRequest{
					WorkflowExecution: &commonv1.WorkflowExecution{
						WorkflowId: workflowID,
					},
					Request: &updatev1.Request{
						Meta: &updatev1.Meta{
							UpdateId: updateID,
						},
						Input: &updatev1.Input{
							Name: "testHandler",
						},
					},
				},
			},
			Identity: updateIdentity,
		}

		err = registry.RouteEvents(context.Background(), []scorebooktypes.Move{event})
		require.NoError(t, err)

		// Small delay to ensure LastSeenAt changes
		time.Sleep(1 * time.Millisecond)
	}

	// Verify deduplication tracking
	updateEntities := registry.QueryEntities(entities.NewWorkflowUpdate())
	require.Len(t, updateEntities, 1, "Should still have only one WorkflowUpdate entity")

	wu := updateEntities[0].(*entities.WorkflowUpdate)
	require.Equal(t, 3, wu.RequestCount, "Should have received 3 duplicate requests")
	require.False(t, wu.FirstSeenAt.IsZero(), "FirstSeenAt should be set")
	require.False(t, wu.LastSeenAt.IsZero(), "LastSeenAt should be set")
	require.True(t, wu.LastSeenAt.After(wu.FirstSeenAt), "LastSeenAt should be after FirstSeenAt")
}

// TestWorkflowUpdateRejection verifies update rejection flow.
func TestWorkflowUpdateRejection(t *testing.T) {
	// Setup
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	updateID := "test-update-rejected"
	workflowID := "test-workflow-rejected"

	updateEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowUpdateType, updateID)
	workflowEntityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
	updateIdentity := &lineuptypes.Identity{
		EntityID: updateEntityID,
		ParentID: &workflowEntityID,
	}

	// Simulate update lifecycle: Request -> Admitted -> Accepted -> Rejected
	events := []scorebooktypes.Move{
		&moves.UpdateWorkflowExecutionRequest{
			Request: &historyservice.UpdateWorkflowExecutionRequest{
				Request: &workflowservicev1.UpdateWorkflowExecutionRequest{
					WorkflowExecution: &commonv1.WorkflowExecution{
						WorkflowId: workflowID,
					},
					Request: &updatev1.Request{
						Meta: &updatev1.Meta{
							UpdateId: updateID,
						},
					},
				},
			},
			Identity: updateIdentity,
		},
		&moves.UpdateAdmitted{
			Attributes: &historyv1.WorkflowExecutionUpdateAdmittedEventAttributes{
				Request: &updatev1.Request{
					Meta: &updatev1.Meta{
						UpdateId: updateID,
					},
				},
			},
			Identity: updateIdentity,
		},
		&moves.UpdateAccepted{
			Attributes: &historyv1.WorkflowExecutionUpdateAcceptedEventAttributes{
				ProtocolInstanceId: "proto-123",
			},
			Identity: updateIdentity,
		},
		&moves.UpdateRejected{
			Attributes: &historyv1.WorkflowExecutionUpdateRejectedEventAttributes{
				ProtocolInstanceId: "proto-123",
				RejectedRequest: &updatev1.Request{
					Meta: &updatev1.Meta{
						UpdateId: updateID,
					},
				},
			},
			Identity: updateIdentity,
		},
	}

	err = registry.RouteEvents(context.Background(), events)
	require.NoError(t, err)

	// Verify entity transitioned to rejected
	updateEntities := registry.QueryEntities(entities.NewWorkflowUpdate())
	require.Len(t, updateEntities, 1)

	wu := updateEntities[0].(*entities.WorkflowUpdate)
	require.Equal(t, "rejected", wu.FSM.Current())
	require.False(t, wu.RejectedAt.IsZero(), "RejectedAt should be set")
	require.True(t, wu.CompletedAt.IsZero(), "CompletedAt should not be set")
}
