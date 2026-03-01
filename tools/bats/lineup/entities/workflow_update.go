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

var _ lineup.Entity = (*WorkflowUpdate)(nil)

// WorkflowUpdate represents a workflow update entity.
// It tracks the lifecycle of an update from admission through completion.
// Parent: Workflow entity (identified by WorkflowID)
// Locking is handled by the registry - individual methods should not lock.
type WorkflowUpdate struct {
	// Identity
	UpdateID    string // Unique identifier for this update
	WorkflowID  string // Parent workflow ID
	HandlerName string // Update handler name (e.g., "myUpdateHandler")

	// Lifecycle tracking
	FSM *fsm.FSM // State machine: unspecified -> admitted -> accepted -> completed/rejected

	// Timestamps for lifecycle stages
	AdmittedAt  time.Time // When update was received by history service
	AcceptedAt  time.Time // When worker accepted the update
	CompletedAt time.Time // When worker completed the update with result
	RejectedAt  time.Time // When update was rejected (if applicable)

	// State tracking
	IsSpeculative bool   // Whether update is on a speculative workflow task
	Outcome       string // "success" or "failure" (set at completion)

	// Deduplication tracking
	RequestCount int       // Number of duplicate requests received
	FirstSeenAt  time.Time // When first request was received
	LastSeenAt   time.Time // When most recent request was received
}

// NewWorkflowUpdate creates a new WorkflowUpdate entity.
func NewWorkflowUpdate() *WorkflowUpdate {
	wu := &WorkflowUpdate{}
	wu.FSM = fsm.NewFSM(
		"unspecified",
		fsm.Events{
			// Valid state transitions based on update lifecycle
			{Name: "admit", Src: []string{"unspecified"}, Dst: "admitted"},
			{Name: "accept", Src: []string{"admitted"}, Dst: "accepted"},
			{Name: "complete", Src: []string{"accepted"}, Dst: "completed"},
			{Name: "reject", Src: []string{"unspecified", "admitted", "accepted"}, Dst: "rejected"},
		},
		fsm.Callbacks{},
	)
	return wu
}

func (wu *WorkflowUpdate) Type() lineuptypes.EntityType {
	return lineuptypes.WorkflowUpdateType
}

func (wu *WorkflowUpdate) OnEvent(_ *lineuptypes.Identity, iter scorebooktypes.MoveIterator) error {
	iter(func(ev scorebooktypes.Move) bool {
		ctx := context.Background()

		switch e := ev.(type) {
		case *moves.UpdateWorkflowExecutionRequest:
			// Track the request for deduplication
			if e.Request != nil && e.Request.Request != nil && e.Request.Request.Request != nil {
				req := e.Request.Request // workflowservice.UpdateWorkflowExecutionRequest
				updateReq := req.Request // update.Request

				// Initialize entity on first request
				if wu.UpdateID == "" && updateReq.Meta != nil {
					wu.UpdateID = updateReq.Meta.UpdateId
					if req.WorkflowExecution != nil {
						wu.WorkflowID = req.WorkflowExecution.WorkflowId
					}
					if updateReq.Input != nil {
						wu.HandlerName = updateReq.Input.Name
					}
					wu.FirstSeenAt = time.Now()
				}
				wu.RequestCount++
				wu.LastSeenAt = time.Now()
			}

		case *moves.UpdateAdmitted:
			// Process admission event
			if e.Attributes != nil && e.Attributes.Request != nil {
				// Initialize entity from admission event if not already set
				if wu.UpdateID == "" && e.Attributes.Request.Meta != nil {
					wu.UpdateID = e.Attributes.Request.Meta.UpdateId
					if e.Attributes.Request.Input != nil {
						wu.HandlerName = e.Attributes.Request.Input.Name
					}
				}

				// Transition to admitted state
				if wu.FSM.Can("admit") {
					_ = wu.FSM.Event(ctx, "admit")
					wu.AdmittedAt = time.Now()
				}
			}

		case *moves.UpdateAccepted:
			// Process acceptance event
			if e.Attributes != nil {
				if wu.FSM.Can("accept") {
					_ = wu.FSM.Event(ctx, "accept")
					wu.AcceptedAt = time.Now()
				}

				// Check if this update was on a speculative workflow task
				// This would need to be determined from the workflow task context
				// For now, we leave it as false (default)
			}

		case *moves.UpdateCompleted:
			// Process completion event
			if e.Attributes != nil {
				// Initialize UpdateID from completion event if not already set
				if wu.UpdateID == "" && e.Attributes.Meta != nil {
					wu.UpdateID = e.Attributes.Meta.UpdateId
				}

				if wu.FSM.Can("complete") {
					_ = wu.FSM.Event(ctx, "complete")
					wu.CompletedAt = time.Now()

					// Determine outcome based on the result
					if e.Attributes.Outcome != nil && e.Attributes.Outcome.GetSuccess() != nil {
						wu.Outcome = "success"
					} else {
						wu.Outcome = "failure"
					}
				}
			}

		case *moves.UpdateRejected:
			// Process rejection event
			if e.Attributes != nil {
				// Initialize UpdateID from rejection event if not already set
				if wu.UpdateID == "" && e.Attributes.RejectedRequest != nil && e.Attributes.RejectedRequest.Meta != nil {
					wu.UpdateID = e.Attributes.RejectedRequest.Meta.UpdateId
				}

				if wu.FSM.Can("reject") {
					_ = wu.FSM.Event(ctx, "reject")
					wu.RejectedAt = time.Now()
				}
			}
		}

		return true
	})

	return nil
}

func (wu *WorkflowUpdate) String() string {
	return fmt.Sprintf("WorkflowUpdate{updateID=%s, workflowID=%s, state=%s, handler=%s}",
		wu.UpdateID, wu.WorkflowID, wu.FSM.Current(), wu.HandlerName)
}
