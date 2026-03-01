package moves

import (
	"fmt"

	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// StoreWorkflowTask represents a workflow task being stored to persistence.
// This happens when a task is removed from matching (e.g., via admin API) and
// persisted to the database. The task is no longer in the matching service
// but still exists in persistence.
type StoreWorkflowTask struct {
	TaskQueue  string
	Identity   *lineuptypes.Identity
	WorkflowID string
	RunID      string
}

func (e *StoreWorkflowTask) MoveType() string {
	return "StoreWorkflowTask"
}

func (e *StoreWorkflowTask) TargetEntity() *lineuptypes.Identity {
	return e.Identity
}

// Parse parses a StoreWorkflowTask from input, mutating the receiver.
// Returns nil on success, error on failure.
// StoreWorkflowTask is not directly generated from gRPC requests.
func (e *StoreWorkflowTask) Parse(input any) error {
	// StoreWorkflowTask is not directly generated from gRPC requests
	// It would need to be generated from admin API calls or other sources
	return fmt.Errorf("StoreWorkflowTask cannot be parsed from gRPC requests")
}
