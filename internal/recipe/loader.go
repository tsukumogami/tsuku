package recipe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// LoaderOptions configures recipe loading behavior.
type LoaderOptions struct {
	// RequireEmbedded restricts this specific load to embedded FS only.
	// When true, the loader skips local recipes and registry lookups,
	// returning an error if the recipe is not found in embedded recipes.
	// Used for validating action dependencies with --require-embedded flag.
	RequireEmbedded bool
}

// Loader handles loading and discovering recipes from multiple providers.
type Loader struct {
	providers        []RecipeProvider
	recipes          map[string]*Recipe        // In-memory parsed cache
	recipeSources    map[string]RecipeSource   // Tracks which source each cached recipe came from
	constraintLookup ConstraintLookup          // Optional lookup for step analysis (nil skips analysis)
	satisfiesIndex   map[string]satisfiesEntry // package_name -> satisfiesEntry (lazy-built)
	satisfiesOnce    sync.Once                 // ensures buildSatisfiesIndex runs once
}

// NewLoader creates a new recipe loader with the given providers.
// Providers are consulted in order; earlier providers shadow later ones.
func NewLoader(providers ...RecipeProvider) *Loader {
	return &Loader{
		providers:     providers,
		recipes:       make(map[string]*Recipe),
		recipeSources: make(map[string]RecipeSource),
	}
}

// SetConstraintLookup sets the constraint lookup function for step analysis.
// When set, loaded recipes will have their steps analyzed for platform constraints.
// When nil (default), step analysis is skipped (backward compatible).
func (l *Loader) SetConstraintLookup(lookup ConstraintLookup) {
	l.constraintLookup = lookup
}

// Get retrieves a recipe by name.
// Priority follows the provider order, with in-memory cache checked first.
// When opts.RequireEmbedded is true, only checks embedded providers.
func (l *Loader) Get(name string, opts LoaderOptions) (*Recipe, error) {
	return l.GetWithContext(context.Background(), name, opts)
}

// GetWithContext retrieves a recipe by name with context support.
// Priority follows the provider order, with in-memory cache checked first.
// When opts.RequireEmbedded is true, only checks embedded providers.
func (l *Loader) GetWithContext(ctx context.Context, name string, opts LoaderOptions) (*Recipe, error) {
	if opts.RequireEmbedded {
		return l.getEmbeddedOnly(ctx, name)
	}

	// Check in-memory cache first
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Resolve from the full provider chain with satisfies fallback
	data, source, err := l.resolveFromChain(ctx, l.providers, name, true)
	if err != nil {
		return nil, err
	}

	recipe, err := l.parseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recipe %s from %s: %w", name, source, err)
	}

	// Warn if a higher-priority provider shadows lower-priority ones
	if source == SourceLocal {
		l.warnIfShadows(ctx, name)
	}

	l.recipes[name] = recipe
	l.recipeSources[name] = source
	return recipe, nil
}

// GetWithSource retrieves a recipe by name and returns the source that provided it.
// This is used by the install flow to record which provider resolved the recipe.
func (l *Loader) GetWithSource(name string, opts LoaderOptions) (*Recipe, RecipeSource, error) {
	r, err := l.Get(name, opts)
	if err != nil {
		return nil, "", err
	}
	source, ok := l.recipeSources[name]
	if !ok {
		// Fallback for recipes loaded before source tracking was added to the cache
		// (e.g., via getEmbeddedOnly which doesn't track source).
		source = SourceRegistry
	}
	return r, source, nil
}

