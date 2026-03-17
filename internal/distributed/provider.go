package distributed

import (
	"context"
	"fmt"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// DistributedProvider fetches recipes from a single GitHub repository's
// .tsuku-recipes/ directory. It implements RecipeProvider and RefreshableProvider.
type DistributedProvider struct {
	owner  string
	repo   string
	client *GitHubClient
}

// NewDistributedProvider creates a provider for the given owner/repo.
func NewDistributedProvider(owner, repo string, client *GitHubClient) *DistributedProvider {
	return &DistributedProvider{
		owner:  owner,
		repo:   repo,
		client: client,
	}
}

// Get retrieves raw recipe TOML bytes by name from the repository's
// .tsuku-recipes/ directory. It looks up the download URL from the cached
// SourceMeta and fetches the file content.
func (p *DistributedProvider) Get(ctx context.Context, name string) ([]byte, error) {
	// Get the source listing (from cache or API)
	meta, err := p.client.ListRecipes(ctx, p.owner, p.repo)
	if err != nil {
		return nil, fmt.Errorf("listing recipes from %s/%s: %w", p.owner, p.repo, err)
	}

	downloadURL, ok := meta.Files[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in %s/%s", name, p.owner, p.repo)
	}

	data, err := p.client.FetchRecipe(ctx, p.owner, p.repo, name, downloadURL)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// List returns metadata for all recipes available from this repository.
// It uses the cached directory listing from the GitHub Contents API.
func (p *DistributedProvider) List(ctx context.Context) ([]recipe.RecipeInfo, error) {
	meta, err := p.client.ListRecipes(ctx, p.owner, p.repo)
	if err != nil {
		return nil, fmt.Errorf("listing recipes from %s/%s: %w", p.owner, p.repo, err)
	}

	result := make([]recipe.RecipeInfo, 0, len(meta.Files))
	for name := range meta.Files {
		result = append(result, recipe.RecipeInfo{
			Name:   name,
			Source: p.Source(),
		})
	}

	return result, nil
}

// Source returns a RecipeSource identifying this provider as "owner/repo".
func (p *DistributedProvider) Source() recipe.RecipeSource {
	return recipe.RecipeSource(p.owner + "/" + p.repo)
}

// Refresh re-fetches the directory listing from the GitHub Contents API,
// bypassing the cache freshness check. Used by update-registry.
func (p *DistributedProvider) Refresh(ctx context.Context) error {
	_, err := p.client.ForceListRecipes(ctx, p.owner, p.repo)
	if err != nil {
		return fmt.Errorf("refreshing recipe listing from %s/%s: %w", p.owner, p.repo, err)
	}
	return nil
}

// Owner returns the repository owner.
func (p *DistributedProvider) Owner() string {
	return p.owner
}

// Repo returns the repository name.
func (p *DistributedProvider) Repo() string {
	return p.repo
}
