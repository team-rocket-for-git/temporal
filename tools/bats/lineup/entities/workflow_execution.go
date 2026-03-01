package entities

import (
	"context"
	"fmt"
	"time"

	"github.com/looplab/fsm"
	"go.temporal.io/server/tools/bats/lineup"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

var _ lineup.Entity = (*WorkflowExecution)(nil)


// WorkflowExecution represents a specific workflow execution (run).
// A workflow can have multiple executions (runs) due to continue-as-new or retries.
// Locking is handled by the registry - individual methods should not lock.
type WorkflowExecution struct {
	WorkflowID  string
	RunID       string
	NamespaceID string
	FSM         *fsm.FSM
	StartedAt   time.Time // When this run was started
	CompletedAt time.Time // When this run was completed
	LastSeenAt  time.Time // Last time any event was received for this run
}

// NewWorkflowExecution creates a new WorkflowExecution entity.
func NewWorkflowExecution() *WorkflowExecution {
	we := &WorkflowExecution{}
	we.FSM = fsm.NewFSM(
		"created",
		fsm.Events{
			{Name: "start", Src: []string{"created"}, Dst: "started"},
			{Name: "complete", Src: []string{"started"}, Dst: "completed"},
		},
		fsm.Callbacks{},
	)
	return we
}

func (we *WorkflowExecution) Type() lineuptypes.EntityType {
	return lineuptypes.WorkflowExecutionType
}

func (we *WorkflowExecution) OnEvent(identity *lineuptypes.Identity, iter scorebooktypes.MoveIterator) error {
	// Extract RunID from the entity identity
	if we.RunID == "" && identity != nil {
		we.RunID = identity.EntityID.ID
	}

	iter(func(ev scorebooktypes.Move) bool {
		switch e := ev.(type) {
		case *moves.StartWorkflow:
			if we.WorkflowID == "" && e.Request != nil && e.Request.StartRequest != nil {
				we.WorkflowID = e.Request.StartRequest.WorkflowId
				we.NamespaceID = e.Request.NamespaceId
			}
			// Update on first start event
			if we.FSM.Can("start") {
				_ = we.FSM.Event(context.Background(), "start")
				we.StartedAt = time.Now()
			}
			we.LastSeenAt = time.Now()

		case *moves.RespondWorkflowTaskCompleted:
			// Mark workflow execution as completed when we see a task completion response
			if we.FSM.Can("complete") {
				_ = we.FSM.Event(context.Background(), "complete")
				we.CompletedAt = time.Now()
			}
			we.LastSeenAt = time.Now()
		}
		return true
	})

	return nil
}

func (we *WorkflowExecution) String() string {
	return fmt.Sprintf("WorkflowExecution{workflowID=%s, runID=%s, state=%s}",
		we.WorkflowID, we.RunID, we.FSM.Current())
}
