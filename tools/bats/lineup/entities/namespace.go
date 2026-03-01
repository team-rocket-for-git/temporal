package entities

import (
	"fmt"

	"go.temporal.io/server/tools/bats/lineup"
	lineuptypes "go.temporal.io/server/tools/bats/lineup/types"
	"go.temporal.io/server/tools/umpire/scorebook/moves"
	scorebooktypes "go.temporal.io/server/tools/umpire/scorebook/types"
)

var _ lineup.Entity = (*Namespace)(nil)


// Namespace represents a Temporal namespace entity.
// Locking is handled by the registry - individual methods should not lock.
type Namespace struct {
	NamespaceID   string
	NamespaceName string
	IsGlobalNamespace bool
}

// NewNamespace creates a new Namespace entity.
func NewNamespace() *Namespace {
	return &Namespace{}
}

func (ns *Namespace) Type() lineuptypes.EntityType {
	return lineuptypes.NamespaceType
}

func (ns *Namespace) OnEvent(_ *lineuptypes.Identity, iter scorebooktypes.MoveIterator) error {
	iter(func(ev scorebooktypes.Move) bool {
		switch e := ev.(type) {
		case *moves.StartWorkflow:
			if ns.NamespaceID == "" && e.Request != nil {
				ns.NamespaceID = e.Request.NamespaceId
			}
			if ns.NamespaceName == "" && e.Request != nil && e.Request.StartRequest != nil {
				ns.NamespaceName = e.Request.StartRequest.Namespace
			}
		}
		return true
	})
	return nil
}

func (ns *Namespace) String() string {
	return fmt.Sprintf("Namespace{id=%s, name=%s}",
		ns.NamespaceID, ns.NamespaceName)
}