// GetFromSource retrieves raw recipe bytes from a specific source, bypassing the
// normal provider priority chain and the in-memory cache. This enables source-directed
// operations (update, outdated, verify) to fetch the recipe from the same source
// that originally provided it.
//
// The source parameter uses user-facing source strings as stored in ToolState.Source:
//   - "central": delegates to the central registry provider (SourceRegistry)
//   - "local": delegates to the local provider (SourceLocal)
//   - "owner/repo": delegates to a distributed provider for that repository
//
// Returns an error if no provider matches the given source.
func (l *Loader) GetFromSource(ctx context.Context, name string, source string) ([]byte, error) {
	switch source {
	case SourceCentral:
		// Try registry provider first, then embedded
		for _, p := range l.providers {
			if p.Source() == SourceRegistry {
				data, err := p.Get(ctx, name)
				if err == nil && data != nil {
					return data, nil
				}
			}
		}
		for _, p := range l.providers {
			if p.Source() == SourceEmbedded {
				data, err := p.Get(ctx, name)
				if err == nil && data != nil {
					return data, nil
				}
			}
		}
		return nil, fmt.Errorf("recipe %q not found in central registry", name)

	case string(SourceLocal):
		for _, p := range l.providers {
			if p.Source() == SourceLocal {
				data, err := p.Get(ctx, name)
				if err != nil {
					return nil, err
				}
				if data != nil {
					return data, nil
				}
			}
		}
		return nil, fmt.Errorf("recipe %q not found in local recipes", name)

	default:
		// Check for "owner/repo" pattern (distributed provider)
		if strings.Contains(source, "/") {
			for _, p := range l.providers {
				if string(p.Source()) == source {
					data, err := p.Get(ctx, name)
					if err != nil {
						return nil, err
					}
					if data != nil {
						return data, nil
					}
					return nil, fmt.Errorf("recipe %q not found in %s", name, source)
				}
			}
			return nil, fmt.Errorf("no provider registered for source %q", source)
		}

		return nil, fmt.Errorf("unknown recipe source %q", source)
	}
}

// getEmbeddedOnly loads a recipe from embedded providers only.
func (l *Loader) getEmbeddedOnly(ctx context.Context, name string) (*Recipe, error) {
	// Check in-memory cache first (recipe may have been loaded previously)
	if recipe, ok := l.recipes[name]; ok {
		return recipe, nil
	}

	// Filter providers to embedded only
	embeddedProviders := l.filterProviders(SourceEmbedded)

	// Try direct resolution without satisfies first
	data, _, err := l.resolveFromChain(ctx, embeddedProviders, name, false)
	if err == nil {
		recipe, parseErr := l.parseBytes(data)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse embedded recipe %s: %w", name, parseErr)
		}
		l.recipes[name] = recipe
		return recipe, nil
	}

	// Satisfies fallback (restricted to embedded-only index entries)
	if canonicalName, ok := l.lookupSatisfiesFiltered(name, SourceEmbedded); ok {
		// Load directly without satisfies to prevent recursion
		data, _, directErr := l.resolveFromChain(ctx, embeddedProviders, canonicalName, false)
		if directErr != nil {
			return nil, fmt.Errorf(
				"recipe %q not found in embedded registry (resolved from satisfies index)", canonicalName,
			)
		}
		recipe, parseErr := l.parseBytes(data)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse embedded recipe %s: %w", canonicalName, parseErr)
		}
		l.recipes[name] = recipe
		return recipe, nil
	}

	return nil, fmt.Errorf(
		"recipe %q not found in embedded registry\n\n"+
			"This error occurs because RequireEmbedded is set, which restricts recipe\n"+
			"loading to the embedded registry only. The recipe must be available without\n"+
			"network access.\n\n"+
			"To fix: ensure the recipe exists in internal/recipe/recipes/",
		name,
	)
}

