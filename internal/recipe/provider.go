package recipe

import (
	"context"
)

// RecipeProvider is a source of recipe TOML data. Providers are ordered
// by priority in the Loader's chain; earlier providers shadow later ones.
type RecipeProvider interface {
	// Get retrieves raw recipe TOML bytes by name.
	// Returns nil, ErrRecipeNotFound if the recipe doesn't exist in this source.
	Get(ctx context.Context, name string) ([]byte, error)

	// List returns metadata for all recipes available from this source.
	List(ctx context.Context) ([]RecipeInfo, error)

	// Source returns the source tag for this provider.
	Source() RecipeSource
}

// SatisfiesProvider is optional. Providers that can cheaply return
// package-name-to-recipe-name mappings implement it for the satisfies index.
type SatisfiesProvider interface {
	// SatisfiesEntries returns a map of package_name -> recipe_name for
	// all satisfies entries this provider knows about.
	SatisfiesEntries(ctx context.Context) (map[string]string, error)
}

// RefreshableProvider is optional. Providers with cached upstream data
// implement it for tsuku update-registry.
type RefreshableProvider interface {
	Refresh(ctx context.Context) error
}

// satisfiesEntry tracks which provider contributed a satisfies index entry.
type satisfiesEntry struct {
	recipeName string
	source     RecipeSource
}
