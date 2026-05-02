package recipe

import (
	"context"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/registry"
)

// Manifest describes the layout and optional index of a recipe registry.
type Manifest struct {
	Layout   string `json:"layout"`    // "flat" or "grouped"
	IndexURL string `json:"index_url"` // optional URL for pre-built recipe index
}

// RegistryProvider is a unified RecipeProvider implementation that works with
// any BackingStore. It replaces EmbeddedProvider, LocalProvider, and
// CentralRegistryProvider with a single type parameterized by a Manifest
// and a store.
type RegistryProvider struct {
	name     string
	source   RecipeSource
	manifest Manifest
	store    BackingStore

	// registry holds a reference to the underlying *registry.Registry when
	// this provider wraps the central registry. Used by update-registry for
	// manifest refresh and cache-level operations that don't belong on the
	// generic provider interface.
	registry *registry.Registry
}

// NewRegistryProvider creates a RegistryProvider with the given configuration.
func NewRegistryProvider(name string, source RecipeSource, manifest Manifest, store BackingStore) *RegistryProvider {
	return &RegistryProvider{
		name:     name,
		source:   source,
		manifest: manifest,
		store:    store,
	}
}

// Get retrieves raw recipe TOML bytes by name.
func (p *RegistryProvider) Get(ctx context.Context, name string) ([]byte, error) {
	path := p.recipePath(name)
	return p.store.Get(ctx, path)
}

// List returns metadata for all recipes available from this provider.
func (p *RegistryProvider) List(ctx context.Context) ([]RecipeInfo, error) {
	paths, err := p.store.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []RecipeInfo
	for _, path := range paths {
		if !strings.HasSuffix(path, ".toml") {
			continue
		}

		recipeName := recipeNameFromPath(path)
		description := ""

		data, err := p.store.Get(ctx, path)
		if err == nil {
			var r Recipe
			if err := toml.Unmarshal(data, &r); err == nil {
				description = r.Metadata.Description
			}
		}

		result = append(result, RecipeInfo{
			Name:        recipeName,
			Description: description,
			Source:      p.source,
		})
	}

	return result, nil
}

// Source returns the source tag for this provider.
func (p *RegistryProvider) Source() RecipeSource {
	return p.source
}

// SatisfiesEntries returns satisfies mappings. When the provider wraps the
// central registry, it reads from the cached manifest (recipes.json) for
// efficiency. Otherwise, it parses individual recipes in the store.
func (p *RegistryProvider) SatisfiesEntries(ctx context.Context) (map[string]string, error) {
	// If we have a registry with a cached manifest, use it (more efficient
	// than parsing every recipe).
	if p.registry != nil {
		return p.satisfiesFromManifest()
	}
	return p.satisfiesFromRecipes(ctx)
}

// satisfiesFromManifest reads satisfies entries from the cached registry manifest.
// The reserved key "aliases" is excluded — it has multi-recipe semantics
// handled by AliasesEntries / aliasesFromManifest, not the 1:1 satisfies index.
func (p *RegistryProvider) satisfiesFromManifest() (map[string]string, error) {
	manifest, err := p.registry.GetCachedManifest()
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, nil
	}

	result := make(map[string]string)
	for _, entry := range manifest.Recipes {
		for ecosystem, pkgNames := range entry.Satisfies {
			if ecosystem == AliasesKey {
				continue
			}
			for _, pkgName := range pkgNames {
				if _, exists := result[pkgName]; !exists {
					result[pkgName] = entry.Name
				}
			}
		}
	}

	return result, nil
}

// satisfiesFromRecipes builds satisfies mappings by parsing all recipes in the store.
func (p *RegistryProvider) satisfiesFromRecipes(ctx context.Context) (map[string]string, error) {
	paths, err := p.store.List(ctx)
	if err != nil {
		if paths == nil {
			return nil, err
		}
		// Partial list -- continue with what we have
	}

	if paths == nil {
		return nil, nil
	}

	result := make(map[string]string)
	for _, path := range paths {
		if !strings.HasSuffix(path, ".toml") {
			continue
		}

		recipeName := recipeNameFromPath(path)
		data, err := p.store.Get(ctx, path)
		if err != nil {
			continue
		}

		var r Recipe
		if err := toml.Unmarshal(data, &r); err != nil {
			continue
		}

		for ecosystem, pkgNames := range r.Metadata.Satisfies {
			if ecosystem == AliasesKey {
				continue
			}
			for _, pkgName := range pkgNames {
				if _, exists := result[pkgName]; !exists {
					result[pkgName] = recipeName
				} else {
					fmt.Printf("Warning: duplicate satisfies entry %q (claimed by %q and %q)\n",
						pkgName, result[pkgName], recipeName)
				}
			}
		}
	}

	return result, nil
}

