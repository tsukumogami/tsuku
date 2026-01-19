package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/registry"
)

// LoaderOptions configures recipe loading behavior.
type LoaderOptions struct {
	// RequireEmbedded restricts this specific load to embedded FS only.
	// When true, the loader skips local recipes and registry lookups,
	// returning an error if the recipe is not found in embedded recipes.
	// Used for validating action dependencies with --require-embedded flag.
	RequireEmbedded bool
}

// Loader handles loading and discovering recipes from the registry
type Loader struct {
	recipes          map[string]*Recipe
	registry         *registry.Registry
	embedded         *EmbeddedRegistry
	recipesDir       string           // Local recipes directory (~/.tsuku/recipes)
	constraintLookup ConstraintLookup // Optional lookup for step analysis (nil skips analysis)
}

// New creates a new recipe loader with the given registry
func New(reg *registry.Registry) *Loader {
	embedded, _ := NewEmbeddedRegistry() // Ignore error - embedded recipes are optional
	return &Loader{
		recipes:  make(map[string]*Recipe),
		registry: reg,
		embedded: embedded,
	}
}

// NewWithLocalRecipes creates a new recipe loader with local recipe support
func NewWithLocalRecipes(reg *registry.Registry, recipesDir string) *Loader {
	embedded, _ := NewEmbeddedRegistry() // Ignore error - embedded recipes are optional
	return &Loader{
		recipes:    make(map[string]*Recipe),
		registry:   reg,
		embedded:   embedded,
		recipesDir: recipesDir,
	}
}

// NewWithoutEmbedded creates a new recipe loader without embedded recipes (for testing)
func NewWithoutEmbedded(reg *registry.Registry, recipesDir string) *Loader {
	return &Loader{
		recipes:    make(map[string]*Recipe),
		registry:   reg,
		embedded:   nil, // No embedded recipes
		recipesDir: recipesDir,
	}
}

// SetRecipesDir sets the local recipes directory
func (l *Loader) SetRecipesDir(dir string) {
	l.recipesDir = dir
}

// SetConstraintLookup sets the constraint lookup function for step analysis.
// When set, loaded recipes will have their steps analyzed for platform constraints.
// When nil (default), step analysis is skipped (backward compatible).
func (l *Loader) SetConstraintLookup(lookup ConstraintLookup) {
	l.constraintLookup = lookup
}

// Get retrieves a recipe by name
// Priority: 1. In-memory cache, 2. Local recipes, 3. Embedded recipes, 4. Registry (disk cache or remote)
// When opts.RequireEmbedded is true, only checks embedded recipes (skips local and registry).
func (l *Loader) Get(name string, opts LoaderOptions) (*Recipe, error) {
	return l.GetWithContext(context.Background(), name, opts)
}

