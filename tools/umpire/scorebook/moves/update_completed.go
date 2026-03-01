package moves

import (
	"fmt"

	historyv1 "go.temporal.io/api/history/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// UpdateCompleted represents a workflow update being completed by a worker.
// This corresponds to the EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_COMPLETED history event.
type UpdateCompleted struct {
	Attributes *historyv1.WorkflowExecutionUpdateCompletedEventAttributes
	Identity   *lineuptypes.Identity
}

func (e *UpdateCompleted) MoveType() string {
	return "UpdateCompleted"
}

func (e *UpdateCompleted) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an UpdateCompleted from a history event, mutating the receiver.
// Returns nil on success, error on failure.
func (e *UpdateCompleted) Parse(input any) error {
	attrs, ok := input.(*historyv1.WorkflowExecutionUpdateCompletedEventAttributes)
	if !ok || attrs == nil {
		return fmt.Errorf("invalid input type for UpdateCompleted")
	}

	if attrs.Meta == nil || attrs.Meta.UpdateId == "" {
		return fmt.Errorf("missing required fields in UpdateCompleted")
	}

	e.Attributes = attrs
	// e.Identity should already be set by the router

	return nil
}
