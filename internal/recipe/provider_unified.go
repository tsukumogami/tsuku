package recipe

import (
	"context"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// Manifest describes the layout and optional index of a recipe registry.
type Manifest struct {
	Layout   string `json:"layout"`    // "flat" or "grouped"
	IndexURL string `json:"index_url"` // optional URL for pre-built recipe index
}

// RegistryProvider is a unified RecipeProvider implementation that works with
// any BackingStore. It replaces EmbeddedProvider and LocalProvider with a
// single type parameterized by a Manifest and a store.
type RegistryProvider struct {
	name     string
	source   RecipeSource
	manifest Manifest
	store    BackingStore
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

// SatisfiesEntries returns satisfies mappings by parsing all recipes in the store.
func (p *RegistryProvider) SatisfiesEntries(ctx context.Context) (map[string]string, error) {
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

		for _, pkgNames := range r.Metadata.Satisfies {
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
