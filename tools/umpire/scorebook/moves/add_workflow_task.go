package moves

import (
	"fmt"

	"go.temporal.io/server/api/matchingservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// AddWorkflowTask represents a workflow task being added to matching.
type AddWorkflowTask struct {
	Request  *matchingservice.AddWorkflowTaskRequest
	Identity *lineuptypes.Identity
}

func (e *AddWorkflowTask) MoveType() string {
	return "AddWorkflowTask"
}

func (e *AddWorkflowTask) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses an AddWorkflowTask from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *AddWorkflowTask) Parse(input any) error {
	req, ok := input.(*matchingservice.AddWorkflowTaskRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for AddWorkflowTask")
	}

	if req.TaskQueue == nil || req.TaskQueue.Name == "" {
		return fmt.Errorf("missing required fields in AddWorkflowTaskRequest")
	}

	e.Request = req
	// e.Identity should already be set by the router

	return nil
}
