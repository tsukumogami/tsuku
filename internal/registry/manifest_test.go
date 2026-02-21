package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest_WithSatisfies(t *testing.T) {
	raw := `{
		"schema_version": "1.2.0",
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "sqlite",
				"description": "SQLite database engine",
				"homepage": "https://sqlite.org",
				"dependencies": [],
				"runtime_dependencies": [],
				"satisfies": {
					"homebrew": ["sqlite3"]
				}
			},
			{
				"name": "serve",
				"description": "HTTP file server",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": []
			}
		]
	}`

	manifest, err := parseManifest([]byte(raw))
	if err != nil {
		t.Fatalf("parseManifest() error: %v", err)
	}

	if manifest.SchemaVersion != "1.2.0" {
		t.Errorf("expected schema_version 1.2.0, got %s", manifest.SchemaVersion)
	}

	if len(manifest.Recipes) != 2 {
		t.Fatalf("expected 2 recipes, got %d", len(manifest.Recipes))
	}

	// First recipe has satisfies
	sqlite := manifest.Recipes[0]
	if sqlite.Name != "sqlite" {
		t.Errorf("expected recipe name 'sqlite', got %q", sqlite.Name)
	}
	if sqlite.Satisfies == nil {
		t.Fatal("expected sqlite.Satisfies to be non-nil")
	}
	homebrew, ok := sqlite.Satisfies["homebrew"]
	if !ok {
		t.Fatal("expected 'homebrew' key in Satisfies")
	}
	if len(homebrew) != 1 || homebrew[0] != "sqlite3" {
		t.Errorf("expected homebrew=[sqlite3], got %v", homebrew)
	}

	// Second recipe has no satisfies
	serve := manifest.Recipes[1]
	if serve.Name != "serve" {
		t.Errorf("expected recipe name 'serve', got %q", serve.Name)
	}
	if len(serve.Satisfies) != 0 {
		t.Errorf("expected empty Satisfies for serve, got %v", serve.Satisfies)
	}
}

func TestParseManifest_InvalidJSON(t *testing.T) {
	_, err := parseManifest([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetCachedManifest_NoCacheDir(t *testing.T) {
	reg := &Registry{CacheDir: ""}
	manifest, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() error: %v", err)
	}
	if manifest != nil {
		t.Error("expected nil manifest when CacheDir is empty")
	}
}

func TestGetCachedManifest_NoCachedFile(t *testing.T) {
	reg := &Registry{CacheDir: t.TempDir()}
	manifest, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() error: %v", err)
	}
	if manifest != nil {
		t.Error("expected nil manifest when no cached file exists")
	}
}

func TestGetCachedManifest_ValidCachedFile(t *testing.T) {
	cacheDir := t.TempDir()
	manifestData := Manifest{
		SchemaVersion: "1.2.0",
		Recipes: []ManifestRecipe{
			{
				Name:      "sqlite",
				Satisfies: map[string][]string{"homebrew": {"sqlite3"}},
			},
		},
	}
	data, err := json.Marshal(manifestData)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(cacheDir, manifestCacheFile), data, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	reg := &Registry{CacheDir: cacheDir}
	manifest, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() error: %v", err)
	}
	if manifest == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(manifest.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(manifest.Recipes))
	}
	if manifest.Recipes[0].Name != "sqlite" {
		t.Errorf("expected recipe name 'sqlite', got %q", manifest.Recipes[0].Name)
	}
}

func TestManifestRecipe_SatisfiesOmittedWhenEmpty(t *testing.T) {
	// Verify JSON serialization omits satisfies when empty
	recipe := ManifestRecipe{
		Name:        "test",
		Description: "Test",
	}

	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if _, ok := raw["satisfies"]; ok {
		t.Error("expected 'satisfies' to be omitted from JSON when empty")
	}
}

