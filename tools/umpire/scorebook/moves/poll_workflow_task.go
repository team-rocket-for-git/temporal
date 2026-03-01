package moves

import (
	"fmt"

	"go.temporal.io/server/api/matchingservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// PollWorkflowTask represents a workflow task being polled.
type PollWorkflowTask struct {
	Request      *matchingservice.PollWorkflowTaskQueueRequest
	Response     *matchingservice.PollWorkflowTaskQueueResponse
	Identity     *lineuptypes.Identity
	TaskReturned bool
}

func (e *PollWorkflowTask) MoveType() string {
	return "PollWorkflowTask"
}

func (e *PollWorkflowTask) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses a PollWorkflowTask from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *PollWorkflowTask) Parse(input any) error {
	req, ok := input.(*matchingservice.PollWorkflowTaskQueueRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for PollWorkflowTask")
	}

	if req.PollRequest == nil || req.PollRequest.TaskQueue == nil || req.PollRequest.TaskQueue.Name == "" {
		return fmt.Errorf("missing required fields in PollWorkflowTaskQueueRequest")
	}

	e.Request = req
	e.Response = nil // Response not available at parse time
	e.TaskReturned = false
	// e.Identity should already be set by the router

	return nil
}
