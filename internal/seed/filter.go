package seed

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// FilterExistingRecipes removes packages that already have recipes in either
// the registry or embedded recipe directories. Returns the filtered slice and
// a list of skipped package names.
func FilterExistingRecipes(packages []Package, recipesDir, embeddedDir string) ([]Package, []string) {
	var kept []Package
	var skipped []string
	for _, p := range packages {
		if recipeExists(p.Name, recipesDir, embeddedDir) {
			skipped = append(skipped, p.Name)
			continue
		}
		kept = append(kept, p)
	}
	return kept, skipped
}

// recipeExists checks whether a recipe file exists for the given name in
// either the registry directory (letter-prefixed) or embedded directory (flat).
func recipeExists(name, recipesDir, embeddedDir string) bool {
	lower := strings.ToLower(name)
	if recipesDir != "" {
		first := string(unicode.ToLower(rune(lower[0])))
		path := filepath.Join(recipesDir, first, lower+".toml")
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	if embeddedDir != "" {
		path := filepath.Join(embeddedDir, lower+".toml")
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
