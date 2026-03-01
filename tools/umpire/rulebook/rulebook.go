package rulebook

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Violation represents a detected invariant violation.
type Violation struct {
	// Model is the name of the model that detected the violation
	Model string

	// Message is a human-readable description of the violation
	Message string

	// Tags contains additional context about the violation
	Tags map[string]string
}

// Model is the interface that all verification models must implement.
type Model interface {
	// Name returns a stable, lowercase identifier for this model.
	Name() string

	// Init is called once before any other method. It should capture
	// dependencies and initialize any internal state.
	// The Deps parameter is provided by the caller (typically watchdog package).
	Init(ctx context.Context, deps interface{}) error

	// Check runs verification against model state and/or the entity registry.
	// Returns a list of detected violations.
	Check(ctx context.Context) []Violation

	// Close releases any resources held by the model.
	Close(ctx context.Context) error
}

// Factory constructs a new Model instance.
type Factory func() Model

// Rulebook manages model registration and initialization.
type Rulebook struct {
	mu sync.RWMutex

	// registry holds model factories by name
	registry map[string]Factory

	// models holds initialized model instances
	models []Model
}

// NewRulebook creates a new rulebook for managing verification models.
// Models should be registered explicitly by calling Register() to avoid import cycles.
func NewRulebook() *Rulebook {
	return &Rulebook{
		registry: make(map[string]Factory),
	}
}

// Register makes a model factory available under the provided name.
// The name is lowercased before registration.
func (r *Rulebook) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.ToLower(strings.TrimSpace(name))
	r.registry[name] = factory
}

// AllModels returns the names of all registered models.
func (r *Rulebook) AllModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var names []string
	for name := range r.registry {
		names = append(names, name)
	}
	return names
}

// InitModels constructs and initializes the set of models specified by names.
// If names is empty, all registered models are enabled.
// An error is returned if a requested model is not registered.
// The deps parameter should implement the model.Deps interface (GetLogger, GetRegistry).
// The initialized models are stored in the rulebook.
func (r *Rulebook) InitModels(ctx context.Context, names []string, deps interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get all model names if none specified (without calling AllModels to avoid deadlock)
	if len(names) == 0 {
		for name := range r.registry {
			names = append(names, name)
		}
	}

	var out []Model
	seen := map[string]struct{}{}

	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		n = strings.ToLower(strings.TrimSpace(n))
		if _, dup := seen[n]; dup {
			continue
		}
		f, ok := r.registry[n]
		if !ok {
			return fmt.Errorf("unknown model: %q", n)
		}
		m := f()
		if err := m.Init(ctx, deps); err != nil {
			return fmt.Errorf("init model %q failed: %w", n, err)
		}
		out = append(out, m)
		seen[n] = struct{}{}
	}
	r.models = out
	return nil
}

// Check runs verification on all initialized models and returns all violations.
func (r *Rulebook) Check(ctx context.Context) []Violation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var violations []Violation
	for _, m := range r.models {
		violations = append(violations, m.Check(ctx)...)
	}
	return violations
}

// Close closes all initialized models.
func (r *Rulebook) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for _, m := range r.models {
		if err := m.Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Models returns all initialized models.
func (r *Rulebook) Models() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models
}
