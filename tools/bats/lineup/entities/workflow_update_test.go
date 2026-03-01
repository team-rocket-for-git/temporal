package entities_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tools/bats/lineup"
	"go.temporal.io/server/tools/bats/lineup/entities"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// TestWorkflowUpdateFSMTransitions verifies the FSM state transitions for WorkflowUpdate entity
func TestWorkflowUpdateFSMTransitions(t *testing.T) {
	logger := log.NewNoopLogger()
	registry, err := lineup.NewEntityRegistry(logger, "")
	require.NoError(t, err)
	defer registry.Close()

	entities.RegisterDefaultEntities(registry)

	// Create a new WorkflowUpdate entity
	wu := entities.NewWorkflowUpdate()
	require.NotNil(t, wu)
	require.NotNil(t, wu.FSM)

	// Verify initial state
	require.Equal(t, "unspecified", wu.FSM.Current())

	// Test transition: unspecified -> admitted
	err = wu.FSM.Event(context.Background(), "admit")
	require.NoError(t, err)
	require.Equal(t, "admitted", wu.FSM.Current())

	// Test transition: admitted -> accepted
	err = wu.FSM.Event(context.Background(), "accept")
	require.NoError(t, err)
	require.Equal(t, "accepted", wu.FSM.Current())

	// Test transition: accepted -> completed
	err = wu.FSM.Event(context.Background(), "complete")
	require.NoError(t, err)
	require.Equal(t, "completed", wu.FSM.Current())
}

// TestWorkflowUpdateFSMRejection verifies rejection transitions from various states
func TestWorkflowUpdateFSMRejection(t *testing.T) {
	testCases := []struct {
		name        string
		startState  string
		transitions []string
	}{
		{
			name:        "reject from unspecified",
			startState:  "unspecified",
			transitions: []string{"reject"},
		},
		{
			name:        "reject from admitted",
			startState:  "admitted",
			transitions: []string{"admit", "reject"},
		},
		{
			name:        "reject from accepted",
			startState:  "accepted",
			transitions: []string{"admit", "accept", "reject"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wu := entities.NewWorkflowUpdate()
			require.Equal(t, "unspecified", wu.FSM.Current())

			// Apply all transitions
			for _, transition := range tc.transitions {
				err := wu.FSM.Event(context.Background(), transition)
				require.NoError(t, err)
			}

			// Verify final state is rejected
			require.Equal(t, "rejected", wu.FSM.Current())
		})
	}
}

// TestWorkflowUpdateType verifies the entity type
func TestWorkflowUpdateType(t *testing.T) {
	wu := entities.NewWorkflowUpdate()
	require.Equal(t, lineuptypes.WorkflowUpdateType, wu.Type())
}

// TestWorkflowUpdateInvalidTransitions verifies that invalid state transitions are rejected
func TestWorkflowUpdateInvalidTransitions(t *testing.T) {
	testCases := []struct {
		name           string
		setupState     string
		transitions    []string
		invalidEvent   string
		expectedError  bool
	}{
		{
			name:          "cannot accept from unspecified",
			setupState:    "unspecified",
			transitions:   []string{},
			invalidEvent:  "accept",
			expectedError: true,
		},
		{
			name:          "cannot complete from admitted",
			setupState:    "admitted",
			transitions:   []string{"admit"},
			invalidEvent:  "complete",
			expectedError: true,
		},
		{
			name:          "cannot transition from completed",
			setupState:    "completed",
			transitions:   []string{"admit", "accept", "complete"},
			invalidEvent:  "admit",
			expectedError: true,
		},
		{
			name:          "cannot transition from rejected",
			setupState:    "rejected",
			transitions:   []string{"reject"},
			invalidEvent:  "admit",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wu := entities.NewWorkflowUpdate()

			// Setup: apply transitions to get to desired state
			for _, transition := range tc.transitions {
				err := wu.FSM.Event(context.Background(), transition)
				require.NoError(t, err)
			}

			require.Equal(t, tc.setupState, wu.FSM.Current())

			// Attempt invalid transition
			err := wu.FSM.Event(context.Background(), tc.invalidEvent)
			if tc.expectedError {
				require.Error(t, err, "Expected error for invalid transition from %s with event %s", tc.setupState, tc.invalidEvent)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestWorkflowUpdateFields verifies that all fields can be set
func TestWorkflowUpdateFields(t *testing.T) {
	wu := entities.NewWorkflowUpdate()

	// Set identity fields
	wu.UpdateID = "update-123"
	wu.WorkflowID = "workflow-456"
	wu.HandlerName = "myUpdateHandler"

	require.Equal(t, "update-123", wu.UpdateID)
	require.Equal(t, "workflow-456", wu.WorkflowID)
	require.Equal(t, "myUpdateHandler", wu.HandlerName)

	// Set state tracking fields
	wu.IsSpeculative = true
	wu.Outcome = "success"

	require.True(t, wu.IsSpeculative)
	require.Equal(t, "success", wu.Outcome)

	// Set deduplication tracking
	wu.RequestCount = 3
	require.Equal(t, 3, wu.RequestCount)
}
