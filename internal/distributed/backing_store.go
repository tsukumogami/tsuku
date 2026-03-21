package distributed

import (
	"context"
	"fmt"
	"strings"
)

// GitHubBackingStore adapts GitHubClient to satisfy recipe.BackingStore.
// It delegates Get to FetchRecipe and List to ListRecipes, translating
// between the BackingStore path-based interface and the GitHubClient's
// owner/repo/name-based interface.
type GitHubBackingStore struct {
	owner  string
	repo   string
	client *GitHubClient
}

// NewGitHubBackingStore creates a BackingStore backed by a GitHubClient.
func NewGitHubBackingStore(owner, repo string, client *GitHubClient) *GitHubBackingStore {
	return &GitHubBackingStore{
		owner:  owner,
		repo:   repo,
		client: client,
	}
}

// Get retrieves raw bytes for a recipe at the given path. The path is expected
// to be in the form "name.toml" (flat) or "n/name.toml" (grouped). It strips
// directory prefixes and the .toml extension to get the recipe name, then
// fetches via the GitHubClient.
func (s *GitHubBackingStore) Get(ctx context.Context, path string) ([]byte, error) {
	name := recipeNameFromStorePath(path)

	meta, err := s.client.ListRecipes(ctx, s.owner, s.repo)
	if err != nil {
		return nil, fmt.Errorf("listing recipes from %s/%s: %w", s.owner, s.repo, err)
	}

	downloadURL, ok := meta.Files[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in %s/%s", name, s.owner, s.repo)
	}

	return s.client.FetchRecipe(ctx, s.owner, s.repo, name, downloadURL)
}

// List returns all recipe file paths available from the repository. Paths are
// returned in flat layout format (e.g., "my-tool.toml").
func (s *GitHubBackingStore) List(ctx context.Context) ([]string, error) {
	meta, err := s.client.ListRecipes(ctx, s.owner, s.repo)
	if err != nil {
		return nil, fmt.Errorf("listing recipes from %s/%s: %w", s.owner, s.repo, err)
	}

	paths := make([]string, 0, len(meta.Files))
	for name := range meta.Files {
		paths = append(paths, name+".toml")
	}
	return paths, nil
}

// ForceRefresh re-fetches the directory listing from the Contents API,
// bypassing the cache freshness check. Used by the Refresh method on the
// RegistryProvider returned by NewDistributedRegistryProvider.
func (s *GitHubBackingStore) ForceRefresh(ctx context.Context) error {
	_, err := s.client.ForceListRecipes(ctx, s.owner, s.repo)
	if err != nil {
		return fmt.Errorf("refreshing recipe listing from %s/%s: %w", s.owner, s.repo, err)
	}
	return nil
}

// recipeNameFromStorePath extracts a recipe name from a store path.
// Handles both flat ("go.toml") and grouped ("g/go.toml") layouts.
func recipeNameFromStorePath(path string) string {
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	return strings.TrimSuffix(base, ".toml")
}