// AliasesEntries returns alias -> []recipe_name mappings declared under
// [metadata.satisfies] aliases = [...]. Multiple recipes may claim the
// same alias; the loader's alias index is multi-valued.
func (p *RegistryProvider) AliasesEntries(ctx context.Context) (map[string][]string, error) {
	if p.registry != nil {
		return p.aliasesFromManifest()
	}
	return p.aliasesFromRecipes(ctx)
}

// aliasesFromManifest reads alias entries from the cached registry manifest.
func (p *RegistryProvider) aliasesFromManifest() (map[string][]string, error) {
	manifest, err := p.registry.GetCachedManifest()
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, nil
	}

	result := make(map[string][]string)
	for _, entry := range manifest.Recipes {
		aliases, ok := entry.Satisfies[AliasesKey]
		if !ok {
			continue
		}
		for _, alias := range aliases {
			result[alias] = append(result[alias], entry.Name)
		}
	}

	return result, nil
}

// aliasesFromRecipes builds alias entries by parsing every recipe in the store.
func (p *RegistryProvider) aliasesFromRecipes(ctx context.Context) (map[string][]string, error) {
	paths, err := p.store.List(ctx)
	if err != nil {
		if paths == nil {
			return nil, err
		}
	}

	if paths == nil {
		return nil, nil
	}

	result := make(map[string][]string)
	for _, path := range paths {
		if !strings.HasSuffix(path, ".toml") {
			continue
		}

		recipeName := recipeNameFromPath(path)
		data, err := p.store.Get(ctx, path)
		if err != nil {
			continue
		}

		var r Recipe
		if err := toml.Unmarshal(data, &r); err != nil {
			continue
		}

		aliases, ok := r.Metadata.Satisfies[AliasesKey]
		if !ok {
			continue
		}
		for _, alias := range aliases {
			result[alias] = append(result[alias], recipeName)
		}
	}

	return result, nil
}

// Has checks if the store contains a recipe with the given name.
func (p *RegistryProvider) Has(ctx context.Context, name string) bool {
	path := p.recipePath(name)
	data, err := p.store.Get(ctx, path)
	return err == nil && data != nil
}

// Store returns the backing store. Used by callers that need to inspect the
// store type (e.g., FSStore for directory access).
func (p *RegistryProvider) Store() BackingStore {
	return p.store
}

// recipePath computes the store path for a recipe name based on the manifest layout.
func (p *RegistryProvider) recipePath(name string) string {
	switch p.manifest.Layout {
	case "grouped":
		if len(name) == 0 {
			return name + ".toml"
		}
		return string(name[0]) + "/" + name + ".toml"
	default: // "flat" or empty
		return name + ".toml"
	}
}

// Refresh implements RefreshableProvider. When the backing store is an
// HTTPStore, it clears stale cache entries so the next Get re-fetches them.
// For stores without caching (MemoryStore, FSStore), this is a no-op.
func (p *RegistryProvider) Refresh(_ context.Context) error {
	// HTTPStore's DiskCache handles freshness via TTL; a "refresh" means
	// clearing the cache so entries are re-fetched on next access. This is
	// a coarse-grained refresh -- update-registry uses CachedRegistry for
	// fine-grained per-recipe control.
	if hs, ok := p.store.(*HTTPStore); ok {
		cache := hs.Cache()
		keys, err := cache.Keys()
		if err != nil {
			return fmt.Errorf("listing cache keys: %w", err)
		}
		for _, key := range keys {
			_ = cache.Delete(key)
		}
	}
	return nil
}

// CacheStats implements CacheIntrospectable. Returns cache statistics when
// the backing store is an HTTPStore, or nil otherwise.
func (p *RegistryProvider) CacheStats() (*DiskCacheStats, error) {
	if hs, ok := p.store.(*HTTPStore); ok {
		return hs.CacheStats()
	}
	return nil, nil
}

// Registry returns the underlying *registry.Registry when this provider wraps
// the central registry. Returns nil for providers without a registry backing
// (embedded, local, distributed). Used by update-registry for manifest refresh
// and cache-level operations.
func (p *RegistryProvider) Registry() *registry.Registry {
	return p.registry
}

// RegistryAccessor is an optional interface for providers that wrap a
// *registry.Registry. The update-registry command uses this to access
// the registry for manifest refresh and per-recipe cache management.
type RegistryAccessor interface {
	Registry() *registry.Registry
}

// Dir returns the filesystem directory when the backing store is an FSStore.
// Returns "" for non-filesystem stores (memory, HTTP).
func (p *RegistryProvider) Dir() string {
	if fs, ok := p.store.(*FSStore); ok {
		return fs.Dir()
	}
	return ""
}

// recipeNameFromPath extracts a recipe name from a store path.
// Handles both flat ("go.toml") and grouped ("g/go.toml") layouts.
func recipeNameFromPath(path string) string {
	// Strip directory prefix if present
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	return strings.TrimSuffix(base, ".toml")
}
