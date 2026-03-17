package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// LocalProvider loads recipes from a local filesystem directory ($TSUKU_HOME/recipes).
type LocalProvider struct {
	dir string
}

// NewLocalProvider creates a LocalProvider for the given directory.
func NewLocalProvider(dir string) *LocalProvider {
	return &LocalProvider{dir: dir}
}

// Get retrieves raw recipe TOML bytes from the local directory.
func (p *LocalProvider) Get(_ context.Context, name string) ([]byte, error) {
	path := filepath.Join(p.dir, name+".toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// List returns metadata for all recipes in the local directory.
func (p *LocalProvider) List(_ context.Context) ([]RecipeInfo, error) {
	if p.dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(p.dir)
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
		data, err := os.ReadFile(filepath.Join(p.dir, name))
		if err == nil {
			var r Recipe
			if err := toml.Unmarshal(data, &r); err == nil {
				description = r.Metadata.Description
			}
		}

		result = append(result, RecipeInfo{
			Name:        recipeName,
			Description: description,
			Source:      SourceLocal,
		})
	}

	return result, nil
}

// Source returns SourceLocal.
func (p *LocalProvider) Source() RecipeSource {
	return SourceLocal
}

// SatisfiesEntries returns satisfies mappings by parsing all local recipes.
func (p *LocalProvider) SatisfiesEntries(_ context.Context) (map[string]string, error) {
	if p.dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	result := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !isTomlFile(entry.Name()) {
			continue
		}

		recipeName := trimTomlExtension(entry.Name())
		data, err := os.ReadFile(filepath.Join(p.dir, entry.Name()))
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

// Dir returns the local recipes directory path.
func (p *LocalProvider) Dir() string {
	return p.dir
}
