package moves

import (
	"fmt"

	"go.temporal.io/server/api/historyservice/v1"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// StartWorkflow represents a workflow being started.
type StartWorkflow struct {
	Request  *historyservice.StartWorkflowExecutionRequest
	Identity *lineuptypes.Identity
}

func (e *StartWorkflow) MoveType() string {
	return "StartWorkflow"
}

func (e *StartWorkflow) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses a StartWorkflow from a gRPC request, mutating the receiver.
// Returns nil on success, error on failure.
func (e *StartWorkflow) Parse(input any) error {
	req, ok := input.(*historyservice.StartWorkflowExecutionRequest)
	if !ok || req == nil {
		return fmt.Errorf("invalid input type for StartWorkflow")
	}

	if req.StartRequest == nil || req.StartRequest.WorkflowId == "" {
		return fmt.Errorf("missing required fields in StartWorkflowExecutionRequest")
	}

	e.Request = req
	// e.Identity should already be set by the router

	return nil
}
