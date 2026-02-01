package registry

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
)

const (
	// DefaultRegistryURL is the default URL for the tsuku recipe registry
	// Points to the repo root; recipeURL appends /recipes/{letter}/{name}.toml
	DefaultRegistryURL = "https://raw.githubusercontent.com/tsukumogami/tsuku/main"

	// EnvRegistryURL is the environment variable to override the registry URL
	EnvRegistryURL = "TSUKU_REGISTRY_URL"
)

// Registry handles fetching recipes from the remote registry
type Registry struct {
	BaseURL  string // Base URL for raw recipe files
	CacheDir string // Local cache directory (~/.tsuku/registry)
	client   *http.Client
}

// newRegistryHTTPClient creates a secure HTTP client for registry operations with:
// - DisableCompression: prevents decompression bomb attacks
// - Proper timeouts
func newRegistryHTTPClient() *http.Client {
	return &http.Client{
		Timeout: config.GetAPITimeout(),
		Transport: &http.Transport{
			DisableCompression: true, // CRITICAL: Prevents decompression bomb attacks
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
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
		client:   newRegistryHTTPClient(),
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
		return nil, &RegistryError{
			Type:    ErrTypeValidation,
			Recipe:  name,
			Message: "invalid recipe name",
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &RegistryError{
			Type:    ErrTypeNetwork,
			Recipe:  name,
			Message: "failed to create request",
			Err:     err,
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, name, "failed to fetch recipe")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &RegistryError{
			Type:    ErrTypeNotFound,
			Recipe:  name,
			Message: fmt.Sprintf("recipe %s not found in registry", name),
		}
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RegistryError{
			Type:    ErrTypeRateLimit,
			Recipe:  name,
			Message: "registry rate limit exceeded",
		}
	}

	if resp.StatusCode != http.StatusOK {
		errType := ErrTypeNetwork
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			errType = ErrTypeValidation
		}
		return nil, &RegistryError{
			Type:    errType,
			Recipe:  name,
			Message: fmt.Sprintf("registry returned status %d", resp.StatusCode),
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RegistryError{
			Type:    ErrTypeParsing,
			Recipe:  name,
			Message: "failed to read recipe content",
			Err:     err,
		}
	}

	return data, nil
}

// GetCached returns a cached recipe if it exists.
// It also updates the LastAccess timestamp and creates metadata for
// pre-existing cached recipes (migration case).
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

	// Check if metadata exists; if not, create it (migration case)
	meta, _ := r.ReadMeta(name)
	if meta == nil {
		// Create metadata for existing cached recipe using file mtime
		meta, err = newCacheMetadataFromFile(path, data, DefaultCacheTTL)
		if err == nil {
			_ = r.WriteMeta(name, meta) // Best effort, don't fail the read
		}
	} else {
		// Update last access time
		_ = r.UpdateLastAccess(name) // Best effort
	}

	return data, nil
}

// CacheRecipe saves a recipe to the local cache and writes metadata sidecar.
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

	// Write metadata sidecar (best effort - don't fail the cache operation)
	meta := newCacheMetadata(data, DefaultCacheTTL)
	_ = r.WriteMeta(name, meta)

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

// ListCached returns all cached recipe names
func (r *Registry) ListCached() ([]string, error) {
	if r.CacheDir == "" {
		return nil, nil
	}

	var names []string

	// Walk the cache directory structure (letter/name.toml)
	entries, err := os.ReadDir(r.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Read recipes in each letter directory
		letterDir := filepath.Join(r.CacheDir, entry.Name())
		recipeEntries, err := os.ReadDir(letterDir)
		if err != nil {
			continue
		}

		for _, recipeEntry := range recipeEntries {
			if recipeEntry.IsDir() {
				continue
			}
			name := recipeEntry.Name()
			if strings.HasSuffix(name, ".toml") {
				names = append(names, strings.TrimSuffix(name, ".toml"))
			}
		}
	}

	return names, nil
}
