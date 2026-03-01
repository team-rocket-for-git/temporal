package moves

import (
	"fmt"

	historyv1 "go.temporal.io/api/history/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// UpdateAdmitted represents a workflow update being admitted by the history service.
// This corresponds to the EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ADMITTED history event.
type UpdateAdmitted struct {
	Attributes *historyv1.WorkflowExecutionUpdateAdmittedEventAttributes
	Identity   *lineuptypes.Identity
}

func (e *UpdateAdmitted) MoveType() string {
	return "UpdateAdmitted"
}

func (e *UpdateAdmitted) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an UpdateAdmitted from a history event, mutating the receiver.
// Returns nil on success, error on failure.
func (e *UpdateAdmitted) Parse(input any) error {
	attrs, ok := input.(*historyv1.WorkflowExecutionUpdateAdmittedEventAttributes)
	if !ok || attrs == nil {
		return fmt.Errorf("invalid input type for UpdateAdmitted")
	}

	if attrs.Request == nil || attrs.Request.Meta == nil || attrs.Request.Meta.UpdateId == "" {
		return fmt.Errorf("missing required fields in UpdateAdmitted")
	}

	e.Attributes = attrs
	// e.Identity should already be set by the router

	return nil
}
