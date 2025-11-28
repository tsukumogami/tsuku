package recipe

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/tsuku-dev/tsuku/internal/registry"
)

// Loader handles loading and discovering recipes from the registry
type Loader struct {
	recipes  map[string]*Recipe
	registry *registry.Registry
}

// New creates a new recipe loader with the given registry
func New(reg *registry.Registry) *Loader {
	return &Loader{
		recipes:  make(map[string]*Recipe),
		registry: reg,
	}
}

// Get retrieves a recipe by name
// Priority: 1. In-memory cache, 2. Disk cache, 3. Remote registry
func (l *Loader) Get(name string) (*Recipe, error) {
	return l.GetWithContext(context.Background(), name)
}

// GetWithContext retrieves a recipe by name with context support
func (l *Loader) GetWithContext(ctx context.Context, name string) (*Recipe, error) {
	// Check in-memory cache first
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Fetch from registry (disk cache or remote)
	recipe, err := l.fetchFromRegistry(ctx, name)
	if err != nil {
		return nil, err
	}

	l.recipes[name] = recipe
	return recipe, nil
}

// fetchFromRegistry attempts to get a recipe from the registry (cache or remote)
func (l *Loader) fetchFromRegistry(ctx context.Context, name string) (*Recipe, error) {
	// Check disk cache first
	data, err := l.registry.GetCached(name)
	if err != nil {
		return nil, err
	}

	if data == nil {
		// Not cached, fetch from remote
		data, err = l.registry.FetchRecipe(ctx, name)
		if err != nil {
			return nil, err
		}

		// Cache the fetched recipe
		if cacheErr := l.registry.CacheRecipe(name, data); cacheErr != nil {
			// Log warning but don't fail
			fmt.Printf("Warning: failed to cache recipe %s: %v\n", name, cacheErr)
		}
	}

	// Parse the recipe
	return l.parseBytes(data)
}

// parseBytes parses a recipe from raw TOML bytes
func (l *Loader) parseBytes(data []byte) (*Recipe, error) {
	var recipe Recipe
	if err := toml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if err := validate(&recipe); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}

	return &recipe, nil
}

// List returns all cached recipe names
// Note: This only returns recipes that have been fetched and cached
func (l *Loader) List() []string {
	names := make([]string, 0, len(l.recipes))
	for name := range l.recipes {
		names = append(names, name)
	}
	return names
}

// Count returns the number of loaded recipes in memory
func (l *Loader) Count() int {
	return len(l.recipes)
}

// Registry returns the registry client
func (l *Loader) Registry() *registry.Registry {
	return l.registry
}

// ClearCache clears the in-memory recipe cache
// This forces recipes to be re-fetched from the registry on next access
func (l *Loader) ClearCache() {
	l.recipes = make(map[string]*Recipe)
}

// validate performs basic recipe validation
func validate(r *Recipe) error {
	// Check metadata
	if r.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	// Check steps
	if len(r.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	for i, step := range r.Steps {
		if step.Action == "" {
			return fmt.Errorf("step %d: action is required", i+1)
		}
	}

	// Check verify
	if r.Verify.Command == "" {
		return fmt.Errorf("verify.command is required")
	}

	return nil
}
