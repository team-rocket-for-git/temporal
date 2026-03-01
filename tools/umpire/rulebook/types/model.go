package types

import (
	"go.temporal.io/server/common/log"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
)

// EntityRegistry is the interface for querying entities.
// This is defined here to avoid import cycles with roster.
type EntityRegistry interface {
	// QueryEntities returns all entities of the given type.
	// The entityRef parameter is used only to get the entity type via its Type() method.
	QueryEntities(entityRef interface{ Type() lineuptypes.EntityType }) []interface{}
}

// Deps bundles dependencies provided to models at initialization.
type Deps struct {
	// Registry provides access to the entity registry for querying entities.
	Registry EntityRegistry
	// Logger to use for logs and soft assertions.
	Logger log.Logger
}

// GetLogger returns the logger for interface-based dependency injection.
func (d Deps) GetLogger() log.Logger {
	return d.Logger
}

// GetRegistry returns the registry for interface-based dependency injection.
func (d Deps) GetRegistry() EntityRegistry {
	return d.Registry
}
