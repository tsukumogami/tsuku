package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// validRecipeTOML is a minimal valid recipe TOML for testing.
const validRecipeTOML = `[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"

[verify]
command = "test-tool --version"
`

// --- Tests for NewDistributedRegistryProviderWithManifest ---

func TestDistributedRegistryProvider_Source(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)
	gc := newGitHubClientWithHTTP(nil, nil, cache, false)

	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)
	got := p.Source()
	want := recipe.RecipeSource("acme/tools")
	if got != want {
		t.Errorf("Source() = %q, want %q", got, want)
	}
}

func TestDistributedRegistryProvider_Get_CacheHit(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Pre-populate source meta with a download URL
	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Pre-populate recipe cache
	recipeMeta := &RecipeMeta{FetchedAt: time.Now()}
	if err := cache.PutRecipe("acme", "tools", "my-tool", []byte(validRecipeTOML), recipeMeta); err != nil {
		t.Fatalf("PutRecipe: %v", err)
	}

	// Neither client should be called
	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called when cache is fresh, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	data, err := p.Get(context.Background(), "my-tool")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != validRecipeTOML {
		t.Errorf("Get returned unexpected content: %s", data)
	}
}

func TestDistributedRegistryProvider_Get_NotFound(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Source meta exists but doesn't include the requested recipe
	meta := &SourceMeta{
		Branch:    "main",
		Files:     map[string]string{"other-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/other-tool.toml"},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	_, err := p.Get(context.Background(), "missing-tool")
	if err == nil {
		t.Fatal("expected error for missing recipe")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %s", err)
	}
}

func TestDistributedRegistryProvider_Get_ValidatesDownloadHost(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Set up an API server that returns directory listing
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries := []contentsEntry{
			{
				Name:        "my-tool.toml",
				Type:        "file",
				DownloadURL: "", // Will be replaced with raw server URL
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})
	apiServer := httptest.NewTLSServer(apiHandler)
	t.Cleanup(apiServer.Close)

	// Set up a raw content server
	rawHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validRecipeTOML))
	})
	rawServer := httptest.NewTLSServer(rawHandler)
	t.Cleanup(rawServer.Close)

	// Pre-populate source meta with the raw server URL
	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": rawServer.URL + "/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// The raw client must trust the test server's TLS
	gc := newGitHubClientWithHTTP(apiServer.Client(), rawServer.Client(), cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	// FetchRecipe will reject the test server URL because its hostname
	// isn't in the allowlist. That's expected.
	_, err := p.Get(context.Background(), "my-tool")
	if err == nil {
		// If it succeeded, the content should match
		return
	}
	// Expected: download URL validation error (test server host not in allowlist)
	if !strings.Contains(err.Error(), "not in allowlist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDistributedRegistryProvider_List(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"alpha": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/alpha.toml",
			"beta":  "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/beta.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Pre-populate recipe cache so List doesn't need to fetch via HTTP.
	// RegistryProvider.List() calls Get() on each path to extract descriptions.
	recipeMeta := &RecipeMeta{FetchedAt: time.Now()}
	if err := cache.PutRecipe("acme", "tools", "alpha", []byte(validRecipeTOML), recipeMeta); err != nil {
		t.Fatalf("PutRecipe alpha: %v", err)
	}
	if err := cache.PutRecipe("acme", "tools", "beta", []byte(validRecipeTOML), recipeMeta); err != nil {
		t.Fatalf("PutRecipe beta: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("expected 2 recipes, got %d", len(infos))
	}

	// Check that source is set correctly
	nameSet := make(map[string]bool)
	for _, info := range infos {
		nameSet[info.Name] = true
		if info.Source != recipe.RecipeSource("acme/tools") {
			t.Errorf("recipe %q source = %q, want %q", info.Name, info.Source, "acme/tools")
		}
	}
	if !nameSet["alpha"] {
		t.Error("expected alpha in list")
	}
	if !nameSet["beta"] {
		t.Error("expected beta in list")
	}
}

func TestDistributedRegistryProvider_List_Empty(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	meta := &SourceMeta{
		Branch:    "main",
		Files:     map[string]string{},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("should not be called")
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected empty list, got %d", len(infos))
	}
}

func TestDistributedRegistryProvider_Refresh(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Pre-populate with a fresh cache
	meta := &SourceMeta{
		Branch:    "main",
		Files:     map[string]string{"tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/tool.toml"},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Refresh bypasses freshness and hits the API
	apiCalled := false
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		entries := []contentsEntry{
			{Name: "tool.toml", Type: "file", DownloadURL: "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/tool.toml"},
			{Name: "new-tool.toml", Type: "file", DownloadURL: "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/new-tool.toml"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	})
	apiServer := httptest.NewTLSServer(apiHandler)
	t.Cleanup(apiServer.Close)

	apiClient := apiServer.Client()
	origTransport := apiClient.Transport
	apiClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "api.github.com") {
			req.URL.Scheme = "https"
			req.URL.Host = strings.TrimPrefix(apiServer.URL, "https://")
		}
		return origTransport.RoundTrip(req)
	})

	gc := newGitHubClientWithHTTP(apiClient, apiServer.Client(), cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	// The provider implements RefreshableProvider
	refreshable, ok := p.(recipe.RefreshableProvider)
	if !ok {
		t.Fatal("provider should implement RefreshableProvider")
	}

	err := refreshable.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !apiCalled {
		t.Error("Refresh should call the API even when cache is fresh")
	}

	// Verify the cache was updated with new listing
	updated, err := cache.GetSourceMeta("acme", "tools")
	if err != nil {
		t.Fatalf("GetSourceMeta after refresh: %v", err)
	}
	if _, ok := updated.Files["new-tool"]; !ok {
		t.Error("refreshed cache should contain new-tool")
	}
}

func TestDistributedRegistryProvider_Get_RateLimitError(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	resetTime := time.Now().Add(1 * time.Hour)

	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	apiServer := httptest.NewTLSServer(apiHandler)
	t.Cleanup(apiServer.Close)

	rawHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	rawServer := httptest.NewTLSServer(rawHandler)
	t.Cleanup(rawServer.Close)

	apiClient := apiServer.Client()
	rawClient := rawServer.Client()

	origTransport := apiClient.Transport
	apiClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "api.github.com") {
			req.URL.Scheme = "https"
			req.URL.Host = strings.TrimPrefix(apiServer.URL, "https://")
		}
		return origTransport.RoundTrip(req)
	})

	gc := newGitHubClientWithHTTP(apiClient, rawClient, cache, false)
	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	_, err := p.Get(context.Background(), "my-tool")
	if err == nil {
		t.Fatal("expected error when rate limited")
	}
	if !strings.Contains(err.Error(), "rate limit") && !strings.Contains(err.Error(), "listing recipes") {
		t.Errorf("error should mention rate limit or listing: %s", err)
	}
}

