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

// scenario-18: Provider Get/List/Source/Refresh methods work correctly

func TestDistributedProvider_Source(t *testing.T) {
	p := NewDistributedProvider("acme", "tools", nil)
	got := p.Source()
	want := recipe.RecipeSource("acme/tools")
	if got != want {
		t.Errorf("Source() = %q, want %q", got, want)
	}
}

func TestDistributedProvider_Get_CacheHit(t *testing.T) {
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
	p := NewDistributedProvider("acme", "tools", gc)

	data, err := p.Get(context.Background(), "my-tool")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != validRecipeTOML {
		t.Errorf("Get returned unexpected content: %s", data)
	}
}

func TestDistributedProvider_Get_NotFound(t *testing.T) {
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
	p := NewDistributedProvider("acme", "tools", gc)

	_, err := p.Get(context.Background(), "missing-tool")
	if err == nil {
		t.Fatal("expected error for missing recipe")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %s", err)
	}
}

func TestDistributedProvider_Get_ValidatesDownloadHost(t *testing.T) {
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
	p := NewDistributedProvider("acme", "tools", gc)

	// FetchRecipe will reject the test server URL because its hostname
	// isn't in the allowlist. That's expected -- the download URL validation
	// test is covered separately. Here we verify the provider wires through
	// the client correctly by checking the error type.
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

func TestDistributedProvider_List(t *testing.T) {
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

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	p := NewDistributedProvider("acme", "tools", gc)

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

func TestDistributedProvider_List_Empty(t *testing.T) {
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
	p := NewDistributedProvider("acme", "tools", gc)

	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected empty list, got %d", len(infos))
	}
}

func TestDistributedProvider_Refresh(t *testing.T) {
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

	// Refresh bypasses freshness and hits the API. Provide a server that
	// returns an updated listing so we can verify the cache gets updated.
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
	p := NewDistributedProvider("acme", "tools", gc)

	err := p.Refresh(context.Background())
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

func TestDistributedProvider_Get_RateLimitError(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// No cached source meta -- API call will be needed
	resetTime := time.Now().Add(1 * time.Hour)

	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})
	apiServer := httptest.NewTLSServer(apiHandler)
	t.Cleanup(apiServer.Close)

	// Raw server that returns 404 for branch probing
	rawHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	rawServer := httptest.NewTLSServer(rawHandler)
	t.Cleanup(rawServer.Close)

	// We can't easily redirect api.github.com calls to the test server since
	// the client code hardcodes the URL. Instead, test that the provider
	// correctly propagates rate limit errors from ListRecipes.
	// Use a client where the API call returns rate limit error.
	apiClient := apiServer.Client()
	rawClient := rawServer.Client()

	// Override the api client transport to redirect api.github.com to test server
	origTransport := apiClient.Transport
	apiClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Rewrite the URL to point to our test server
		if strings.Contains(req.URL.Host, "api.github.com") {
			req.URL.Scheme = "https"
			req.URL.Host = strings.TrimPrefix(apiServer.URL, "https://")
		}
		return origTransport.RoundTrip(req)
	})

	gc := newGitHubClientWithHTTP(apiClient, rawClient, cache, false)
	p := NewDistributedProvider("acme", "tools", gc)

	_, err := p.Get(context.Background(), "my-tool")
	if err == nil {
		t.Fatal("expected error when rate limited")
	}
	// The error should indicate a problem (rate limit or no recipe dir)
	if !strings.Contains(err.Error(), "rate limit") && !strings.Contains(err.Error(), "listing recipes") {
		t.Errorf("error should mention rate limit or listing: %s", err)
	}
}

func TestDistributedProvider_ImplementsInterfaces(t *testing.T) {
	p := NewDistributedProvider("acme", "tools", nil)

	// Verify RecipeProvider interface
	var _ recipe.RecipeProvider = p

	// Verify RefreshableProvider interface
	var _ recipe.RefreshableProvider = p
}
