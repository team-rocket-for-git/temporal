package types

import "strings"

// Identifiable represents anything that can produce a canonical identity string.
// Both legacy EntityID and new hierarchical identities implement this.
type Identifiable interface {
	String() string
}

// =============================================================================
// Legacy types for scorebook/moves compatibility
// =============================================================================

// TypedEntity represents any entity that has a type.
type TypedEntity interface {
	Type() EntityType
}

// EntityType is a strongly-typed entity type identifier.
type EntityType string

// Common entity type constants.
const (
	ActivityTaskType      EntityType = "ActivityTask"
	TaskQueueType         EntityType = "TaskQueue"
	WorkflowType          EntityType = "Workflow"
	WorkflowTaskType      EntityType = "WorkflowTask"
	WorkflowUpdateType    EntityType = "WorkflowUpdate"
	WorkflowExecutionType EntityType = "WorkflowExecution"
	NamespaceType         EntityType = "Namespace"
)

// EntityID uniquely identifies an entity within its type.
// For observability, prefer using hierarchical identities (Namespace().Workflow()...).
type EntityID struct {
	Type EntityType
	ID   string
}

// String implements Identifiable.
func (e EntityID) String() string {
	return string(e.Type) + ":" + e.ID
}

// NewEntityID creates an EntityID from a typed entity instance.
func NewEntityID(e TypedEntity, id string) EntityID {
	return EntityID{Type: e.Type(), ID: id}
}

// NewEntityIDFromType creates an EntityID from an entity type and ID string.
func NewEntityIDFromType(entityType EntityType, id string) EntityID {
	return EntityID{Type: entityType, ID: id}
}

// Identity represents the full identity of an entity with optional parent.
// Used by moves/scorebook for entity tracking.
type Identity struct {
	EntityID EntityID
	ParentID *EntityID
}

// =============================================================================
// Hierarchical identity builders for observability
// These produce canonical identity strings like:
//   "namespace:ns1/workflow:wf1/execution:run1"
// =============================================================================

// NamespaceID is the root identity for namespace-scoped entities.
type NamespaceID struct {
	id string
}

// Namespace creates a namespace identity - the root of most entity hierarchies.
func Namespace(id string) NamespaceID {
	return NamespaceID{id: id}
}

func (n NamespaceID) String() string {
	return "namespace:" + n.id
}

// Workflow creates a workflow identity as a child of this namespace.
func (n NamespaceID) Workflow(workflowID string) WorkflowID {
	return WorkflowID{parent: n, id: workflowID}
}

// TaskQueue creates a task queue identity within this namespace.
func (n NamespaceID) TaskQueue(name string) TaskQueueID {
	return TaskQueueID{namespace: n, name: name}
}

// WorkflowID identifies a workflow within a namespace.
type WorkflowID struct {
	parent NamespaceID
	id     string
}

func (w WorkflowID) String() string {
	return w.parent.String() + "/workflow:" + w.id
}

// Execution creates a workflow execution identity as a child of this workflow.
func (w WorkflowID) Execution(runID string) ExecutionID {
	return ExecutionID{parent: w, id: runID}
}

// Task creates a workflow task identity as a child of this workflow.
func (w WorkflowID) Task(taskID string) WorkflowTaskID {
	return WorkflowTaskID{parent: w, id: taskID}
}

// Update creates a workflow update identity as a child of this workflow.
func (w WorkflowID) Update(updateID string) WorkflowUpdateID {
	return WorkflowUpdateID{parent: w, id: updateID}
}

// ExecutionID identifies a specific workflow execution (run).
type ExecutionID struct {
	parent WorkflowID
	id     string
}

func (e ExecutionID) String() string {
	return e.parent.String() + "/execution:" + e.id
}

// WorkflowTaskID identifies a workflow task.
type WorkflowTaskID struct {
	parent WorkflowID
	id     string
}

func (t WorkflowTaskID) String() string {
	return t.parent.String() + "/task:" + t.id
}

// WorkflowUpdateID identifies a workflow update.
type WorkflowUpdateID struct {
	parent WorkflowID
	id     string
}

func (u WorkflowUpdateID) String() string {
	return u.parent.String() + "/update:" + u.id
}

// TaskQueueID identifies a task queue within a namespace.
type TaskQueueID struct {
	namespace NamespaceID
	name      string
}

func (t TaskQueueID) String() string {
	return t.namespace.String() + "/taskqueue:" + t.name
}

// =============================================================================
// Parsing utilities
// =============================================================================

// ParseIdentity parses a canonical identity string back into its components.
// Returns a map of type -> id for each level in the hierarchy.
func ParseIdentity(s string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(s, "/")
	for _, part := range parts {
		if idx := strings.Index(part, ":"); idx > 0 {
			result[part[:idx]] = part[idx+1:]
		}
	}
	return result
}
