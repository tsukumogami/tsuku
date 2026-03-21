package recipe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/registry"
)

// --- Schema Tests ---

func TestSatisfies_ParseFromTOML(t *testing.T) {
	recipeToml := `[metadata]
name = "test-lib"
type = "library"

[metadata.satisfies]
homebrew = ["test-lib@3", "test-lib@2"]
crates-io = ["libtest"]

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"
`
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	recipe, err := loader.parseBytes([]byte(recipeToml))
	if err != nil {
		t.Fatalf("parseBytes() failed: %v", err)
	}

	if recipe.Metadata.Satisfies == nil {
		t.Fatal("expected Satisfies to be non-nil")
	}

	homebrew, ok := recipe.Metadata.Satisfies["homebrew"]
	if !ok {
		t.Fatal("expected 'homebrew' key in Satisfies")
	}
	if len(homebrew) != 2 {
		t.Fatalf("expected 2 homebrew entries, got %d", len(homebrew))
	}
	if homebrew[0] != "test-lib@3" || homebrew[1] != "test-lib@2" {
		t.Errorf("unexpected homebrew entries: %v", homebrew)
	}

	cratesIO, ok := recipe.Metadata.Satisfies["crates-io"]
	if !ok {
		t.Fatal("expected 'crates-io' key in Satisfies")
	}
	if len(cratesIO) != 1 || cratesIO[0] != "libtest" {
		t.Errorf("unexpected crates-io entries: %v", cratesIO)
	}
}

func TestSatisfies_BackwardCompatible(t *testing.T) {
	// Recipe without satisfies field should parse and work unchanged
	recipeToml := `[metadata]
name = "no-satisfies"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "no-satisfies --version"
`
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	recipe, err := loader.parseBytes([]byte(recipeToml))
	if err != nil {
		t.Fatalf("parseBytes() failed: %v", err)
	}

	if len(recipe.Metadata.Satisfies) != 0 {
		t.Errorf("expected empty Satisfies for recipe without field, got %v", recipe.Metadata.Satisfies)
	}
}

func TestSatisfies_EmbeddedOpenSSL(t *testing.T) {
	// Verify the embedded openssl recipe has the satisfies field parsed correctly
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	recipe, err := loader.Get("openssl", LoaderOptions{RequireEmbedded: true})
	if err != nil {
		t.Fatalf("Get(openssl) failed: %v", err)
	}

	if recipe.Metadata.Satisfies == nil {
		t.Fatal("expected openssl.Satisfies to be non-nil")
	}

	homebrew, ok := recipe.Metadata.Satisfies["homebrew"]
	if !ok {
		t.Fatal("expected 'homebrew' key in openssl Satisfies")
	}

	found := false
	for _, name := range homebrew {
		if name == "openssl@3" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected openssl@3 in homebrew satisfies entries, got %v", homebrew)
	}
}

// --- Satisfies Index Tests ---

func TestSatisfies_BuildIndex(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Force index build by looking up a name
	loader.satisfiesOnce.Do(loader.buildSatisfiesIndex)

	// The embedded openssl recipe should populate the index
	if entry, ok := loader.satisfiesIndex["openssl@3"]; ok {
		if entry.recipeName != "openssl" {
			t.Errorf("expected openssl@3 -> openssl, got -> %s", entry.recipeName)
		}
	} else {
		t.Error("expected openssl@3 in satisfies index from embedded openssl recipe")
	}
}

func TestSatisfies_LookupKnownName(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	canonicalName, ok := loader.lookupSatisfies("openssl@3")
	if !ok {
		t.Fatal("expected lookupSatisfies to find openssl@3")
	}
	if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}
}

func TestSatisfies_LookupUnknownName(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	_, ok := loader.lookupSatisfies("nonexistent-package@99")
	if ok {
		t.Error("expected lookupSatisfies to return false for unknown name")
	}
}

func TestSatisfies_PublicLookup(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// LookupSatisfies is the public API wrapper
	canonicalName, ok := loader.LookupSatisfies("openssl@3")
	if !ok {
		t.Fatal("expected LookupSatisfies to find openssl@3")
	}
	if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}
}

// --- Loader Fallback Tests ---

