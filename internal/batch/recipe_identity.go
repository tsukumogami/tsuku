package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// RecipeIdentity is what a recipe file says about which tool it provides:
// its name, the queue-style source its steps install from, and the names it
// declares under [metadata.satisfies]. It is the single place a recipe's
// source is derived, shared by queue bootstrap and queue reconciliation.
type RecipeIdentity struct {
	// Path is the recipe file the identity was read from.
	Path string

	// Name is the recipe's metadata name.
	Name string

	// Source is the ecosystem:identifier the recipe installs from (for
	// example "homebrew:go-task" or "github:BurntSushi/ripgrep"), derived
	// from the first ecosystem-indicating step, falling back to
	// [version] github_repo. Empty when the recipe has neither.
	Source string

	// Satisfies holds [metadata.satisfies]: ecosystem keys mapping to the
	// package names the recipe stands in for, plus the reserved "aliases"
	// key for alternative command names.
	Satisfies map[string][]string
}

// ReadRecipeIdentity parses the parts of a recipe file that identify the tool
// it provides. A recipe with no derivable source is returned with an empty
// Source rather than an error.
func ReadRecipeIdentity(path string) (*RecipeIdentity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var raw recipeMinimal
	meta, err := toml.Decode(string(data), &raw)
	if err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	if raw.Metadata.Name == "" {
		return nil, fmt.Errorf("recipe has no name")
	}

	source := ""
	for _, prim := range raw.Steps {
		var step recipeStepMinimal
		if err := meta.PrimitiveDecode(prim, &step); err != nil {
			continue
		}
		source = sourceFromStep(step)
		if source != "" {
			break
		}
	}
	// Recipes that use generic download/extract steps but resolve versions
	// from GitHub releases (e.g., HashiCorp tools, Go SDK).
	if source == "" && raw.Version.GitHubRepo != "" {
		source = "github:" + raw.Version.GitHubRepo
	}

	return &RecipeIdentity{
		Path:      path,
		Name:      raw.Metadata.Name,
		Source:    source,
		Satisfies: raw.Metadata.Satisfies,
	}, nil
}

// ScanRecipeIdentities reads every .toml recipe under the given directories.
// A directory that does not exist is skipped. Files that cannot be parsed are
// skipped and returned as warnings, so one bad file does not hide the rest.
func ScanRecipeIdentities(dirs ...string) ([]RecipeIdentity, []string, error) {
	var ids []RecipeIdentity
	var warnings []string
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".toml") {
				return nil
			}
			id, err := ReadRecipeIdentity(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			ids = append(ids, *id)
			return nil
		})
		if err != nil {
			return nil, warnings, err
		}
	}
	return ids, warnings, nil
}

// NormalizeSource returns a source in the form used for comparison. Recipes
// install crates with cargo_install, which bootstrap records as "cargo:",
// while the queue's discovery pipeline records the same crates as
// "crates.io:"; both are compared as "crates.io:".
func NormalizeSource(source string) string {
	if strings.HasPrefix(source, "cargo:") {
		return "crates.io:" + strings.TrimPrefix(source, "cargo:")
	}
	return source
}

// normalizeEcosystem applies NormalizeSource's equivalence to a bare
// ecosystem name, such as a [metadata.satisfies] key.
func normalizeEcosystem(eco string) string {
	if eco == "cargo" {
		return "crates.io"
	}
	return eco
}
