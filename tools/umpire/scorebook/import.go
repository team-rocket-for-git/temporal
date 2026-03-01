package scorebook

import (
	"encoding/json"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/api/matchingservice/v1"
	"go.temporal.io/server/common/persistence"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	"go.temporal.io/server/tools/umpire/scorebook/types"
)

// SpanIterator is a function that yields spans one at a time.
// The yield function returns false to stop iteration early.
type SpanIterator func(yield func(ptrace.Span) bool)

// Importer converts gRPC requests and OTEL spans into canonical moves.
type Importer struct {
	// Empty for now, may add configuration options later
}

// NewImporter creates a new importer.
func NewImporter() *Importer {
	return &Importer{}
}

// ImportRequest converts a gRPC request directly to a move.
// This is used by the gRPC interceptor to record moves synchronously.
// Returns nil if the request type is not recognized.
func (imp *Importer) ImportRequest(request any) types.Move {
	return FromRequest(request)
}

// ImportSpans converts OTEL spans to events.
// This is primarily for non-RPC OTEL events like persistence operations.
// RPC calls are now recorded directly via the gRPC interceptor.
func (imp *Importer) ImportSpans(iter SpanIterator) []types.Move {
	var events []types.Move

	iter(func(span ptrace.Span) bool {
		spanName := span.Name()

		// Check for persistence.TaskStore/CreateTasks spans
		if spanName == "persistence.TaskStore/CreateTasks" {
			// Get the request payload from span attributes
			attrs := span.Attributes()
			requestPayload, ok := attrs.Get("persistence.request.payload")
			if !ok {
				return true
			}

			// Parse the JSON payload to get InternalCreateTasksRequest
			var req persistence.InternalCreateTasksRequest
			if err := json.Unmarshal([]byte(requestPayload.Str()), &req); err != nil {
				return true
			}

			// Create a StoreWorkflowTask move for each task in the request
			// Note: We don't have full workflow execution details in the persistence layer,
			// so we create a simplified move with available information
			for range req.Tasks {
				move := &moves.StoreWorkflowTask{
					TaskQueue: req.TaskQueue,
				}

				// Compute identity from task queue
				if req.TaskQueue != "" {
					taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, req.TaskQueue)
					move.Identity = &lineuptypes.Identity{
						EntityID: taskQueueID,
					}
				}

				events = append(events, move)
			}
		}

		// TODO: Add history event processing for workflow updates
		// History events come from workflow execution and include:
		// - EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ADMITTED (UpdateAdmitted move)
		// - EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_ACCEPTED (UpdateAccepted move)
		// - EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_COMPLETED (UpdateCompleted move)
		// - EVENT_TYPE_WORKFLOW_EXECUTION_UPDATE_REJECTED (UpdateRejected move)
		//
		// These would need to be extracted from history service spans that contain
		// HistoryEvent payloads. Pattern would be similar to persistence events above.
		//
		// Example:
		// if spanName == "history.AddHistoryEvent" || spanName == "history.ProcessEvent" {
		//     // Extract history event from span attributes
		//     // Parse event type and attributes
		//     // Create corresponding move (UpdateAdmitted, UpdateAccepted, etc.)
		//     // Set identity based on UpdateID and WorkflowID from event
		// }

		return true
	})

	return events
}

