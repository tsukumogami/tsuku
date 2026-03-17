package recipe

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/registry"
)

// CentralRegistryProvider wraps a *registry.Registry to implement RecipeProvider.
// It also implements SatisfiesProvider (via manifest) and RefreshableProvider.
type CentralRegistryProvider struct {
	registry *registry.Registry
}

// NewCentralRegistryProvider creates a CentralRegistryProvider wrapping the given Registry.
func NewCentralRegistryProvider(reg *registry.Registry) *CentralRegistryProvider {
	return &CentralRegistryProvider{registry: reg}
}

// Get retrieves raw recipe TOML bytes from the registry (disk cache or remote).
func (p *CentralRegistryProvider) Get(ctx context.Context, name string) ([]byte, error) {
	// Check disk cache first
	data, err := p.registry.GetCached(name)
	if err != nil {
		return nil, err
	}

	if data != nil {
		return data, nil
	}

	// Not cached, fetch from remote
	data, err = p.registry.FetchRecipe(ctx, name)
	if err != nil {
		return nil, err
	}

	// Cache the fetched recipe
	if cacheErr := p.registry.CacheRecipe(name, data); cacheErr != nil {
		fmt.Printf("Warning: failed to cache recipe %s: %v\n", name, cacheErr)
	}

	return data, nil
}

// List returns metadata for all cached registry recipes.
func (p *CentralRegistryProvider) List(_ context.Context) ([]RecipeInfo, error) {
	names, err := p.registry.ListCached()
	if err != nil {
		return nil, err
	}

	var result []RecipeInfo
	for _, name := range names {
		description := ""
		data, err := p.registry.GetCached(name)
		if err == nil && data != nil {
			// Parse just enough to get description
			if r, err := quickParseDescription(data); err == nil {
				description = r
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

// Source returns SourceRegistry.
func (p *CentralRegistryProvider) Source() RecipeSource {
	return SourceRegistry
}

// SatisfiesEntries returns satisfies mappings from the cached manifest.
func (p *CentralRegistryProvider) SatisfiesEntries(_ context.Context) (map[string]string, error) {
	manifest, err := p.registry.GetCachedManifest()
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, nil
	}

	result := make(map[string]string)
	for _, entry := range manifest.Recipes {
		for _, pkgNames := range entry.Satisfies {
			for _, pkgName := range pkgNames {
				if _, exists := result[pkgName]; !exists {
					result[pkgName] = entry.Name
				}
			}
		}
	}

	return result, nil
}

// Refresh is not implemented directly on CentralRegistryProvider.
// The update-registry command uses Registry() to access internals.
// This method exists to satisfy the RefreshableProvider interface at a basic level.
func (p *CentralRegistryProvider) Refresh(_ context.Context) error {
	// The real refresh logic lives in the update-registry command,
	// which uses Registry() + CachedRegistry for fine-grained control.
	return nil
}

// Registry returns the underlying *registry.Registry for escape-hatch access.
// This is used by update-registry for cache-level operations that don't
// belong on the provider interface.
func (p *CentralRegistryProvider) Registry() *registry.Registry {
	return p.registry
}

// quickParseDescription extracts the description from recipe TOML bytes
// without a full parse. Uses TOML unmarshaling of just the metadata section.
func quickParseDescription(data []byte) (string, error) {
	var partial struct {
		Metadata struct {
			Description string `toml:"description"`
		} `toml:"metadata"`
	}
	if err := toml.Unmarshal(data, &partial); err != nil {
		return "", err
	}
	return partial.Metadata.Description, nil
}
