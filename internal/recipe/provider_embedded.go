package recipe

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
)

// EmbeddedProvider wraps an EmbeddedRegistry to implement RecipeProvider.
type EmbeddedProvider struct {
	registry *EmbeddedRegistry
}

// NewEmbeddedProvider creates an EmbeddedProvider from an EmbeddedRegistry.
// Returns nil if the registry is nil.
func NewEmbeddedProvider(er *EmbeddedRegistry) *EmbeddedProvider {
	if er == nil {
		return nil
	}
	return &EmbeddedProvider{registry: er}
}

// Get retrieves raw recipe TOML bytes from embedded recipes.
func (p *EmbeddedProvider) Get(_ context.Context, name string) ([]byte, error) {
	data, ok := p.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in embedded registry", name)
	}
	return data, nil
}

// List returns metadata for all embedded recipes.
func (p *EmbeddedProvider) List(_ context.Context) ([]RecipeInfo, error) {
	return p.registry.ListWithInfo()
}

// Source returns SourceEmbedded.
func (p *EmbeddedProvider) Source() RecipeSource {
	return SourceEmbedded
}

// SatisfiesEntries returns satisfies mappings by parsing all embedded recipes.
func (p *EmbeddedProvider) SatisfiesEntries(_ context.Context) (map[string]string, error) {
	result := make(map[string]string)

	for _, name := range p.registry.List() {
		data, ok := p.registry.Get(name)
		if !ok {
			continue
		}
		var r Recipe
		if err := toml.Unmarshal(data, &r); err != nil {
			continue
		}
		for _, pkgNames := range r.Metadata.Satisfies {
			for _, pkgName := range pkgNames {
				if _, exists := result[pkgName]; !exists {
					result[pkgName] = name
				} else {
					fmt.Printf("Warning: duplicate satisfies entry %q (claimed by %q and %q)\n",
						pkgName, result[pkgName], name)
				}
			}
		}
	}

	return result, nil
}

// Has checks if the embedded registry contains a recipe with the given name.
func (p *EmbeddedProvider) Has(name string) bool {
	return p.registry.Has(name)
}
