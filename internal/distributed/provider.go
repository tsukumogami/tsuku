package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/tsukumogami/tsuku/internal/httputil"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// manifestPaths lists the directories to probe for a manifest, in priority order.
// tsuku tries .tsuku-recipes/ first (canonical), then recipes/ as fallback.
var manifestPaths = []string{
	".tsuku-recipes/manifest.json",
	"recipes/manifest.json",
}

// defaultBranchesForProbe lists branch names to try when probing for manifests.
var defaultBranchesForProbe = []string{"main", "master"}

// DiscoverManifest probes a GitHub repository for a manifest file. It tries
// .tsuku-recipes/manifest.json first, then recipes/manifest.json. If neither
// exists, it returns a default flat manifest with no index.
//
// The client parameter is used for HTTP requests. If nil, a default secure
// client is used. The rawClient is used for unauthenticated raw content fetches.
func DiscoverManifest(ctx context.Context, owner, repo string, rawClient *http.Client) (recipe.Manifest, error) {
	if rawClient == nil {
		rawClient = httputil.NewSecureClient(httputil.DefaultOptions())
	}

	// Try each manifest path on each default branch
	for _, branch := range defaultBranchesForProbe {
		for _, path := range manifestPaths {
			manifest, err := fetchManifest(ctx, rawClient, owner, repo, branch, path)
			if err != nil {
				continue // Not found or error, try next
			}
			return manifest, nil
		}
	}

	// No manifest found: default to flat layout, no index
	return recipe.Manifest{Layout: "flat"}, nil
}

// fetchManifest attempts to fetch and parse a manifest from a specific branch and path.
func fetchManifest(ctx context.Context, client *http.Client, owner, repo, branch, path string) (recipe.Manifest, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, branch, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return recipe.Manifest{}, err
	}
	req.Header.Set("User-Agent", httputil.DefaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return recipe.Manifest{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return recipe.Manifest{}, fmt.Errorf("manifest not found: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return recipe.Manifest{}, err
	}

	var manifest recipe.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return recipe.Manifest{}, fmt.Errorf("parsing manifest: %w", err)
	}

	// Default layout to "flat" if empty
	if manifest.Layout == "" {
		manifest.Layout = "flat"
	}

	return manifest, nil
}

// distributedRegistryProvider wraps a RegistryProvider and adds Refresh support
// via the GitHubBackingStore.
type distributedRegistryProvider struct {
	*recipe.RegistryProvider
	store *GitHubBackingStore
}

// Refresh re-fetches the directory listing from the GitHub Contents API.
func (p *distributedRegistryProvider) Refresh(ctx context.Context) error {
	return p.store.ForceRefresh(ctx)
}

// NewDistributedRegistryProvider creates a RegistryProvider backed by a
// GitHubBackingStore for the given owner/repo. It discovers the manifest
// (or uses a default flat manifest) and configures the provider.
//
// The returned provider implements both RecipeProvider and RefreshableProvider.
func NewDistributedRegistryProvider(ctx context.Context, owner, repo string, client *GitHubClient) (recipe.RecipeProvider, error) {
	// Discover manifest from the repository
	manifest, err := DiscoverManifest(ctx, owner, repo, client.RawClient())
	if err != nil {
		// Fall back to default flat manifest on discovery error
		manifest = recipe.Manifest{Layout: "flat"}
	}

	store := NewGitHubBackingStore(owner, repo, client)
	source := recipe.RecipeSource(owner + "/" + repo)

	rp := recipe.NewRegistryProvider(
		owner+"/"+repo,
		source,
		manifest,
		store,
	)

	return &distributedRegistryProvider{
		RegistryProvider: rp,
		store:            store,
	}, nil
}

// NewDistributedRegistryProviderWithManifest creates a RegistryProvider with
// an explicitly provided manifest instead of discovering it. This is useful
// for testing and when the manifest has already been cached.
func NewDistributedRegistryProviderWithManifest(owner, repo string, manifest recipe.Manifest, client *GitHubClient) recipe.RecipeProvider {
	store := NewGitHubBackingStore(owner, repo, client)
	source := recipe.RecipeSource(owner + "/" + repo)

	rp := recipe.NewRegistryProvider(
		owner+"/"+repo,
		source,
		manifest,
		store,
	)

	return &distributedRegistryProvider{
		RegistryProvider: rp,
		store:            store,
	}
}