// FromRequest creates a move from a gRPC request.
// Returns nil if the request type is not recognized or invalid.
func FromRequest(request any) types.Move {
	switch req := request.(type) {
	case *matchingservice.AddWorkflowTaskRequest:
		// AddWorkflowTask represents storing a task to the matching service
		// We track this as StoreWorkflowTask since it's the store operation
		move := &moves.StoreWorkflowTask{}

		if req.TaskQueue != nil && req.TaskQueue.Name != "" &&
			req.Execution != nil && req.Execution.WorkflowId != "" && req.Execution.RunId != "" {
			workflowTaskID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowTaskType,
				req.TaskQueue.Name+":"+req.Execution.WorkflowId+":"+req.Execution.RunId)
			taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, req.TaskQueue.Name)
			move.Identity = &lineuptypes.Identity{
				EntityID: workflowTaskID,
				ParentID: &taskQueueID,
			}
			move.TaskQueue = req.TaskQueue.Name
			move.WorkflowID = req.Execution.WorkflowId
			move.RunID = req.Execution.RunId
		}
		return move

	case *matchingservice.PollWorkflowTaskQueueRequest:
		move := &moves.PollWorkflowTask{}
		if req.PollRequest != nil && req.PollRequest.TaskQueue != nil && req.PollRequest.TaskQueue.Name != "" {
			taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, req.PollRequest.TaskQueue.Name)
			move.Identity = &lineuptypes.Identity{
				EntityID: taskQueueID,
			}
		}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	case *matchingservice.AddActivityTaskRequest:
		move := &moves.AddActivityTask{}
		if req.TaskQueue != nil && req.TaskQueue.Name != "" &&
			req.Execution != nil && req.Execution.WorkflowId != "" && req.Execution.RunId != "" {
			activityTaskID := lineuptypes.NewEntityIDFromType(lineuptypes.ActivityTaskType,
				req.TaskQueue.Name+":"+req.Execution.WorkflowId+":"+req.Execution.RunId)
			taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, req.TaskQueue.Name)
			move.Identity = &lineuptypes.Identity{
				EntityID: activityTaskID,
				ParentID: &taskQueueID,
			}
		}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	case *matchingservice.PollActivityTaskQueueRequest:
		move := &moves.PollActivityTask{}
		if req.PollRequest != nil && req.PollRequest.TaskQueue != nil && req.PollRequest.TaskQueue.Name != "" {
			taskQueueID := lineuptypes.NewEntityIDFromType(lineuptypes.TaskQueueType, req.PollRequest.TaskQueue.Name)
			move.Identity = &lineuptypes.Identity{
				EntityID: taskQueueID,
			}
		}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	case *historyservice.StartWorkflowExecutionRequest:
		move := &moves.StartWorkflow{}
		if req.StartRequest != nil && req.StartRequest.WorkflowId != "" {
			workflowID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, req.StartRequest.WorkflowId)
			move.Identity = &lineuptypes.Identity{
				EntityID: workflowID,
			}
		}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	case *historyservice.RespondWorkflowTaskCompletedRequest:
		move := &moves.RespondWorkflowTaskCompleted{}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	case *workflowservice.UpdateWorkflowExecutionRequest:
		// SDK calls go through the frontend service with workflowservice.UpdateWorkflowExecutionRequest
		move := &moves.UpdateWorkflowExecutionRequest{}
		// Extract identity from the update request
		if req.Request != nil && req.Request.Meta != nil && req.Request.Meta.UpdateId != "" {
			updateID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowUpdateType, req.Request.Meta.UpdateId)

			// Parent is the workflow entity
			var parentID *lineuptypes.EntityID
			if req.WorkflowExecution != nil && req.WorkflowExecution.WorkflowId != "" {
				workflowID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, req.WorkflowExecution.WorkflowId)
				parentID = &workflowID
			}

			move.Identity = &lineuptypes.Identity{
				EntityID: updateID,
				ParentID: parentID,
			}
		}
		// Convert to historyservice request format for parsing
		histReq := &historyservice.UpdateWorkflowExecutionRequest{
			NamespaceId: req.Namespace,
			Request:     req,
		}
		if err := move.Parse(histReq); err != nil {
			return nil
		}
		return move

	case *historyservice.UpdateWorkflowExecutionRequest:
		// History service internal calls
		move := &moves.UpdateWorkflowExecutionRequest{}
		// Extract identity from the update request
		if req.Request != nil && req.Request.Request != nil {
			updateReq := req.Request.Request
			if updateReq.Meta != nil && updateReq.Meta.UpdateId != "" {
				updateID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowUpdateType, updateReq.Meta.UpdateId)

				// Parent is the workflow entity
				var parentID *lineuptypes.EntityID
				if req.Request.WorkflowExecution != nil && req.Request.WorkflowExecution.WorkflowId != "" {
					workflowID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, req.Request.WorkflowExecution.WorkflowId)
					parentID = &workflowID
				}

				move.Identity = &lineuptypes.Identity{
					EntityID: updateID,
					ParentID: parentID,
				}
			}
		}
		if err := move.Parse(req); err != nil {
			return nil
		}
		return move

	default:
		return nil
	}
}
