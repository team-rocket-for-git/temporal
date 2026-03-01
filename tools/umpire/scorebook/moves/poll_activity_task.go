package moves

import (
	"fmt"

	"go.temporal.io/server/api/matchingservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// PollActivityTask represents an activity task being polled.
type PollActivityTask struct {
	Request      *matchingservice.PollActivityTaskQueueRequest
	Response     *matchingservice.PollActivityTaskQueueResponse
	Identity     *lineuptypes.Identity
	TaskReturned bool
}

func (e *PollActivityTask) MoveType() string {
	return "PollActivityTask"
}

func (e *PollActivityTask) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses a PollActivityTask from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *PollActivityTask) Parse(input any) error {
	req, ok := input.(*matchingservice.PollActivityTaskQueueRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for PollActivityTask")
	}

	if req.PollRequest == nil || req.PollRequest.TaskQueue == nil || req.PollRequest.TaskQueue.Name == "" {
		return fmt.Errorf("missing required fields in PollActivityTaskQueueRequest")
	}

	e.Request = req
	e.Response = nil // Response not available at parse time
	e.TaskReturned = false
	// e.Identity should already be set by the router

	return nil
}
