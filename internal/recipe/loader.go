package recipe

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Loader handles loading and discovering recipes
type Loader struct {
	recipes map[string]*Recipe
	fs      embed.FS
}

// NewLoader creates a new recipe loader with bundled recipes
func NewLoader(recipesFS embed.FS) (*Loader, error) {
	loader := &Loader{
		recipes: make(map[string]*Recipe),
		fs:      recipesFS,
	}

	// Load all bundled recipes
	if err := loader.loadBundled(); err != nil {
		return nil, fmt.Errorf("failed to load bundled recipes: %w", err)
	}

	return loader, nil
}

// loadBundled loads all recipes from the embedded filesystem
func (l *Loader) loadBundled() error {
	entries, err := l.fs.ReadDir("recipes")
	if err != nil {
		return fmt.Errorf("failed to read embedded recipes directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".toml")
		path := filepath.Join("recipes", entry.Name())

		recipe, err := l.parseEmbedded(path)
		if err != nil {
			return fmt.Errorf("failed to parse recipe %s: %w", name, err)
		}

		l.recipes[name] = recipe
	}

	return nil
}

// parseEmbedded parses a recipe from the embedded filesystem
func (l *Loader) parseEmbedded(path string) (*Recipe, error) {
	data, err := l.fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %w", err)
	}

	var recipe Recipe
	if err := toml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Validate recipe
	if err := validate(&recipe); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}

	return &recipe, nil
}

// Get retrieves a recipe by name
func (l *Loader) Get(name string) (*Recipe, error) {
	recipe, ok := l.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe not found: %s", name)
	}
	return recipe, nil
}

// List returns all available recipe names
func (l *Loader) List() []string {
	names := make([]string, 0, len(l.recipes))
	for name := range l.recipes {
		names = append(names, name)
	}
	return names
}

// Count returns the number of loaded recipes
func (l *Loader) Count() int {
	return len(l.recipes)
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
