package recipe

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed recipes/*/*.toml
var embeddedRecipes embed.FS

// EmbeddedRegistry provides access to recipes embedded in the binary
type EmbeddedRegistry struct {
	recipes map[string][]byte // name -> raw TOML bytes
}

// NewEmbeddedRegistry creates a new embedded registry by loading all embedded recipes
func NewEmbeddedRegistry() (*EmbeddedRegistry, error) {
	er := &EmbeddedRegistry{
		recipes: make(map[string][]byte),
	}

	err := fs.WalkDir(embeddedRecipes, "recipes", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".toml") {
			return nil
		}

		data, err := embeddedRecipes.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded recipe %s: %w", path, err)
		}

		// Extract recipe name from path (e.g., "recipes/a/actionlint.toml" -> "actionlint")
		name := strings.TrimSuffix(filepath.Base(path), ".toml")
		er.recipes[name] = data

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load embedded recipes: %w", err)
	}

	return er, nil
}

// Get retrieves an embedded recipe by name
func (er *EmbeddedRegistry) Get(name string) ([]byte, bool) {
	data, ok := er.recipes[name]
	return data, ok
}

// Has checks if an embedded recipe exists
func (er *EmbeddedRegistry) Has(name string) bool {
	_, ok := er.recipes[name]
	return ok
}

// List returns all embedded recipe names
func (er *EmbeddedRegistry) List() []string {
	names := make([]string, 0, len(er.recipes))
	for name := range er.recipes {
		names = append(names, name)
	}
	return names
}

// Count returns the number of embedded recipes
func (er *EmbeddedRegistry) Count() int {
	return len(er.recipes)
}

// ListWithInfo returns all embedded recipes with their metadata
func (er *EmbeddedRegistry) ListWithInfo() ([]RecipeInfo, error) {
	var result []RecipeInfo

	for name, data := range er.recipes {
		var recipe Recipe
		if err := toml.Unmarshal(data, &recipe); err != nil {
			// Skip recipes that fail to parse
			continue
		}

		result = append(result, RecipeInfo{
			Name:        name,
			Description: recipe.Metadata.Description,
			Source:      SourceEmbedded,
		})
	}

	return result, nil
}
