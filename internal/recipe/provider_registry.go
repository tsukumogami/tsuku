package recipe

import (
	"os"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/registry"
)

const (
	// centralRegistryCacheTTL is the TTL for central registry cache entries.
	centralRegistryCacheTTL = 24 * time.Hour

	// centralRegistryCacheMaxSize is the maximum disk cache size for central
	// registry recipes (50 MB).
	centralRegistryCacheMaxSize = 50 * 1024 * 1024

	// centralRegistryCacheMaxStale is the stale-if-error window. When a fetch
	// fails and the cache entry is within this window past TTL, the stale
	// content is returned instead of an error.
	centralRegistryCacheMaxStale = 7 * 24 * time.Hour
)

// NewCentralRegistryProvider creates a RegistryProvider configured for the
// central recipe registry. It uses an HTTPStore backed by a DiskCache at
// cacheDir with grouped layout ({letter}/{name}.toml).
//
// The reg parameter is retained for operations that still require direct
// registry access (e.g., manifest refresh in update-registry). Access it
// via the RegistryAccessor interface.
func NewCentralRegistryProvider(reg *registry.Registry) *RegistryProvider {
	// Derive the base path for recipes from the registry's base URL.
	// Registry layout: {BaseURL}/recipes/{letter}/{name}.toml
	baseURL := reg.BaseURL + "/recipes"
	if envURL := os.Getenv(registry.EnvRegistryURL); envURL != "" {
		baseURL = envURL + "/recipes"
	}

	// Use FSStore for local filesystem registries (e.g., functional tests
	// that set TSUKU_REGISTRY_URL to a local path), HTTPStore otherwise.
	var store BackingStore
	if isLocalRegistryURL(baseURL) {
		store = NewFSStore(baseURL)
	} else {
		store = NewHTTPStore(HTTPStoreConfig{
			BaseURL:  baseURL,
			CacheDir: reg.CacheDir,
			TTL:      centralRegistryCacheTTL,
			MaxSize:  centralRegistryCacheMaxSize,
			Eviction: EvictLRU,
			MaxStale: centralRegistryCacheMaxStale,
			Client:   registry.NewRegistryHTTPClient(),
		})
	}

	manifest := Manifest{
		Layout:   "grouped",
		IndexURL: registry.DefaultManifestURL,
	}

	p := NewRegistryProvider("central-registry", SourceRegistry, manifest, store)
	p.registry = reg
	return p
}

// isLocalRegistryURL returns true if the URL is a local filesystem path.
func isLocalRegistryURL(url string) bool {
	return !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://")
}
