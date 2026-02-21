package registry

import (
	"encoding/json"
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
