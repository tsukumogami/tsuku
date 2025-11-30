package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/tsuku-dev/tsuku/internal/registry"
)

// Loader handles loading and discovering recipes from the registry
type Loader struct {
	recipes    map[string]*Recipe
	registry   *registry.Registry
	recipesDir string // Local recipes directory (~/.tsuku/recipes)
}

// New creates a new recipe loader with the given registry
func New(reg *registry.Registry) *Loader {
	return &Loader{
		recipes:  make(map[string]*Recipe),
		registry: reg,
	}
}

// NewWithLocalRecipes creates a new recipe loader with local recipe support
func NewWithLocalRecipes(reg *registry.Registry, recipesDir string) *Loader {
	return &Loader{
		recipes:    make(map[string]*Recipe),
		registry:   reg,
		recipesDir: recipesDir,
	}
}

// SetRecipesDir sets the local recipes directory
func (l *Loader) SetRecipesDir(dir string) {
	l.recipesDir = dir
}

// Get retrieves a recipe by name
// Priority: 1. In-memory cache, 2. Local recipes, 3. Registry (disk cache or remote)
func (l *Loader) Get(name string) (*Recipe, error) {
	return l.GetWithContext(context.Background(), name)
}

// GetWithContext retrieves a recipe by name with context support
// Priority: 1. In-memory cache, 2. Local recipes, 3. Registry (disk cache or remote)
func (l *Loader) GetWithContext(ctx context.Context, name string) (*Recipe, error) {
	// Check in-memory cache first
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Check local recipes directory if configured
	if l.recipesDir != "" {
		localRecipe, localErr := l.loadLocalRecipe(name)
		if localErr == nil && localRecipe != nil {
			// Check if this shadows a registry recipe and warn
			l.warnIfShadowsRegistry(ctx, name)
			l.recipes[name] = localRecipe
			return localRecipe, nil
		}
		// If file doesn't exist, continue to registry
		// If file exists but has parse error, return the error
		if localErr != nil && !os.IsNotExist(localErr) {
			return nil, localErr
		}
	}

	// Fetch from registry (disk cache or remote)
	recipe, err := l.fetchFromRegistry(ctx, name)
	if err != nil {
		return nil, err
	}

	l.recipes[name] = recipe
	return recipe, nil
}

// loadLocalRecipe attempts to load a recipe from the local recipes directory
func (l *Loader) loadLocalRecipe(name string) (*Recipe, error) {
	path := filepath.Join(l.recipesDir, name+".toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return l.parseBytes(data)
}

// warnIfShadowsRegistry checks if a local recipe shadows a registry recipe and logs a warning
func (l *Loader) warnIfShadowsRegistry(ctx context.Context, name string) {
	// Check if recipe exists in registry cache
	data, _ := l.registry.GetCached(name)
	if data != nil {
		fmt.Printf("Warning: local recipe '%s' shadows registry recipe\n", name)
		return
	}
	// Optionally check remote (but don't block on it)
	// For now, we only warn if already cached to avoid network delay
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

// RecipeSource indicates where a recipe comes from
type RecipeSource string

const (
	// SourceLocal indicates a recipe from the local recipes directory ($TSUKU_HOME/recipes)
	SourceLocal RecipeSource = "local"
	// SourceRegistry indicates a recipe from the cached registry ($TSUKU_HOME/registry)
	SourceRegistry RecipeSource = "registry"
)

// RecipeInfo contains a recipe with its source information
type RecipeInfo struct {
	Name        string
	Description string
	Source      RecipeSource
}

// ListAllWithSource returns all available recipes from both local and registry sources
// Local recipes are listed first, and recipes that appear in both are shown as local
func (l *Loader) ListAllWithSource() ([]RecipeInfo, error) {
	seen := make(map[string]bool)
	var result []RecipeInfo

	// First, list local recipes
	localRecipes, err := l.listLocalRecipes()
	if err != nil {
		return nil, fmt.Errorf("failed to list local recipes: %w", err)
	}
	for _, info := range localRecipes {
		seen[info.Name] = true
		result = append(result, info)
	}

	// Then, list registry recipes (cached only)
	registryRecipes, err := l.listRegistryRecipes()
	if err != nil {
		return nil, fmt.Errorf("failed to list registry recipes: %w", err)
	}
	for _, info := range registryRecipes {
		if !seen[info.Name] {
			result = append(result, info)
		}
	}

	return result, nil
}

// ListLocal returns only recipes from the local recipes directory
func (l *Loader) ListLocal() ([]RecipeInfo, error) {
	return l.listLocalRecipes()
}

// listLocalRecipes scans the local recipes directory and returns recipe info
func (l *Loader) listLocalRecipes() ([]RecipeInfo, error) {
	if l.recipesDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(l.recipesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []RecipeInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isTomlFile(name) {
			continue
		}

		recipeName := trimTomlExtension(name)
		description := ""

		// Try to load the recipe to get description
		recipe, err := l.loadLocalRecipe(recipeName)
		if err == nil && recipe != nil {
			description = recipe.Metadata.Description
		}

		result = append(result, RecipeInfo{
			Name:        recipeName,
			Description: description,
			Source:      SourceLocal,
		})
	}

	return result, nil
}

// listRegistryRecipes scans the registry cache and returns recipe info
func (l *Loader) listRegistryRecipes() ([]RecipeInfo, error) {
	names, err := l.registry.ListCached()
	if err != nil {
		return nil, err
	}

	var result []RecipeInfo
	for _, name := range names {
		description := ""

		// Try to load the recipe to get description
		data, err := l.registry.GetCached(name)
		if err == nil && data != nil {
			if recipe, err := l.parseBytes(data); err == nil {
				description = recipe.Metadata.Description
			}
		}

		result = append(result, RecipeInfo{
			Name:        name,
			Description: description,
			Source:      SourceRegistry,
		})
	}

	return result, nil
}

// isTomlFile checks if a filename has a .toml extension
func isTomlFile(name string) bool {
	return len(name) > 5 && name[len(name)-5:] == ".toml"
}

// trimTomlExtension removes the .toml extension from a filename
func trimTomlExtension(name string) string {
	if isTomlFile(name) {
		return name[:len(name)-5]
	}
	return name
}

// RecipesDir returns the local recipes directory
func (l *Loader) RecipesDir() string {
	return l.recipesDir
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
