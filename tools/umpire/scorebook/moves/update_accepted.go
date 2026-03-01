package moves

import (
	"fmt"

	historyv1 "go.temporal.io/api/history/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// UpdateAccepted represents a workflow update being accepted by a worker.
// This corresponds to the EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ACCEPTED history event.
type UpdateAccepted struct {
	Attributes *historyv1.WorkflowExecutionUpdateAcceptedEventAttributes
	Identity   *lineuptypes.Identity
}

func (e *UpdateAccepted) MoveType() string {
	return "UpdateAccepted"
}

func (e *UpdateAccepted) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an UpdateAccepted from a history event, mutating the receiver.
// Returns nil on success, error on failure.
func (e *UpdateAccepted) Parse(input any) error {
	attrs, ok := input.(*historyv1.WorkflowExecutionUpdateAcceptedEventAttributes)
	if !ok || attrs == nil {
		return fmt.Errorf("invalid input type for UpdateAccepted")
	}

	if attrs.ProtocolInstanceId == "" {
		return fmt.Errorf("missing required fields in UpdateAccepted")
	}

	e.Attributes = attrs
	// e.Identity should already be set by the router

	return nil
}
