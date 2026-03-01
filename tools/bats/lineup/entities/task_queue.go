package entities

import (
	"time"

	"go.temporal.io/server/tools/bats/lineup"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

var _ lineup.Entity = (*TaskQueue)(nil)


// TaskQueue represents a task queue entity.
// Locking is handled by the registry - individual methods should not lock.
type TaskQueue struct {
	Name              string
	LastEmptyPollTime time.Time
}

// NewTaskQueue creates a new TaskQueue entity.
func NewTaskQueue() *TaskQueue {
	return &TaskQueue{}
}

func (tq *TaskQueue) Type() lineuptypes.EntityType {
	return lineuptypes.TaskQueueType
}

func (tq *TaskQueue) OnEvent(_ *lineuptypes.Identity, iter scorebooktypes.MoveIterator) error {
	iter(func(ev scorebooktypes.Move) bool {
		switch e := ev.(type) {
		case *moves.PollWorkflowTask:
			if tq.Name == "" && e.Request != nil && e.Request.PollRequest != nil && e.Request.PollRequest.TaskQueue != nil {
				tq.Name = e.Request.PollRequest.TaskQueue.Name
			}
			// Track empty polls (polls that returned nothing)
			if !e.TaskReturned {
				tq.LastEmptyPollTime = time.Now()
			}
		}
		return true
	})
	return nil
}
