package simulation

import (
	"fmt"
	"sync"
)

// Registry manages named ForecastEngine instances.
type Registry struct {
	mu      sync.RWMutex
	engines map[string]ForecastEngine
}

// NewRegistry creates an empty engine registry.
func NewRegistry() *Registry {
	return &Registry{engines: make(map[string]ForecastEngine)}
}

// Register adds an engine to the registry. Panics on duplicate names.
// This is intentional: registration happens only during init() at startup,
// so a duplicate is a programmer error that should fail fast (Go convention).
func (r *Registry) Register(e ForecastEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := e.Name()
	if _, exists := r.engines[name]; exists {
		panic(fmt.Sprintf("duplicate engine registration: %q", name))
	}
	r.engines[name] = e
}

// Get returns the engine with the given name, or an error if not found.
func (r *Registry) Get(name string) (ForecastEngine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.engines[name]
	if !ok {
		return nil, fmt.Errorf("unknown engine: %q", name)
	}
	return e, nil
}

// Enabled returns all engines whose weight > 0 in the given weight map.
func (r *Registry) Enabled(weights map[string]int) []ForecastEngine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ForecastEngine
	for name, e := range r.engines {
		if w, ok := weights[name]; ok && w > 0 {
			out = append(out, e)
		}
	}
	return out
}
