package scorebook

import (
	"sync"

	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/types"
)

// Scorebook keeps track of all moves for querying during tests.
type Scorebook struct {
	mu    sync.RWMutex
	moves []types.Move
}

// NewScorebook creates a new scorebook tracker.
func NewScorebook() *Scorebook {
	return &Scorebook{
		moves: make([]types.Move, 0),
	}
}

// Add records a move in the scorebook.
func (h *Scorebook) Add(move types.Move) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.moves = append(h.moves, move)
}

// AddAll records multiple moves in the scorebook.
func (h *Scorebook) AddAll(moves []types.Move) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.moves = append(h.moves, moves...)
}

// QueryByType returns moves matching the given entity ID and move type.
// This will match moves where:
// - The move's EntityID matches the query ID, OR
// - The move's ParentID matches the query ID (for child entities)
//
// Example usage:
//   QueryByType(lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID), &moves.StoreWorkflowTask{})
func (h *Scorebook) QueryByType(entityID lineuptypes.EntityID, moveType types.Move) []types.Move {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []types.Move
	for _, move := range h.moves {
		// Check move type
		if move.MoveType() != moveType.MoveType() {
			continue
		}

		// Check entity ID
		identity := move.TargetEntity()
		if identity == nil {
			continue
		}

		// Check if the move's entity ID matches or contains the query entity ID
		if matchesEntityID(identity.EntityID, entityID) {
			result = append(result, move)
			continue
		}

		// Also check if the move's parent ID matches (for child entities like updates)
		if identity.ParentID != nil && matchesEntityID(*identity.ParentID, entityID) {
			result = append(result, move)
			continue
		}
	}
	return result
}

// QueryByID returns moves matching the given entity ID.
// This will match moves where:
// - The move's EntityID matches the query ID, OR
// - The move's ParentID matches the query ID (for child entities)
//
// Example usage:
//   QueryByID(lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID))
func (h *Scorebook) QueryByID(entityID lineuptypes.EntityID) []types.Move {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []types.Move
	for _, move := range h.moves {
		// Check entity ID
		identity := move.TargetEntity()
		if identity == nil {
			continue
		}

		// Check if the move's entity ID matches or contains the query entity ID
		if matchesEntityID(identity.EntityID, entityID) {
			result = append(result, move)
			continue
		}

		// Also check if the move's parent ID matches (for child entities like updates)
		if identity.ParentID != nil && matchesEntityID(*identity.ParentID, entityID) {
			result = append(result, move)
			continue
		}
	}
	return result
}

// All returns all moves in the scorebook.
func (h *Scorebook) All() []types.Move {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]types.Move, len(h.moves))
	copy(result, h.moves)
	return result
}

// Clear removes all moves from scorebook.
func (h *Scorebook) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.moves = make([]types.Move, 0)
}

// QueryByWorkflowID is a convenience function to query moves by workflow ID.
// It creates the appropriate entity ID and queries by ID.
func (h *Scorebook) QueryByWorkflowID(workflowID string) []types.Move {
	entityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)
	return h.QueryByID(entityID)
}

// QueryByWorkflowIDAndType is a convenience function to query moves by workflow ID and move type.
// It creates the appropriate entity ID and queries by type.
func (h *Scorebook) QueryByWorkflowIDAndType(workflowID string, moveType string) []types.Move {
	entityID := lineuptypes.NewEntityIDFromType(lineuptypes.WorkflowType, workflowID)

	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []types.Move
	for _, move := range h.moves {
		// Check move type by name
		if move.MoveType() != moveType {
			continue
		}

		// Check entity ID
		identity := move.TargetEntity()
		if identity == nil {
			continue
		}

		// Check if the move's entity ID matches or contains the query entity ID
		if matchesEntityID(identity.EntityID, entityID) {
			result = append(result, move)
			continue
		}

		// Also check if the move's parent ID matches (for child entities like updates)
		if identity.ParentID != nil && matchesEntityID(*identity.ParentID, entityID) {
			result = append(result, move)
			continue
		}
	}
	return result
}

// matchesEntityID checks if the move's entity ID matches or contains the query entity ID.
// For composite IDs like "taskQueue:workflowID:runID", this checks if the query ID is a substring.
func matchesEntityID(moveEntityID, queryEntityID lineuptypes.EntityID) bool {
	// Exact match
	if moveEntityID.ID == queryEntityID.ID {
		return true
	}

	// For composite IDs, check if query ID is a substring
	// This handles cases like WorkflowTask IDs which contain workflow IDs
	if len(moveEntityID.ID) > len(queryEntityID.ID) {
		// Simple substring match
		for i := 0; i <= len(moveEntityID.ID)-len(queryEntityID.ID); i++ {
			if moveEntityID.ID[i:i+len(queryEntityID.ID)] == queryEntityID.ID {
				return true
			}
		}
	}

	return false
}
