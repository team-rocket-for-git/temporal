package entities

import (
	"go.temporal.io/server/tools/bats/lineup"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
)

// RegisterDefaultEntities registers the default entity types with a registry.
// This is separated from the lineup package to avoid import cycles.
func RegisterDefaultEntities(r *lineup.Registry) {
	r.RegisterEntity(NewTaskQueue(), func() lineup.Entity {
		return NewTaskQueue()
	}, &moves.AddWorkflowTask{}, &moves.PollWorkflowTask{}, &moves.AddActivityTask{}, &moves.PollActivityTask{})

	r.RegisterEntity(NewWorkflowTask(), func() lineup.Entity {
		return NewWorkflowTask()
	}, &moves.AddWorkflowTask{}, &moves.PollWorkflowTask{}, &moves.StoreWorkflowTask{})

	r.RegisterEntity(NewWorkflowUpdate(), func() lineup.Entity {
		return NewWorkflowUpdate()
	}, &moves.UpdateWorkflowExecutionRequest{}, &moves.UpdateAdmitted{}, &moves.UpdateAccepted{}, &moves.UpdateCompleted{}, &moves.UpdateRejected{})
}