// Helper: create a loader with a test-only setup using local recipes
// and a satisfies entry for a test recipe.
func setupSatisfiesTestLoader(t *testing.T) (*Loader, *httptest.Server) {
	t.Helper()

	// Create a local recipe that has a satisfies entry
	satisfyingRecipe := `[metadata]
name = "test-canonical"
type = "library"

[metadata.satisfies]
testeco = ["test-alias@2", "test-other-alias"]

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"
`

	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "test-canonical.toml"), []byte(satisfyingRecipe), 0644); err != nil {
		t.Fatalf("Failed to write test recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderNoEmbedded(reg, recipesDir)

	// Pre-populate the satisfies index manually since we have no embedded recipes.
	// Trigger the Once first so it doesn't overwrite our manual index.
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"test-alias@2":     {recipeName: "test-canonical", source: SourceLocal},
			"test-other-alias": {recipeName: "test-canonical", source: SourceLocal},
		}
	})

	return loader, server
}

func TestSatisfies_GetWithContext_FallbackToSatisfies(t *testing.T) {
	loader, server := setupSatisfiesTestLoader(t)
	defer server.Close()

	// Look up an alias that doesn't exist as a recipe name
	recipe, err := loader.GetWithContext(context.Background(), "test-alias@2", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithContext() failed for satisfies alias: %v", err)
	}

	if recipe.Metadata.Name != "test-canonical" {
		t.Errorf("expected recipe name 'test-canonical', got %q", recipe.Metadata.Name)
	}
}

func TestSatisfies_GetWithContext_ExactMatchTakesPriority(t *testing.T) {
	// Create two recipes: one exact match and one that satisfies the same name
	exactRecipe := `[metadata]
name = "exact-match"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "exact-match --version"
`

	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "exact-match.toml"), []byte(exactRecipe), 0644); err != nil {
		t.Fatalf("Failed to write exact recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderNoEmbedded(reg, recipesDir)

	// Set up satisfies index that also maps "exact-match" to something else
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"exact-match": {recipeName: "other-recipe", source: SourceLocal},
		}
	})

	// Exact match should win
	recipe, err := loader.Get("exact-match", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Name != "exact-match" {
		t.Errorf("expected exact match recipe, got %q", recipe.Metadata.Name)
	}
}

func TestSatisfies_GetEmbeddedOnly_Fallback(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// The embedded openssl recipe satisfies "openssl@3"
	recipe, err := loader.Get("openssl@3", LoaderOptions{RequireEmbedded: true})
	if err != nil {
		t.Fatalf("Get(openssl@3, RequireEmbedded) failed: %v", err)
	}

	if recipe.Metadata.Name != "openssl" {
		t.Errorf("expected recipe name 'openssl', got %q", recipe.Metadata.Name)
	}
}

func TestSatisfies_GetEmbeddedOnly_NonEmbeddedSatisfier(t *testing.T) {
	// A loader with no embedded recipes should not find anything via satisfies
	reg := registry.New(t.TempDir())
	loader := newTestLoaderNoEmbedded(reg, "")

	// Manually add a satisfies entry pointing to a non-embedded recipe
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"some-alias": {recipeName: "non-embedded-recipe", source: SourceRegistry},
		}
	})

	_, err := loader.Get("some-alias", LoaderOptions{RequireEmbedded: true})
	if err == nil {
		t.Error("expected error when satisfier is not embedded")
	}
}

// --- Validation Tests ---

func TestSatisfies_Validation_SelfReferential(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name: "mylib",
			Type: RecipeTypeLibrary,
			Satisfies: map[string][]string{
				"homebrew": {"mylib"}, // self-referential
			},
		},
		Steps: []Step{{Action: "download"}},
	}

	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if strings.Contains(err.Message, "self-referential") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected validation error for self-referential satisfies entry")
	}
}

func TestSatisfies_Validation_MalformedEcosystem(t *testing.T) {
	tests := []struct {
		name      string
		ecosystem string
		wantErr   bool
	}{
		{"valid lowercase", "homebrew", false},
		{"valid with hyphen", "crates-io", false},
		{"valid with number", "python3", false},
		{"uppercase", "Homebrew", true},
		{"space", "home brew", true},
		{"underscore", "crates_io", true},
		{"special chars", "brew!", true},
		{"starts with number", "3brew", true},
		{"starts with hyphen", "-brew", true},
		{"empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					Name: "test-lib",
					Type: RecipeTypeLibrary,
					Satisfies: map[string][]string{
						tc.ecosystem: {"some-pkg"},
					},
				},
				Steps: []Step{{Action: "download"}},
			}

			errs := ValidateStructural(r)

			foundEcoErr := false
			for _, err := range errs {
				if strings.Contains(err.Field, "metadata.satisfies") &&
					strings.Contains(err.Message, "ecosystem name") {
					foundEcoErr = true
					break
				}
			}

			if tc.wantErr && !foundEcoErr {
				t.Errorf("expected validation error for ecosystem %q", tc.ecosystem)
			}
			if !tc.wantErr && foundEcoErr {
				t.Errorf("unexpected validation error for ecosystem %q", tc.ecosystem)
			}
		})
	}
}

