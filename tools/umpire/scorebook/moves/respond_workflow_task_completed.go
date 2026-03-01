package moves

import (
	"fmt"

	"go.temporal.io/server/api/historyservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// RespondWorkflowTaskCompleted represents a workflow task completion response.
type RespondWorkflowTaskCompleted struct {
	Request  *historyservice.RespondWorkflowTaskCompletedRequest
	Response *historyservice.RespondWorkflowTaskCompletedResponse
	Identity *lineuptypes.Identity
}

func (e *RespondWorkflowTaskCompleted) MoveType() string {
	return "RespondWorkflowTaskCompleted"
}

func (e *RespondWorkflowTaskCompleted) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses a RespondWorkflowTaskCompleted from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *RespondWorkflowTaskCompleted) Parse(input any) error {
	req, ok := input.(*historyservice.RespondWorkflowTaskCompletedRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for RespondWorkflowTaskCompleted")
	}

	if req.CompleteRequest == nil {
		return fmt.Errorf("missing required fields in RespondWorkflowTaskCompletedRequest")
	}

	e.Request = req
	e.Response = nil // Response not available at parse time
	// e.Identity should already be set by the router

	return nil
}
