package moves

import (
	"fmt"

	"go.temporal.io/server/api/matchingservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// AddActivityTask represents an activity task being added to matching.
type AddActivityTask struct {
	Request  *matchingservice.AddActivityTaskRequest
	Identity *lineuptypes.Identity
}

func (e *AddActivityTask) MoveType() string {
	return "AddActivityTask"
}

func (e *AddActivityTask) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an AddActivityTask from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *AddActivityTask) Parse(input any) error {
	req, ok := input.(*matchingservice.AddActivityTaskRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for AddActivityTask")
	}

	if req.TaskQueue == nil || req.TaskQueue.Name == "" {
		return fmt.Errorf("missing required fields in AddActivityTaskRequest")
	}

	e.Request = req
	// e.Identity should already be set by the router

	return nil
}