// resolveFromChain tries each provider in order, returning the first successful result.
// If trySatisfies is true and no provider has the recipe, it falls back to the
// satisfies index and retries with trySatisfies=false (recursion guard).
func (l *Loader) resolveFromChain(ctx context.Context, providers []RecipeProvider, name string, trySatisfies bool) ([]byte, RecipeSource, error) {
	var lastErr error

	for _, p := range providers {
		data, err := p.Get(ctx, name)
		if err != nil {
			// If file doesn't exist, continue to next provider
			if os.IsNotExist(err) {
				lastErr = err
				continue
			}
			// For registry not-found errors, continue to next provider
			if isNotFoundError(err) {
				lastErr = err
				continue
			}
			// For other errors (parse errors, network errors), return immediately
			return nil, "", err
		}
		if data != nil {
			return data, p.Source(), nil
		}
		// data is nil without error -- treat as not found
		lastErr = fmt.Errorf("recipe %q not found in %s", name, p.Source())
	}

	// Satisfies fallback
	if trySatisfies {
		if canonicalName, ok := l.lookupSatisfies(name); ok {
			return l.resolveFromChain(ctx, providers, canonicalName, false)
		}
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("recipe %q not found", name)
}

// filterProviders returns providers matching the given source.
func (l *Loader) filterProviders(source RecipeSource) []RecipeProvider {
	var filtered []RecipeProvider
	for _, p := range l.providers {
		if p.Source() == source {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// warnIfShadows checks if a local recipe shadows recipes from lower-priority providers.
func (l *Loader) warnIfShadows(ctx context.Context, name string) {
	// Check all non-local providers for the same recipe name
	for _, p := range l.providers {
		if p.Source() == SourceLocal {
			continue
		}
		if p.Source() == SourceEmbedded {
			if ep, ok := p.(*EmbeddedProvider); ok && ep.Has(name) {
				fmt.Printf("Warning: local recipe '%s' shadows embedded recipe\n", name)
				return
			}
		}
		if p.Source() == SourceRegistry {
			// Check registry cache without network
			if rp, ok := p.(*CentralRegistryProvider); ok {
				data, _ := rp.Registry().GetCached(name)
				if data != nil {
					fmt.Printf("Warning: local recipe '%s' shadows registry recipe\n", name)
					return
				}
			}
		}
	}
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

// List returns all cached recipe names.
// Note: This only returns recipes that have been fetched and cached in memory.
func (l *Loader) List() []string {
	names := make([]string, 0, len(l.recipes))
	for name := range l.recipes {
		names = append(names, name)
	}
	return names
}

// Count returns the number of loaded recipes in memory.
func (l *Loader) Count() int {
	return len(l.recipes)
}

// ProviderBySource returns the first provider matching the given source, or nil.
// This is the documented escape hatch for commands like update-registry that
// need type-assertion access to provider internals.
func (l *Loader) ProviderBySource(source RecipeSource) RecipeProvider {
	for _, p := range l.providers {
		if p.Source() == source {
			return p
		}
	}
	return nil
}

// ClearCache clears the in-memory recipe cache and satisfies index.
// This forces recipes to be re-fetched from providers on next access,
// and the satisfies index to be rebuilt on next fallback lookup.
func (l *Loader) ClearCache() {
	l.recipes = make(map[string]*Recipe)
	l.satisfiesIndex = nil
	l.satisfiesOnce = sync.Once{}
}

// CacheRecipe adds a recipe to the in-memory cache.
// This is useful for testing or loading recipes from non-standard sources.
func (l *Loader) CacheRecipe(name string, r *Recipe) {
	l.recipes[name] = r
}

// buildSatisfiesIndex scans providers implementing SatisfiesProvider for
// satisfies entries. Called lazily on first fallback lookup.
// The index is keyed by bare package name (not prefixed by ecosystem),
// because callers don't know which ecosystem a dependency comes from.
//
// Priority follows provider order: earlier providers' entries take precedence.
func (l *Loader) buildSatisfiesIndex() {
	l.satisfiesIndex = make(map[string]satisfiesEntry)

	for _, p := range l.providers {
		sp, ok := p.(SatisfiesProvider)
		if !ok {
			continue
		}

		entries, err := sp.SatisfiesEntries(context.Background())
		if err != nil {
			continue
		}

		source := p.Source()
		for pkgName, recipeName := range entries {
			if _, exists := l.satisfiesIndex[pkgName]; !exists {
				l.satisfiesIndex[pkgName] = satisfiesEntry{
					recipeName: recipeName,
					source:     source,
				}
			}
			// No warning for duplicates from later providers since
			// earlier providers win by design (same as pre-refactor behavior).
		}
	}
}

// lookupSatisfies checks if a name is satisfied by another recipe.
// Searches across all providers. Returns the satisfying recipe name
// and true, or "" and false. Triggers lazy index build on first call.
func (l *Loader) lookupSatisfies(name string) (string, bool) {
	l.satisfiesOnce.Do(l.buildSatisfiesIndex)
	entry, ok := l.satisfiesIndex[name]
	if !ok {
		return "", false
	}
	return entry.recipeName, true
}

// lookupSatisfiesFiltered checks if a name is satisfied by a recipe from
// a specific source. Used by getEmbeddedOnly to restrict to embedded sources.
func (l *Loader) lookupSatisfiesFiltered(name string, source RecipeSource) (string, bool) {
	l.satisfiesOnce.Do(l.buildSatisfiesIndex)
	entry, ok := l.satisfiesIndex[name]
	if !ok {
		return "", false
	}
	if entry.source != source {
		return "", false
	}
	return entry.recipeName, true
}

// LookupSatisfies checks whether a name is satisfied by an existing recipe.
// It exposes the satisfies index for callers that need the mapping without
// loading the full recipe.
// Returns the canonical recipe name and true if found, or "" and false.
func (l *Loader) LookupSatisfies(name string) (string, bool) {
	return l.lookupSatisfies(name)
}

// ListAllWithSource returns all available recipes from all providers.
// Priority order follows provider order (same as resolution chain).
func (l *Loader) ListAllWithSource() ([]RecipeInfo, error) {
	seen := make(map[string]bool)
	var result []RecipeInfo

	for _, p := range l.providers {
		recipes, err := p.List(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to list recipes from %s: %w", p.Source(), err)
		}
		for _, info := range recipes {
			if !seen[info.Name] {
				seen[info.Name] = true
				result = append(result, info)
			}
		}
	}

	return result, nil
}

// ListLocal returns only recipes from local providers.
func (l *Loader) ListLocal() ([]RecipeInfo, error) {
	for _, p := range l.providers {
		if p.Source() == SourceLocal {
			return p.List(context.Background())
		}
	}
	return nil, nil
}

// RecipesDir returns the local recipes directory, or "" if no local provider.
func (l *Loader) RecipesDir() string {
	for _, p := range l.providers {
		if lp, ok := p.(*LocalProvider); ok {
			return lp.Dir()
		}
	}
	return ""
}

// SetRecipesDir sets the local recipes directory by finding or adding a LocalProvider.
func (l *Loader) SetRecipesDir(dir string) {
	for i, p := range l.providers {
		if _, ok := p.(*LocalProvider); ok {
			l.providers[i] = NewLocalProvider(dir)
			return
		}
	}
	// No local provider found; add one at the beginning (highest priority)
	l.providers = append([]RecipeProvider{NewLocalProvider(dir)}, l.providers...)
}

// isNotFoundError checks if an error is a "not found" type error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for registry RegistryError with ErrTypeNotFound
	msg := err.Error()
	return containsNotFound(msg)
}

// containsNotFound is a simple check for common not-found error patterns.
func containsNotFound(msg string) bool {
	return len(msg) > 0 && (strings.Contains(msg, "not found") || strings.Contains(msg, "no such file"))
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

	// SourceCentral is the user-facing name for the central registry.
	// Both SourceRegistry and SourceEmbedded map to "central" for source tracking,
	// because embedded recipes are a cached subset of the central registry.
	SourceCentral = "central"
)

// RecipeInfo contains a recipe with its source information
type RecipeInfo struct {
	Name        string
	Description string
	Source      RecipeSource
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
	if r.Metadata.Type != RecipeTypeLibrary && (r.Verify == nil || r.Verify.Command == "") {
		return fmt.Errorf("verify.command is required")
	}

	return nil
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
