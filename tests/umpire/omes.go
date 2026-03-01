// Package kitchensink provides Omes kitchensink workflow for CATCH testing
package catch

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	omeskitchensink "github.com/temporalio/omes/workers/go/kitchensink"
)

// KitchenSinkWorkflow is the Omes kitchensink workflow
// This is re-exported from github.com/temporalio/omes/workers/go/kitchensink
var KitchenSinkWorkflow = omeskitchensink.KitchenSinkWorkflow

// RegisterWorkflows registers the kitchensink workflow and activities with a worker
func RegisterWorkflows(w worker.Worker) {
	w.RegisterWorkflowWithOptions(KitchenSinkWorkflow, workflow.RegisterOptions{
		Name: "kitchensink",
	})
	// Register activities used by the kitchensink workflow
	// Use explicit names to match what the workflow expects
	w.RegisterActivityWithOptions(omeskitchensink.Noop, activity.RegisterOptions{
		Name: "noop",
	})
	w.RegisterActivityWithOptions(omeskitchensink.Payload, activity.RegisterOptions{
		Name: "payload",
	})
	w.RegisterActivityWithOptions(omeskitchensink.Delay, activity.RegisterOptions{
		Name: "delay",
	})
}
