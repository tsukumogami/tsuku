package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GraduationResult holds the outcome of the graduation step.
type GraduationResult struct {
	Kept      []SeedEntry
	Graduated []SeedEntry
	Backfills []BackfillResult
}

// BackfillResult records a metadata backfill applied to a recipe file.
type BackfillResult struct {
	Name        string
	RecipePath  string
	Description bool // true if description was backfilled
	Homepage    bool // true if homepage was backfilled
}

// GraduateEntries filters out entries that already have recipes, unless they
// are disambiguation entries. When graduating an entry, it also backfills
// missing metadata (description, homepage) into the recipe TOML file if the
// discovery entry has that data and the recipe doesn't.
func GraduateEntries(entries []SeedEntry, recipesDir string) (*GraduationResult, error) {
	if recipesDir == "" {
		return &GraduationResult{Kept: entries}, nil
	}

	// Build a set of existing recipe names (lowercase) and their paths
	recipeMap, err := buildRecipeMap(recipesDir)
	if err != nil {
		return nil, fmt.Errorf("scan recipes directory: %w", err)
	}

	result := &GraduationResult{}
	for _, e := range entries {
		recipePath, hasRecipe := recipeMap[strings.ToLower(e.Name)]
		if !hasRecipe {
			result.Kept = append(result.Kept, e)
			continue
		}

		if e.Disambiguation {
			result.Kept = append(result.Kept, e)
			continue
		}

		// Entry graduates — but first try to backfill metadata into the recipe
		if e.Description != "" || e.Homepage != "" {
			bf, err := backfillRecipeMetadata(recipePath, e)
			if err == nil && (bf.Description || bf.Homepage) {
				result.Backfills = append(result.Backfills, bf)
			}
		}

		result.Graduated = append(result.Graduated, e)
	}
	return result, nil
}

// buildRecipeMap scans the recipes directory and returns a map of lowercase
// recipe names to their file paths. Recipes are at {dir}/{letter}/{name}.toml.
func buildRecipeMap(dir string) (map[string]string, error) {
	m := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range entries {
		if !d.IsDir() || len(d.Name()) != 1 {
			continue
		}
		letterDir := filepath.Join(dir, d.Name())
		files, err := os.ReadDir(letterDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".toml") {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".toml")
			m[strings.ToLower(name)] = filepath.Join(letterDir, f.Name())
		}
	}
	return m, nil
}

// backfillRecipeMetadata reads a recipe TOML file and inserts missing
// description and/or homepage fields from the discovery entry. It operates
// on the raw text to avoid losing fields that the Recipe struct doesn't
// round-trip cleanly.
func backfillRecipeMetadata(recipePath string, entry SeedEntry) (BackfillResult, error) {
	result := BackfillResult{Name: entry.Name, RecipePath: recipePath}

	data, err := os.ReadFile(recipePath)
	if err != nil {
		return result, err
	}

	content := string(data)
	hasDescription := containsTOMLKey(content, "description")
	hasHomepage := containsTOMLKey(content, "homepage")

	if (hasDescription || entry.Description == "") && (hasHomepage || entry.Homepage == "") {
		return result, nil // nothing to backfill
	}

	lines := strings.Split(content, "\n")
	var out []string
	inserted := false

	for _, line := range lines {
		out = append(out, line)

		// Insert after the name = "..." line in [metadata]
		if !inserted && strings.HasPrefix(strings.TrimSpace(line), "name") && strings.Contains(line, "=") {
			if !hasDescription && entry.Description != "" {
				out = append(out, fmt.Sprintf("description = %q", entry.Description))
				result.Description = true
			}
			if !hasHomepage && entry.Homepage != "" {
				out = append(out, fmt.Sprintf("homepage = %q", entry.Homepage))
				result.Homepage = true
			}
			inserted = true
		}
	}

	if result.Description || result.Homepage {
		if err := os.WriteFile(recipePath, []byte(strings.Join(out, "\n")), 0644); err != nil {
			return result, err
		}
	}

	return result, nil
}

// containsTOMLKey checks if a TOML key exists in the content (not in a comment).
func containsTOMLKey(content, key string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key) && strings.Contains(trimmed, "=") {
			return true
		}
	}
	return false
}
