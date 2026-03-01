package types

import (
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// MoveIterator is a function that yields moves one at a time.
// The yield function returns false to stop iteration early.
type MoveIterator func(yield func(Move) bool)

// Move is the interface that all moves must implement.
// In baseball terminology, a "move" is an action in the game.
type Move interface {
	// MoveType returns the move type name (e.g., "AddWorkflowTask")
	MoveType() string

	// TargetEntity returns the identity of the entity this move targets.
	// Returns nil if the move doesn't target a specific entity.
	TargetEntity() *lineuptypes.Identity
}

// EventIterator is deprecated, use MoveIterator
type EventIterator = MoveIterator

// Event is deprecated, use Move
type Event = Move