func TestFetchManifest_RemoteSuccess(t *testing.T) {
	manifestJSON := `{
		"schema_version": "1.2.0",
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "sqlite",
				"description": "SQLite database engine",
				"homepage": "https://sqlite.org",
				"dependencies": [],
				"runtime_dependencies": [],
				"satisfies": {"homebrew": ["sqlite3"]}
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, manifestJSON)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := &Registry{
		BaseURL:  "https://example.com",
		CacheDir: cacheDir,
		client:   server.Client(),
	}

	// Override manifest URL via env var
	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	manifest, err := reg.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest() error: %v", err)
	}

	if manifest.SchemaVersion != "1.2.0" {
		t.Errorf("expected schema_version 1.2.0, got %s", manifest.SchemaVersion)
	}
	if len(manifest.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(manifest.Recipes))
	}
	if manifest.Recipes[0].Name != "sqlite" {
		t.Errorf("expected recipe name 'sqlite', got %q", manifest.Recipes[0].Name)
	}

	// Verify it was cached
	cached, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() error after fetch: %v", err)
	}
	if cached == nil {
		t.Fatal("expected manifest to be cached after FetchManifest")
	}
	if cached.SchemaVersion != "1.2.0" {
		t.Errorf("cached manifest schema_version = %q, want 1.2.0", cached.SchemaVersion)
	}
}

func TestFetchManifest_RemoteServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := &Registry{
		BaseURL:  "https://example.com",
		CacheDir: t.TempDir(),
		client:   server.Client(),
	}
	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	_, err := reg.FetchManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestFetchManifest_RemoteInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not valid json")
	}))
	defer server.Close()

	reg := &Registry{
		BaseURL:  "https://example.com",
		CacheDir: t.TempDir(),
		client:   server.Client(),
	}
	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	_, err := reg.FetchManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}

	// Verify invalid data was not cached
	cached, cacheErr := reg.GetCachedManifest()
	if cacheErr != nil {
		t.Fatalf("GetCachedManifest() error: %v", cacheErr)
	}
	if cached != nil {
		t.Error("expected manifest NOT to be cached when response is invalid JSON")
	}
}

func TestFetchManifest_LocalRegistry(t *testing.T) {
	// Create a local registry structure with a manifest
	localDir := t.TempDir()
	siteDir := filepath.Join(localDir, "_site")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	manifestJSON := `{
		"schema_version": "1.2.0",
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "test-tool",
				"description": "A test tool",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": []
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(siteDir, "recipes.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cacheDir := t.TempDir()
	reg := &Registry{
		BaseURL:  localDir,
		CacheDir: cacheDir,
		isLocal:  true,
	}

	manifest, err := reg.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest() error for local registry: %v", err)
	}

	if len(manifest.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(manifest.Recipes))
	}
	if manifest.Recipes[0].Name != "test-tool" {
		t.Errorf("expected recipe name 'test-tool', got %q", manifest.Recipes[0].Name)
	}

	// Verify caching
	cached, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() error: %v", err)
	}
	if cached == nil {
		t.Fatal("expected manifest to be cached")
	}
}

func TestFetchManifest_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response -- context should cancel before this completes
		<-r.Context().Done()
	}))
	defer server.Close()

	reg := &Registry{
		BaseURL:  "https://example.com",
		CacheDir: t.TempDir(),
		client:   server.Client(),
	}
	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := reg.FetchManifest(ctx)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

func TestCacheManifest_NoCacheDir(t *testing.T) {
	reg := &Registry{CacheDir: ""}
	err := reg.CacheManifest([]byte("{}"))
	if err == nil {
		t.Error("expected error when CacheDir is empty")
	}
}

func TestCacheManifest_WritesFile(t *testing.T) {
	cacheDir := t.TempDir()
	reg := &Registry{CacheDir: cacheDir}

	data := []byte(`{"schema_version":"1.2.0","recipes":[]}`)
	if err := reg.CacheManifest(data); err != nil {
		t.Fatalf("CacheManifest() error: %v", err)
	}

	// Verify the file was written
	written, err := os.ReadFile(filepath.Join(cacheDir, manifestCacheFile))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(written) != string(data) {
		t.Errorf("cached manifest content mismatch:\ngot:  %s\nwant: %s", written, data)
	}
}

func TestManifestURL_EnvOverride(t *testing.T) {
	reg := &Registry{BaseURL: "https://example.com"}

	customURL := "https://custom.example.com/manifest.json"
	t.Setenv(EnvManifestURL, customURL)

	got := reg.manifestURL()
	if got != customURL {
		t.Errorf("manifestURL() = %q, want %q", got, customURL)
	}
}

func TestManifestURL_DefaultRemote(t *testing.T) {
	reg := &Registry{BaseURL: "https://example.com"}
	// Ensure env is not set
	t.Setenv(EnvManifestURL, "")

	got := reg.manifestURL()
	if got != DefaultManifestURL {
		t.Errorf("manifestURL() = %q, want %q", got, DefaultManifestURL)
	}
}

func TestManifestURL_LocalRegistry(t *testing.T) {
	reg := &Registry{BaseURL: "/tmp/test-registry", isLocal: true}
	// Ensure env is not set
	t.Setenv(EnvManifestURL, "")

	got := reg.manifestURL()
	want := filepath.Join("/tmp/test-registry", "_site", "recipes.json")
	if got != want {
		t.Errorf("manifestURL() = %q, want %q", got, want)
	}
}
