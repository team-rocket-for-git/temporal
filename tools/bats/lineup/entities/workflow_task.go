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

var _ lineup.Entity = (*WorkflowTask)(nil)


// WorkflowTask represents a workflow task entity.
// Locking is handled by the registry - individual methods should not lock.
type WorkflowTask struct {
	TaskQueue  string
	WorkflowID string
	RunID      string
	FSM        *fsm.FSM
	AddedAt    time.Time
	PolledAt   time.Time
	StoredAt   time.Time // When task was stored to persistence (removed from matching)
}

// NewWorkflowTask creates a new WorkflowTask entity.
func NewWorkflowTask() *WorkflowTask {
	wt := &WorkflowTask{}
	wt.FSM = fsm.NewFSM(
		"created",
		fsm.Events{
			{Name: "add", Src: []string{"created"}, Dst: "added"},
			{Name: "poll", Src: []string{"added"}, Dst: "polled"},
			{Name: "store", Src: []string{"added"}, Dst: "stored"}, // Task removed from matching, persisted
		},
		fsm.Callbacks{},
	)
	return wt
}

func (wt *WorkflowTask) Type() lineuptypes.EntityType {
	return lineuptypes.WorkflowTaskType
}

func (wt *WorkflowTask) OnEvent(_ *lineuptypes.Identity, iter scorebooktypes.MoveIterator) error {
	iter(func(ev scorebooktypes.Move) bool {
		switch e := ev.(type) {
		case *moves.AddWorkflowTask:
			if wt.TaskQueue == "" && e.Request != nil {
				if e.Request.TaskQueue != nil {
					wt.TaskQueue = e.Request.TaskQueue.Name
				}
				if e.Request.Execution != nil {
					wt.WorkflowID = e.Request.Execution.WorkflowId
					wt.RunID = e.Request.Execution.RunId
				}
			}

			if wt.FSM.Can("add") {
				_ = wt.FSM.Event(context.Background(), "add")
				wt.AddedAt = time.Now()
			}
		case *moves.PollWorkflowTask:
			if wt.FSM.Can("poll") && e.TaskReturned {
				_ = wt.FSM.Event(context.Background(), "poll")
				wt.PolledAt = time.Now()
			}
		case *moves.StoreWorkflowTask:
			if wt.FSM.Can("store") {
				_ = wt.FSM.Event(context.Background(), "store")
				wt.StoredAt = time.Now()
			}
		}
		return true
	})

	return nil
}

func (wt *WorkflowTask) String() string {
	return fmt.Sprintf("WorkflowTask{taskQueue=%s, workflow=%s:%s, state=%s}",
		wt.TaskQueue, wt.WorkflowID, wt.RunID, wt.FSM.Current())
}
