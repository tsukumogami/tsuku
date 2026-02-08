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

func TestClearCache_NoCacheDir(t *testing.T) {
	reg := &Registry{
		CacheDir: "",
	}

	err := reg.ClearCache()
	if err == nil {
		t.Error("ClearCache should fail when CacheDir is not set")
	}
}

func TestIsCached_EmptyName(t *testing.T) {
	reg := New(t.TempDir())

	// Empty name should return false
	if reg.IsCached("") {
		t.Error("IsCached should return false for empty name")
	}
}

func TestCacheRecipe_EmptyName(t *testing.T) {
	reg := New(t.TempDir())

	// Empty name should fail
	err := reg.CacheRecipe("", []byte("content"))
	if err == nil {
		t.Error("CacheRecipe should fail for empty name")
	}
}

func TestGetCached_EmptyName(t *testing.T) {
	reg := New(t.TempDir())

	// Empty name should return error
	data, err := reg.GetCached("")
	if err == nil {
		t.Error("GetCached should fail for empty name")
	}
	if data != nil {
		t.Error("GetCached should return nil data for empty name")
	}
}

func TestListCached(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Initially empty
	names, err := reg.ListCached()
	if err != nil {
		t.Fatalf("ListCached() failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListCached() returned %d names, want 0", len(names))
	}

	// Add some recipes
	_ = reg.CacheRecipe("tool-a", []byte("content a"))
	_ = reg.CacheRecipe("tool-b", []byte("content b"))

	// List again
	names, err = reg.ListCached()
	if err != nil {
		t.Fatalf("ListCached() failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("ListCached() returned %d names, want 2", len(names))
	}

	// Verify names
	foundA, foundB := false, false
	for _, n := range names {
		if n == "tool-a" {
			foundA = true
		}
		if n == "tool-b" {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Errorf("ListCached() returned %v, expected tool-a and tool-b", names)
	}
}

func TestListCached_EmptyCacheDir(t *testing.T) {
	reg := &Registry{CacheDir: ""}

	names, err := reg.ListCached()
	if err != nil {
		t.Fatalf("ListCached() failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListCached() should return empty for empty cache dir")
	}
}

func TestListCached_NonExistentDir(t *testing.T) {
	reg := New("/non/existent/path")

	names, err := reg.ListCached()
	if err != nil {
		t.Fatalf("ListCached() should not fail for non-existent dir: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListCached() should return empty for non-existent dir")
	}
}

func TestFetchDiscoveryEntry(t *testing.T) {
	entryJSON := `{"builder":"homebrew","source":"jq"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/discovery/j/jq/jq.json" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(entryJSON))
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

	ctx := context.Background()
	data, err := reg.FetchDiscoveryEntry(ctx, "j/jq/jq.json")
	if err != nil {
		t.Fatalf("FetchDiscoveryEntry failed: %v", err)
	}
	if string(data) != entryJSON {
		t.Errorf("content = %q, want %q", data, entryJSON)
	}

	// Verify cached locally
	cachePath := filepath.Join(cacheDir, "discovery", "j", "jq", "jq.json")
	cached, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if string(cached) != entryJSON {
		t.Errorf("cached = %q, want %q", cached, entryJSON)
	}
}

func TestFetchDiscoveryEntry_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	reg := &Registry{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		client:   &http.Client{},
	}

	_, err := reg.FetchDiscoveryEntry(context.Background(), "z/zz/zzz.json")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("Expected *RegistryError, got %T", err)
	}
	if regErr.Type != ErrTypeNotFound {
		t.Errorf("Error type = %v, want %v", regErr.Type, ErrTypeNotFound)
	}
}

func TestFetchDiscoveryEntry_NetworkError(t *testing.T) {
	reg := &Registry{
		BaseURL:  "http://localhost:1", // connection refused
		CacheDir: t.TempDir(),
		client:   &http.Client{},
	}

	_, err := reg.FetchDiscoveryEntry(context.Background(), "j/jq/jq.json")
	if err == nil {
		t.Fatal("Expected error for unreachable server")
	}
}

// TestRegistryHTTPClient_DisableCompression tests that registry HTTP client has compression disabled
func TestRegistryHTTPClient_DisableCompression(t *testing.T) {
	client := newRegistryHTTPClient()

	// Verify the transport has DisableCompression set
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport, got different type")
	}

	if !transport.DisableCompression {
		t.Error("Expected DisableCompression to be true, got false")
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"https://example.com", false},
		{"http://example.com", false},
		{"https://raw.githubusercontent.com/tsukumogami/tsuku/main", false},
		{"/path/to/registry", true},
		{"./relative/path", true},
		{"../parent/path", true},
		{"/home/user/recipes", true},
		{"", true}, // Empty is treated as local (will fail later)
	}

	for _, tc := range tests {
		got := isLocalPath(tc.path)
		if got != tc.expected {
			t.Errorf("isLocalPath(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

func TestLocalRegistryIsLocal(t *testing.T) {
	// Save original env
	original := os.Getenv(EnvRegistryURL)
	defer os.Setenv(EnvRegistryURL, original)

	// Test with local path
	os.Setenv(EnvRegistryURL, "/path/to/local/registry")
	reg := New("/tmp/cache")
	if !reg.IsLocal() {
		t.Error("Registry should be local when BaseURL is a local path")
	}

	// Test with HTTP URL
	os.Setenv(EnvRegistryURL, "https://example.com")
	reg = New("/tmp/cache")
	if reg.IsLocal() {
		t.Error("Registry should not be local when BaseURL is an HTTP URL")
	}

	// Test with default (unset)
	_ = os.Unsetenv(EnvRegistryURL)
	reg = New("/tmp/cache")
	if reg.IsLocal() {
		t.Error("Registry should not be local with default URL")
	}
}

func TestFetchRecipe_LocalRegistry(t *testing.T) {
	// Create a local registry structure
	localRegistry := t.TempDir()
	recipesDir := filepath.Join(localRegistry, "recipes", "t")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatalf("Failed to create recipes dir: %v", err)
	}

	recipeContent := `[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "test-tool --version"
`
	recipePath := filepath.Join(recipesDir, "test-tool.toml")
	if err := os.WriteFile(recipePath, []byte(recipeContent), 0644); err != nil {
		t.Fatalf("Failed to write recipe: %v", err)
	}

	// Create registry pointing to local directory
	reg := &Registry{
		BaseURL:  localRegistry,
		CacheDir: t.TempDir(),
		isLocal:  true,
	}

	// Test successful fetch
	ctx := context.Background()
	data, err := reg.FetchRecipe(ctx, "test-tool")
	if err != nil {
		t.Fatalf("FetchRecipe failed: %v", err)
	}
	if string(data) != recipeContent {
		t.Errorf("FetchRecipe returned unexpected content:\ngot: %s\nwant: %s", data, recipeContent)
	}

	// Test not found
	_, err = reg.FetchRecipe(ctx, "nonexistent")
	if err == nil {
		t.Error("FetchRecipe should fail for nonexistent recipe")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("Expected *RegistryError, got %T", err)
	}
	if regErr.Type != ErrTypeNotFound {
		t.Errorf("Error type = %v, want %v", regErr.Type, ErrTypeNotFound)
	}
}

func TestFetchDiscoveryEntry_LocalRegistry(t *testing.T) {
	// Create a local registry structure with discovery entries
	localRegistry := t.TempDir()
	discoveryDir := filepath.Join(localRegistry, "recipes", "discovery", "j", "jq")
	if err := os.MkdirAll(discoveryDir, 0755); err != nil {
		t.Fatalf("Failed to create discovery dir: %v", err)
	}

	entryContent := `{"builder":"homebrew","source":"jq"}`
	entryPath := filepath.Join(discoveryDir, "jq.json")
	if err := os.WriteFile(entryPath, []byte(entryContent), 0644); err != nil {
		t.Fatalf("Failed to write discovery entry: %v", err)
	}

	// Create registry pointing to local directory
	reg := &Registry{
		BaseURL:  localRegistry,
		CacheDir: t.TempDir(),
		isLocal:  true,
	}

	// Test successful fetch
	ctx := context.Background()
	data, err := reg.FetchDiscoveryEntry(ctx, "j/jq/jq.json")
	if err != nil {
		t.Fatalf("FetchDiscoveryEntry failed: %v", err)
	}
	if string(data) != entryContent {
		t.Errorf("FetchDiscoveryEntry returned %q, want %q", data, entryContent)
	}

	// Test not found
	_, err = reg.FetchDiscoveryEntry(ctx, "z/zz/nonexistent.json")
	if err == nil {
		t.Error("FetchDiscoveryEntry should fail for nonexistent entry")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("Expected *RegistryError, got %T", err)
	}
	if regErr.Type != ErrTypeNotFound {
		t.Errorf("Error type = %v, want %v", regErr.Type, ErrTypeNotFound)
	}
}

func TestFetchRecipe_EmptyLocalRegistry(t *testing.T) {
	// Create an empty local registry (no recipes directory)
	localRegistry := t.TempDir()

	reg := &Registry{
		BaseURL:  localRegistry,
		CacheDir: t.TempDir(),
		isLocal:  true,
	}

	// All fetches should return not found
	ctx := context.Background()
	_, err := reg.FetchRecipe(ctx, "any-tool")
	if err == nil {
		t.Error("FetchRecipe should fail when recipes directory doesn't exist")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("Expected *RegistryError, got %T", err)
	}
	if regErr.Type != ErrTypeNotFound {
		t.Errorf("Error type = %v, want %v", regErr.Type, ErrTypeNotFound)
	}
}