// GetWithContext retrieves a recipe by name with context support
// Priority: 1. In-memory cache, 2. Local recipes, 3. Embedded recipes, 4. Registry (disk cache or remote)
// When opts.RequireEmbedded is true, only checks embedded recipes (skips local and registry).
func (l *Loader) GetWithContext(ctx context.Context, name string, opts LoaderOptions) (*Recipe, error) {
	// When RequireEmbedded is set, only check embedded recipes
	if opts.RequireEmbedded {
		return l.getEmbeddedOnly(name)
	}

	// Check in-memory cache first
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Check local recipes directory if configured
	if l.recipesDir != "" {
		localRecipe, localErr := l.loadLocalRecipe(name)
		if localErr == nil && localRecipe != nil {
			// Check if this shadows an embedded or registry recipe and warn
			l.warnIfShadows(ctx, name)
			l.recipes[name] = localRecipe
			return localRecipe, nil
		}
		// If file doesn't exist, continue to embedded/registry
		// If file exists but has parse error, return the error
		if localErr != nil && !os.IsNotExist(localErr) {
			return nil, localErr
		}
	}

	// Check embedded recipes
	if l.embedded != nil {
		if data, ok := l.embedded.Get(name); ok {
			recipe, err := l.parseBytes(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse embedded recipe %s: %w", name, err)
			}
			l.recipes[name] = recipe
			return recipe, nil
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

// getEmbeddedOnly loads a recipe from embedded FS only, returning a clear error if not found.
// This is used when opts.RequireEmbedded is true.
func (l *Loader) getEmbeddedOnly(name string) (*Recipe, error) {
	// Check in-memory cache first (recipe may have been loaded previously)
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Check embedded recipes only
	if l.embedded != nil {
		if data, ok := l.embedded.Get(name); ok {
			recipe, err := l.parseBytes(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse embedded recipe %s: %w", name, err)
			}
			l.recipes[name] = recipe
			return recipe, nil
		}
	}

	// Recipe not found in embedded FS - return actionable error
	return nil, fmt.Errorf(
		"recipe %q not found in embedded registry\n\n"+
			"This error occurs because RequireEmbedded is set, which restricts recipe\n"+
			"loading to the embedded registry only. The recipe must be available without\n"+
			"network access.\n\n"+
			"To fix: ensure the recipe exists in internal/recipe/recipes/",
		name,
	)
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

// warnIfShadows checks if a local recipe shadows an embedded or registry recipe and logs a warning
func (l *Loader) warnIfShadows(ctx context.Context, name string) {
	// Check if recipe exists in embedded recipes
	if l.embedded != nil && l.embedded.Has(name) {
		fmt.Printf("Warning: local recipe '%s' shadows embedded recipe\n", name)
		return
	}
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

	// Compute step analysis if constraint lookup is configured
	if l.constraintLookup != nil {
		if err := computeStepAnalysis(&recipe, l.constraintLookup); err != nil {
			return nil, fmt.Errorf("step analysis failed: %w", err)
		}
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

// CacheRecipe adds a recipe to the in-memory cache
// This is useful for testing or loading recipes from non-standard sources
func (l *Loader) CacheRecipe(name string, r *Recipe) {
	l.recipes[name] = r
}

// RecipeSource indicates where a recipe comes from
type RecipeSource string

const (
	// SourceLocal indicates a recipe from the local recipes directory ($TSUKU_HOME/recipes)
	SourceLocal RecipeSource = "local"
	// SourceEmbedded indicates a recipe embedded in the binary
	SourceEmbedded RecipeSource = "embedded"
	// SourceRegistry indicates a recipe from the cached registry ($TSUKU_HOME/registry)
	SourceRegistry RecipeSource = "registry"
)

// RecipeInfo contains a recipe with its source information
type RecipeInfo struct {
	Name        string
	Description string
	Source      RecipeSource
}

// ListAllWithSource returns all available recipes from local, embedded, and registry sources
// Priority order: local > embedded > registry (same as resolution chain)
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

	// Then, list embedded recipes
	if l.embedded != nil {
		embeddedRecipes, err := l.embedded.ListWithInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to list embedded recipes: %w", err)
		}
		for _, info := range embeddedRecipes {
			if !seen[info.Name] {
				seen[info.Name] = true
				result = append(result, info)
			}
		}
	}

	// Finally, list registry recipes (cached only)
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

// ListEmbedded returns only recipes embedded in the binary
func (l *Loader) ListEmbedded() ([]RecipeInfo, error) {
	if l.embedded == nil {
		return nil, nil
	}
	return l.embedded.ListWithInfo()
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

// ParseFile parses a recipe from a file path.
// This is a convenience function for loading recipes outside the registry/loader
// system (e.g., for evaluating local recipe files).
// An optional ConstraintLookup can be provided for step analysis.
func ParseFile(path string, lookup ...ConstraintLookup) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var r Recipe
	if err := toml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if err := validate(&r); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}

	// Compute step analysis if lookup is provided
	if len(lookup) > 0 && lookup[0] != nil {
		if err := computeStepAnalysis(&r, lookup[0]); err != nil {
			return nil, fmt.Errorf("step analysis failed: %w", err)
		}
	}

	return &r, nil
}

// RecipesDir returns the local recipes directory
func (l *Loader) RecipesDir() string {
	return l.recipesDir
}

// computeStepAnalysis computes analysis for all steps in a recipe.
// It populates each step's analysis field using the provided lookup.
// Returns the first error encountered (step index included in message).
func computeStepAnalysis(r *Recipe, lookup ConstraintLookup) error {
	for i := range r.Steps {
		step := &r.Steps[i]
		analysis, err := ComputeAnalysis(step.Action, step.When, step.Params, lookup)
		if err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.Action, err)
		}
		step.SetAnalysis(analysis)
	}
	return nil
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

	// Check verify (libraries don't require verification)
	if r.Metadata.Type != RecipeTypeLibrary && r.Verify.Command == "" {
		return fmt.Errorf("verify.command is required")
	}

	return nil
}
