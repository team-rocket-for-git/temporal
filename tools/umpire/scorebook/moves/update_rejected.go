package moves

import (
	"fmt"

	historyv1 "go.temporal.io/api/history/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// UpdateRejected represents a workflow update being rejected by a worker or the system.
// This corresponds to the EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_REJECTED history event.
type UpdateRejected struct {
	Attributes *historyv1.WorkflowExecutionUpdateRejectedEventAttributes
	Identity   *lineuptypes.Identity
}

func (e *UpdateRejected) MoveType() string {
	return "UpdateRejected"
}

func (e *UpdateRejected) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an UpdateRejected from a history event, mutating the receiver.
// Returns nil on success, error on failure.
func (e *UpdateRejected) Parse(input any) error {
	attrs, ok := input.(*historyv1.WorkflowExecutionUpdateRejectedEventAttributes)
	if !ok || attrs == nil {
		return fmt.Errorf("invalid input type for UpdateRejected")
	}

	if attrs.ProtocolInstanceId == "" {
		return fmt.Errorf("missing required fields in UpdateRejected")
	}

	e.Attributes = attrs
	// e.Identity should already be set by the router

	return nil
}
