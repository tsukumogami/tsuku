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
	loader := New(reg)

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
	loader := New(reg)

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
	loader := New(reg)

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
	loader := New(reg) // has embedded recipes

	// Force index build by looking up a name
	loader.satisfiesOnce.Do(loader.buildSatisfiesIndex)

	// The embedded openssl recipe should populate the index
	if canonicalName, ok := loader.satisfiesIndex["openssl@3"]; ok {
		if canonicalName != "openssl" {
			t.Errorf("expected openssl@3 -> openssl, got -> %s", canonicalName)
		}
	} else {
		t.Error("expected openssl@3 in satisfies index from embedded openssl recipe")
	}
}

func TestSatisfies_LookupKnownName(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := New(reg)

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
	loader := New(reg)

	_, ok := loader.lookupSatisfies("nonexistent-package@99")
	if ok {
		t.Error("expected lookupSatisfies to return false for unknown name")
	}
}

func TestSatisfies_PublicLookup(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := New(reg)

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

// Helper: create a loader with a test-only embedded-like setup using local recipes
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

	loader := NewWithoutEmbedded(reg, recipesDir)

	// Pre-populate the satisfies index manually since we're using NewWithoutEmbedded
	// (no embedded recipes means buildSatisfiesIndex won't find anything).
	// Trigger the Once first so it doesn't overwrite our manual index.
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]string{
			"test-alias@2":     "test-canonical",
			"test-other-alias": "test-canonical",
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

	loader := NewWithoutEmbedded(reg, recipesDir)

	// Set up satisfies index that also maps "exact-match" to something else
	// (this shouldn't happen in practice due to validation, but tests the priority)
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]string{
			"exact-match": "other-recipe",
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
	loader := New(reg) // includes embedded recipes

	// The embedded openssl recipe satisfies "openssl@3"
	// But openssl@3 also exists as a registry recipe (exact match).
	// Since we're using RequireEmbedded, the registry is skipped,
	// so satisfies fallback should find openssl via the embedded index.
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
	loader := NewWithoutEmbedded(reg, "")

	// Manually add a satisfies entry pointing to a non-embedded recipe
	loader.satisfiesOnce.Do(func() {
		loader.satisfiesIndex = map[string]string{
			"some-alias": "non-embedded-recipe",
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
		Verify:   VerifySection{Command: "test --version"},
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
	loader := New(reg)

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
	loader := New(reg)

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