func TestSatisfies_Validation_EmptyPackageName(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name: "test-lib",
			Type: RecipeTypeLibrary,
			Satisfies: map[string][]string{
				"homebrew": {""},
			},
		},
		Steps: []Step{{Action: "download"}},
	}

	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if strings.Contains(err.Message, "must not be empty") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected validation error for empty package name")
	}
}

func TestSatisfies_Validation_ValidRecipe(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name: "test-lib",
			Type: RecipeTypeLibrary,
			Satisfies: map[string][]string{
				"homebrew":  {"test-lib@3"},
				"crates-io": {"libtest"},
			},
		},
		Steps: []Step{{Action: "download"}},
	}

	errs := ValidateStructural(r)

	// Filter for satisfies-related errors only
	for _, err := range errs {
		if strings.Contains(err.Field, "satisfies") {
			t.Errorf("unexpected satisfies validation error: %v", err)
		}
	}
}

func TestSatisfies_Validation_NoSatisfiesField(t *testing.T) {
	// Recipe without satisfies should pass validation
	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Action: "download"}},
		Verify:   &VerifySection{Command: "test --version"},
	}

	errs := ValidateStructural(r)

	for _, err := range errs {
		if strings.Contains(err.Field, "satisfies") {
			t.Errorf("unexpected satisfies validation error for recipe without satisfies: %v", err)
		}
	}
}

// --- Lazy Initialization Tests ---

func TestSatisfies_LazyBuild(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Before any lookup, satisfiesIndex should be nil
	if loader.satisfiesIndex != nil {
		t.Error("expected satisfiesIndex to be nil before first lookup")
	}

	// First lookup triggers build
	loader.lookupSatisfies("anything")

	// After lookup, index should be populated (even if empty for the query)
	if loader.satisfiesIndex == nil {
		t.Error("expected satisfiesIndex to be non-nil after first lookup")
	}
}

func TestSatisfies_ClearCacheResetsIndex(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Build the index
	loader.lookupSatisfies("anything")
	if loader.satisfiesIndex == nil {
		t.Fatal("expected satisfiesIndex to be built")
	}

	// Clear cache
	loader.ClearCache()

	// Index should be reset
	if loader.satisfiesIndex != nil {
		t.Error("expected satisfiesIndex to be nil after ClearCache")
	}
}

// --- Cross-Recipe Cycle Tests ---

func TestSatisfies_GetWithContext_NoCrossRecipeCycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderNoEmbedded(reg, "")

	// Create a cycle: alias-a -> recipe-b -> alias-a
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"alias-a":  {recipeName: "recipe-b", source: SourceRegistry},
			"recipe-b": {recipeName: "alias-a", source: SourceRegistry},
		}
	})

	// This should NOT hang or stack-overflow.
	_, err := loader.GetWithContext(context.Background(), "alias-a", LoaderOptions{})
	if err == nil {
		t.Fatal("expected error for unresolvable satisfies cycle, got nil")
	}

	// Verify the error mentions the canonical name that wasn't found
	if !strings.Contains(err.Error(), "recipe-b") {
		t.Errorf("expected error to mention 'recipe-b', got: %v", err)
	}
}

func TestSatisfies_GetEmbeddedOnly_NoCrossRecipeCycle(t *testing.T) {
	// Same cycle test but for the embedded-only path.
	reg := registry.New(t.TempDir())
	loader := newTestLoaderNoEmbedded(reg, "")

	// Create a cycle in the satisfies index
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"alias-x":  {recipeName: "recipe-y", source: SourceEmbedded},
			"recipe-y": {recipeName: "alias-x", source: SourceEmbedded},
		}
	})

	_, err := loader.Get("alias-x", LoaderOptions{RequireEmbedded: true})
	if err == nil {
		t.Fatal("expected error for unresolvable satisfies cycle in embedded mode")
	}
}

// --- Registry Manifest Integration Tests ---

