package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeURL(t *testing.T) {
	r := &Registry{BaseURL: "https://example.com/registry"}

	tests := []struct {
		name     string
		expected string
	}{
		{"actionlint", "https://example.com/registry/recipes/a/actionlint.toml"},
		{"golang", "https://example.com/registry/recipes/g/golang.toml"},
		{"kubectl", "https://example.com/registry/recipes/k/kubectl.toml"},
		{"", ""},
	}

	for _, tc := range tests {
		got := r.recipeURL(tc.name)
		if got != tc.expected {
			t.Errorf("recipeURL(%q) = %q, want %q", tc.name, got, tc.expected)
		}
	}
}

func TestCachePath(t *testing.T) {
	r := &Registry{CacheDir: "/tmp/test-cache"}

	tests := []struct {
		name     string
		expected string
	}{
		{"actionlint", "/tmp/test-cache/a/actionlint.toml"},
		{"golang", "/tmp/test-cache/g/golang.toml"},
		{"", ""},
	}

	for _, tc := range tests {
		got := r.cachePath(tc.name)
		if got != tc.expected {
			t.Errorf("cachePath(%q) = %q, want %q", tc.name, got, tc.expected)
		}
	}
}

func TestFetchRecipe(t *testing.T) {
	// Create a mock server
	mockRecipe := `[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "test-tool --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/t/test-tool.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockRecipe))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := &Registry{
		BaseURL:  server.URL,
		CacheDir: cacheDir,
		client:   &http.Client{},
	}

	// Test successful fetch
	ctx := context.Background()
	data, err := reg.FetchRecipe(ctx, "test-tool")
	if err != nil {
		t.Fatalf("FetchRecipe failed: %v", err)
	}
	if string(data) != mockRecipe {
		t.Errorf("FetchRecipe returned unexpected content")
	}

	// Test not found
	_, err = reg.FetchRecipe(ctx, "nonexistent")
	if err == nil {
		t.Error("FetchRecipe should fail for nonexistent recipe")
	}
}

func TestCacheOperations(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	testData := []byte("test recipe content")

	// Test caching
	err := reg.CacheRecipe("test-recipe", testData)
	if err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Verify file was created
	expectedPath := filepath.Join(cacheDir, "t", "test-recipe.toml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Error("Cache file was not created")
	}

	// Test retrieving cached recipe
	cached, err := reg.GetCached("test-recipe")
	if err != nil {
		t.Fatalf("GetCached failed: %v", err)
	}
	if string(cached) != string(testData) {
		t.Errorf("GetCached returned %q, want %q", cached, testData)
	}

	// Test IsCached
	if !reg.IsCached("test-recipe") {
		t.Error("IsCached should return true for cached recipe")
	}
	if reg.IsCached("not-cached") {
		t.Error("IsCached should return false for non-cached recipe")
	}

	// Test getting non-cached recipe
	notCached, err := reg.GetCached("not-cached")
	if err != nil {
		t.Fatalf("GetCached failed for non-cached: %v", err)
	}
	if notCached != nil {
		t.Error("GetCached should return nil for non-cached recipe")
	}
}

func TestClearCache(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Add some cached recipes
	_ = reg.CacheRecipe("recipe-a", []byte("content a"))
	_ = reg.CacheRecipe("recipe-b", []byte("content b"))

	// Verify they exist
	if !reg.IsCached("recipe-a") || !reg.IsCached("recipe-b") {
		t.Fatal("Recipes should be cached")
	}

	// Clear cache
	err := reg.ClearCache()
	if err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}

	// Verify they're gone
	if reg.IsCached("recipe-a") || reg.IsCached("recipe-b") {
		t.Error("Cache should be empty after ClearCache")
	}

	// Verify cache directory still exists (was recreated)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Cache directory should still exist after ClearCache")
	}
}

func TestEnvironmentVariableOverride(t *testing.T) {
	// Save original env
	original := os.Getenv(EnvRegistryURL)
	defer os.Setenv(EnvRegistryURL, original)

	// Test with custom URL
	customURL := "https://custom-registry.example.com"
	os.Setenv(EnvRegistryURL, customURL)

	reg := New("/tmp/test-cache")
	if reg.BaseURL != customURL {
		t.Errorf("Registry BaseURL = %q, want %q", reg.BaseURL, customURL)
	}

	// Test with default (unset env)
	_ = os.Unsetenv(EnvRegistryURL)
	reg = New("/tmp/test-cache")
	if reg.BaseURL != DefaultRegistryURL {
		t.Errorf("Registry BaseURL = %q, want %q", reg.BaseURL, DefaultRegistryURL)
	}
}

func TestFetchRecipeContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	reg := &Registry{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		client:   &http.Client{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := reg.FetchRecipe(ctx, "test")
	if err == nil {
		t.Error("FetchRecipe should fail with canceled context")
	}
}
