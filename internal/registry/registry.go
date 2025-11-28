package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultRegistryURL is the default URL for the tsuku recipe registry
	DefaultRegistryURL = "https://raw.githubusercontent.com/tsuku-dev/tsuku-registry/main"

	// EnvRegistryURL is the environment variable to override the registry URL
	EnvRegistryURL = "TSUKU_REGISTRY_URL"

	// fetchTimeout is the timeout for fetching a single recipe
	fetchTimeout = 30 * time.Second
)

// Registry handles fetching recipes from the remote registry
type Registry struct {
	BaseURL  string // Base URL for raw recipe files
	CacheDir string // Local cache directory (~/.tsuku/registry)
	client   *http.Client
}

// New creates a new Registry with the given cache directory
func New(cacheDir string) *Registry {
	baseURL := os.Getenv(EnvRegistryURL)
	if baseURL == "" {
		baseURL = DefaultRegistryURL
	}

	return &Registry{
		BaseURL:  baseURL,
		CacheDir: cacheDir,
		client: &http.Client{
			Timeout: fetchTimeout,
		},
	}
}

// recipeURL returns the URL for a recipe file
// Registry structure: recipes/{first-letter}/{name}.toml
func (r *Registry) recipeURL(name string) string {
	if name == "" {
		return ""
	}
	firstLetter := strings.ToLower(string(name[0]))
	return fmt.Sprintf("%s/recipes/%s/%s.toml", r.BaseURL, firstLetter, name)
}

// cachePath returns the local cache path for a recipe
func (r *Registry) cachePath(name string) string {
	if name == "" {
		return ""
	}
	firstLetter := strings.ToLower(string(name[0]))
	return filepath.Join(r.CacheDir, firstLetter, name+".toml")
}

// FetchRecipe fetches a recipe from the registry
// Returns the recipe content as bytes, or an error if not found
func (r *Registry) FetchRecipe(ctx context.Context, name string) ([]byte, error) {
	url := r.recipeURL(name)
	if url == "" {
		return nil, fmt.Errorf("invalid recipe name")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("recipe not found in registry: %s", name)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d for recipe %s", resp.StatusCode, name)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe content: %w", err)
	}

	return data, nil
}

// GetCached returns a cached recipe if it exists
func (r *Registry) GetCached(name string) ([]byte, error) {
	path := r.cachePath(name)
	if path == "" {
		return nil, fmt.Errorf("invalid recipe name")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not cached, not an error
		}
		return nil, fmt.Errorf("failed to read cached recipe: %w", err)
	}

	return data, nil
}

// CacheRecipe saves a recipe to the local cache
func (r *Registry) CacheRecipe(name string, data []byte) error {
	path := r.cachePath(name)
	if path == "" {
		return fmt.Errorf("invalid recipe name")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cached recipe: %w", err)
	}

	return nil
}

// ClearCache removes all cached recipes
func (r *Registry) ClearCache() error {
	if r.CacheDir == "" {
		return fmt.Errorf("cache directory not set")
	}

	// Remove and recreate the cache directory
	if err := os.RemoveAll(r.CacheDir); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	if err := os.MkdirAll(r.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to recreate cache directory: %w", err)
	}

	return nil
}

// IsCached checks if a recipe is cached locally
func (r *Registry) IsCached(name string) bool {
	path := r.cachePath(name)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