func TestSatisfies_BuildIndex_IncludesManifestData(t *testing.T) {
	cacheDir := t.TempDir()
	manifestJSON := `{
		"schema_version": 1,
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
				"name": "libcurl",
				"description": "curl library",
				"homepage": "https://curl.se",
				"dependencies": [],
				"runtime_dependencies": []
			},
			{
				"name": "serve",
				"description": "HTTP server",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": []
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(cacheDir, "manifest.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	reg := registry.New(cacheDir)
	loader := newTestLoaderNoEmbedded(reg, "")

	// Trigger index build
	loader.satisfiesOnce.Do(loader.buildSatisfiesIndex)

	// Manifest entries should be in the index
	if entry, ok := loader.satisfiesIndex["sqlite3"]; ok {
		if entry.recipeName != "sqlite" {
			t.Errorf("expected sqlite3 -> sqlite, got -> %s", entry.recipeName)
		}
	} else {
		t.Error("expected sqlite3 in satisfies index from manifest")
	}

	// libcurl should NOT have any satisfies entries in the index
	if entry, ok := loader.satisfiesIndex["curl"]; ok {
		t.Errorf("unexpected satisfies index entry: curl -> %s (curl is a canonical recipe name, not a satisfies alias)", entry.recipeName)
	}
}

func TestSatisfies_BuildIndex_EmbeddedOverManifest(t *testing.T) {
	cacheDir := t.TempDir()
	manifestJSON := `{
		"schema_version": 1,
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "other-openssl",
				"description": "Alternative openssl",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": [],
				"satisfies": {
					"homebrew": ["openssl@3"]
				}
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(cacheDir, "manifest.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	reg := registry.New(cacheDir)
	loader := newTestLoaderWithRegistry(reg) // Includes embedded recipes (openssl claims openssl@3)

	// Trigger index build
	loader.satisfiesOnce.Do(loader.buildSatisfiesIndex)

	// Embedded openssl should win over manifest's other-openssl
	entry, ok := loader.satisfiesIndex["openssl@3"]
	if !ok {
		t.Fatal("expected openssl@3 in satisfies index")
	}
	if entry.recipeName != "openssl" {
		t.Errorf("expected embedded 'openssl' to win over manifest, got %q", entry.recipeName)
	}
}

func TestSatisfies_BuildIndex_NoManifest(t *testing.T) {
	// When no manifest is cached, the index should still work with embedded data only
	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	loader := newTestLoaderWithRegistry(reg)

	// Trigger index build (no manifest file exists)
	loader.satisfiesOnce.Do(loader.buildSatisfiesIndex)

	// Embedded openssl@3 should still be in the index
	entry, ok := loader.satisfiesIndex["openssl@3"]
	if !ok {
		t.Fatal("expected openssl@3 in satisfies index from embedded recipes")
	}
	if entry.recipeName != "openssl" {
		t.Errorf("expected openssl@3 -> openssl, got -> %s", entry.recipeName)
	}
}

func TestSatisfies_ManifestRecipeResolvable(t *testing.T) {
	// Create a local recipe that the manifest claims satisfies "test-alias@2"
	localRecipe := `[metadata]
name = "test-registry-recipe"
type = "library"

[metadata.satisfies]
testeco = ["test-alias@2"]

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"
`
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "test-registry-recipe.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write recipe: %v", err)
	}

	// Create a cached manifest declaring the same satisfies mapping
	cacheDir := t.TempDir()
	manifestJSON := `{
		"schema_version": 1,
		"generated_at": "2026-01-01T00:00:00Z",
		"recipes": [
			{
				"name": "test-registry-recipe",
				"description": "Test registry recipe",
				"homepage": "https://example.com",
				"dependencies": [],
				"runtime_dependencies": [],
				"satisfies": {
					"testeco": ["test-alias@2"]
				}
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(cacheDir, "manifest.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL
	loader := newTestLoaderNoEmbedded(reg, recipesDir)

	// Look up the alias through the satisfies fallback
	recipe, err := loader.GetWithContext(context.Background(), "test-alias@2", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithContext() failed for manifest satisfies alias: %v", err)
	}

	if recipe.Metadata.Name != "test-registry-recipe" {
		t.Errorf("expected recipe name 'test-registry-recipe', got %q", recipe.Metadata.Name)
	}
}

func TestSatisfies_LoadDirect_SkipsSatisfiesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderNoEmbedded(reg, "")

	// "phantom" exists in the satisfies index but not as a real recipe
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]satisfiesEntry{
			"phantom": {recipeName: "also-phantom", source: SourceRegistry},
		}
	})

	// resolveFromChain with trySatisfies=false should NOT follow the satisfies index
	_, _, err := loader.resolveFromChain(context.Background(), loader.providers, "phantom", false)
	if err == nil {
		t.Fatal("expected resolveFromChain to return error for non-existent recipe")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "404") {
		t.Errorf("expected error about not found, got: %v", err)
	}
}
