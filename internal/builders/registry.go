package builders

import "sync"

// Registry holds all available builders.
type Registry struct {
	mu       sync.RWMutex
	builders map[string]SessionBuilder
}

// NewRegistry creates a new builder registry.
func NewRegistry() *Registry {
	return &Registry{
		builders: make(map[string]SessionBuilder),
	}
}

// Register adds a builder to the registry.
// If a builder with the same name already exists, it will be replaced.
func (r *Registry) Register(b SessionBuilder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[b.Name()] = b
}

// Get retrieves a builder by name.
// Returns nil and false if the builder is not found.
func (r *Registry) Get(name string) (SessionBuilder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.builders[name]
	return b, ok
}

// List returns the names of all registered builders.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.builders))
	for name := range r.builders {
		names = append(names, name)
	}
	return names
}
