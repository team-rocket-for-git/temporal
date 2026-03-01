package moves

import (
	"fmt"

	"go.temporal.io/server/api/historyservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// UpdateWorkflowExecutionRequest represents a workflow update request being sent to the history service.
// This is the admission stage of an update's lifecycle.
type UpdateWorkflowExecutionRequest struct {
	Request  *historyservice.UpdateWorkflowExecutionRequest
	Response *historyservice.UpdateWorkflowExecutionResponse
	Identity *lineuptypes.Identity
}

func (e *UpdateWorkflowExecutionRequest) MoveType() string {
	return "UpdateWorkflowExecutionRequest"
}

func (e *UpdateWorkflowExecutionRequest) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an UpdateWorkflowExecutionRequest from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *UpdateWorkflowExecutionRequest) Parse(input any) error {
	req, ok := input.(*historyservice.UpdateWorkflowExecutionRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for UpdateWorkflowExecutionRequest")
	}

	if req.Request == nil || req.Request.Request == nil || req.Request.Request.Meta == nil || req.Request.Request.Meta.UpdateId == "" {
		return fmt.Errorf("missing required fields in UpdateWorkflowExecutionRequest")
	}

	e.Request = req
	e.Response = nil // Response not available at parse time
	// e.Identity should already be set by the router

	return nil
}
