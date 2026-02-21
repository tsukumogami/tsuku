package registry

import (
	"context"
	"encoding/json"
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
				Name:      "libcurl",
				Satisfies: map[string][]string{"homebrew": {"curl"}},
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
	if manifest.Recipes[0].Name != "libcurl" {
		t.Errorf("expected recipe name 'libcurl', got %q", manifest.Recipes[0].Name)
	}
}

func TestFetchManifest_CachesLocally(t *testing.T) {
	manifestJSON := `{
		"schema_version": "1.2.0",
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "test-tool",
				"description": "A test tool",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": [],
				"satisfies": {"homebrew": ["test-tool@2"]}
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifestJSON))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := &Registry{
		CacheDir: cacheDir,
		client:   server.Client(),
	}

	// Override manifest URL for testing
	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	manifest, err := reg.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest() error: %v", err)
	}

	if manifest.SchemaVersion != "1.2.0" {
		t.Errorf("expected schema version 1.2.0, got %s", manifest.SchemaVersion)
	}
	if len(manifest.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(manifest.Recipes))
	}

	// Verify cache was written
	cached, err := os.ReadFile(filepath.Join(cacheDir, manifestCacheFile))
	if err != nil {
		t.Fatalf("expected cached manifest file, got error: %v", err)
	}
	if len(cached) == 0 {
		t.Error("expected non-empty cached manifest")
	}

	// Verify cached manifest is readable
	cachedManifest, err := reg.GetCachedManifest()
	if err != nil {
		t.Fatalf("GetCachedManifest() after fetch error: %v", err)
	}
	if cachedManifest == nil {
		t.Fatal("expected non-nil cached manifest after fetch")
	}
	if cachedManifest.Recipes[0].Name != "test-tool" {
		t.Errorf("expected cached recipe name 'test-tool', got %q", cachedManifest.Recipes[0].Name)
	}
}

func TestFetchManifest_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := &Registry{
		CacheDir: t.TempDir(),
		client:   server.Client(),
	}

	t.Setenv(EnvManifestURL, server.URL+"/recipes.json")

	_, err := reg.FetchManifest(context.Background())
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestFetchManifest_LocalFile(t *testing.T) {
	// Create a local manifest file
	manifestJSON := `{
		"schema_version": "1.2.0",
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "local-tool",
				"description": "Local tool",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": []
			}
		]
	}`

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "recipes.json")
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	reg := &Registry{CacheDir: t.TempDir()}

	t.Setenv(EnvManifestURL, manifestPath)

	manifest, err := reg.FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest() with local file error: %v", err)
	}
	if manifest.Recipes[0].Name != "local-tool" {
		t.Errorf("expected recipe name 'local-tool', got %q", manifest.Recipes[0].Name)
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