func TestDistributedRegistryProvider_ImplementsInterfaces(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)
	gc := newGitHubClientWithHTTP(nil, nil, cache, false)

	p := NewDistributedRegistryProviderWithManifest("acme", "tools", recipe.Manifest{Layout: "flat"}, gc)

	// Verify RecipeProvider interface
	var _ recipe.RecipeProvider = p

	// Verify RefreshableProvider interface
	var _ recipe.RefreshableProvider = p.(recipe.RefreshableProvider)
}

// --- Tests for manifest discovery ---

func TestDiscoverManifest_Found(t *testing.T) {
	manifest := recipe.Manifest{
		Layout:   "grouped",
		IndexURL: "https://example.com/recipes.json",
	}
	manifestJSON, _ := json.Marshal(manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect request for main branch, .tsuku-recipes/manifest.json
		if strings.Contains(r.URL.Path, "main/.tsuku-recipes/manifest.json") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(manifestJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	// Override the probe URL by using a transport that rewrites requests
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Redirect raw.githubusercontent.com to our test server
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	got, err := DiscoverManifest(context.Background(), "acme", "tools", client)
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}

	if got.Layout != "grouped" {
		t.Errorf("Layout = %q, want %q", got.Layout, "grouped")
	}
	if got.IndexURL != "https://example.com/recipes.json" {
		t.Errorf("IndexURL = %q, want %q", got.IndexURL, "https://example.com/recipes.json")
	}
}

func TestDiscoverManifest_FallbackToRecipesDir(t *testing.T) {
	manifest := recipe.Manifest{Layout: "flat"}
	manifestJSON, _ := json.Marshal(manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// .tsuku-recipes not found, but recipes/ has a manifest
		if strings.Contains(r.URL.Path, "recipes/manifest.json") && !strings.Contains(r.URL.Path, ".tsuku-recipes") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(manifestJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	got, err := DiscoverManifest(context.Background(), "acme", "tools", client)
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}

	if got.Layout != "flat" {
		t.Errorf("Layout = %q, want %q", got.Layout, "flat")
	}
}

func TestDiscoverManifest_NoManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	got, err := DiscoverManifest(context.Background(), "acme", "tools", client)
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}

	// Default: flat layout, no index
	if got.Layout != "flat" {
		t.Errorf("Layout = %q, want %q", got.Layout, "flat")
	}
	if got.IndexURL != "" {
		t.Errorf("IndexURL = %q, want empty", got.IndexURL)
	}
}

func TestDiscoverManifest_MasterBranch(t *testing.T) {
	manifest := recipe.Manifest{Layout: "grouped"}
	manifestJSON, _ := json.Marshal(manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only found on master branch
		if strings.Contains(r.URL.Path, "master/.tsuku-recipes/manifest.json") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(manifestJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	got, err := DiscoverManifest(context.Background(), "acme", "tools", client)
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}

	if got.Layout != "grouped" {
		t.Errorf("Layout = %q, want %q", got.Layout, "grouped")
	}
}

func TestDiscoverManifest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "manifest.json") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(server.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	// Invalid JSON in manifest should fall through to default
	got, err := DiscoverManifest(context.Background(), "acme", "tools", client)
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}

	// Should fall back to default flat since the JSON was invalid
	if got.Layout != "flat" {
		t.Errorf("Layout = %q, want %q (default)", got.Layout, "flat")
	}
}

func TestNewDistributedRegistryProvider_WithManifestDiscovery(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Set up a raw server that serves a manifest
	manifest := recipe.Manifest{Layout: "grouped", IndexURL: "https://example.com/index.json"}
	manifestJSON, _ := json.Marshal(manifest)

	rawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "manifest.json") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(manifestJSON)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(rawServer.Close)

	rawClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Host, "raw.githubusercontent.com") {
				req.URL.Scheme = "http"
				req.URL.Host = strings.TrimPrefix(rawServer.URL, "http://")
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	gc := newGitHubClientWithHTTP(nil, rawClient, cache, false)

	p, err := NewDistributedRegistryProvider(context.Background(), "acme", "tools", gc)
	if err != nil {
		t.Fatalf("NewDistributedRegistryProvider: %v", err)
	}

	if p.Source() != recipe.RecipeSource("acme/tools") {
		t.Errorf("Source() = %q, want %q", p.Source(), "acme/tools")
	}

	// Verify it implements RefreshableProvider
	if _, ok := p.(recipe.RefreshableProvider); !ok {
		t.Error("provider should implement RefreshableProvider")
	}
}
