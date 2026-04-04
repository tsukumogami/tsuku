package recipe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/registry"
)

// Test helper: creates a loader with embedded + central registry providers (no local).
// Equivalent to the old New(reg).
func newTestLoaderWithRegistry(reg *registry.Registry) *Loader {
	embedded, _ := NewEmbeddedRegistry()
	var providers []RecipeProvider
	if ep := NewEmbeddedProvider(embedded); ep != nil {
		providers = append(providers, ep)
	}
	providers = append(providers, NewCentralRegistryProvider(reg))
	return NewLoader(providers...)
}

// Test helper: creates a loader with local + embedded + central registry providers.
// Equivalent to the old NewWithLocalRecipes(reg, dir).
func newTestLoaderWithLocal(reg *registry.Registry, recipesDir string) *Loader {
	embedded, _ := NewEmbeddedRegistry()
	var providers []RecipeProvider
	if recipesDir != "" {
		providers = append(providers, NewLocalProvider(recipesDir))
	}
	if ep := NewEmbeddedProvider(embedded); ep != nil {
		providers = append(providers, ep)
	}
	providers = append(providers, NewCentralRegistryProvider(reg))
	return NewLoader(providers...)
}

// Test helper: creates a loader with local + central registry providers (no embedded).
// Equivalent to the old NewWithoutEmbedded(reg, dir).
func newTestLoaderNoEmbedded(reg *registry.Registry, recipesDir string) *Loader {
	var providers []RecipeProvider
	if recipesDir != "" {
		providers = append(providers, NewLocalProvider(recipesDir))
	}
	providers = append(providers, NewCentralRegistryProvider(reg))
	return NewLoader(providers...)
}

func TestNewLoader(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	if loader == nil {
		t.Fatal("NewLoader() returned nil")
		return
	}
	if len(loader.providers) == 0 {
		t.Error("loader.providers should not be empty")
	}
	if loader.recipes == nil {
		t.Error("loader.recipes map not initialized")
	}
}

func TestLoader_Get_FromRegistry(t *testing.T) {
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
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	recipe, err := loader.Get("test-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Name != "test-tool" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "test-tool")
	}

	// Second call should use in-memory cache
	recipe2, err := loader.Get("test-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() second call failed: %v", err)
	}
	if recipe2 != recipe {
		t.Error("Expected same recipe instance from cache")
	}
}

func TestLoader_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	_, err := loader.Get("nonexistent", LoaderOptions{})
	if err == nil {
		t.Error("Get() should fail for nonexistent recipe")
	}
}

func TestLoader_GetWithContext(t *testing.T) {
	mockRecipe := `[metadata]
name = "ctx-tool"
description = "A tool for context testing"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "ctx-tool --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/c/ctx-tool.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockRecipe))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	ctx := context.Background()
	recipe, err := loader.GetWithContext(ctx, "ctx-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithContext() failed: %v", err)
	}

	if recipe.Metadata.Name != "ctx-tool" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "ctx-tool")
	}
}

func TestLoader_List(t *testing.T) {
	mockRecipe := `[metadata]
name = "list-tool"
description = "A tool for list testing"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "list-tool --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRecipe))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	// Initially empty
	names := loader.List()
	if len(names) != 0 {
		t.Errorf("List() returned %d items, want 0", len(names))
	}

	// Load a recipe
	_, _ = loader.Get("list-tool", LoaderOptions{})

	// Now should have one
	names = loader.List()
	if len(names) != 1 {
		t.Errorf("List() returned %d items, want 1", len(names))
	}
}

func TestLoader_Count(t *testing.T) {
	mockRecipe := `[metadata]
name = "count-tool"
description = "A tool for count testing"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "count-tool --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRecipe))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	if loader.Count() != 0 {
		t.Errorf("Count() = %d, want 0", loader.Count())
	}

	_, _ = loader.Get("count-tool", LoaderOptions{})

	if loader.Count() != 1 {
		t.Errorf("Count() = %d, want 1", loader.Count())
	}
}

func TestLoader_ProviderBySource(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	p := loader.ProviderBySource(SourceRegistry)
	if p == nil {
		t.Fatal("ProviderBySource(SourceRegistry) returned nil")
	}
	rp, ok := p.(*RegistryProvider)
	if !ok {
		t.Fatal("Expected RegistryProvider type")
	}
	if rp.Registry() != reg {
		t.Error("Registry() did not return the expected registry")
	}
}

func TestLoader_ClearCache(t *testing.T) {
	mockRecipe := `[metadata]
name = "cache-tool"
description = "A tool for cache testing"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "cache-tool --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockRecipe))
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithRegistry(reg)

	// Load a recipe
	_, _ = loader.Get("cache-tool", LoaderOptions{})
	if loader.Count() != 1 {
		t.Fatalf("Expected 1 recipe after Get, got %d", loader.Count())
	}

	// Clear cache
	loader.ClearCache()

	if loader.Count() != 0 {
		t.Errorf("Count() = %d after ClearCache, want 0", loader.Count())
	}
}

func TestLoader_parseBytes_Invalid(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Invalid TOML
	_, err := loader.parseBytes([]byte("invalid toml [[["))
	if err == nil {
		t.Error("parseBytes should fail for invalid TOML")
	}

	// Valid TOML but missing required fields
	_, err = loader.parseBytes([]byte(`[metadata]
name = ""
`))
	if err == nil {
		t.Error("parseBytes should fail for invalid recipe (empty name)")
	}
}

func TestNewLoader_WithLocalRecipes(t *testing.T) {
	reg := registry.New(t.TempDir())
	recipesDir := "/some/recipes/dir"
	loader := newTestLoaderWithLocal(reg, recipesDir)

	if loader == nil {
		t.Fatal("NewLoader() returned nil")
	}
	if loader.RecipesDir() != recipesDir {
		t.Errorf("RecipesDir() = %q, want %q", loader.RecipesDir(), recipesDir)
	}
}

func TestLoader_SetRecipesDir(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	if loader.RecipesDir() != "" {
		t.Errorf("initial RecipesDir() should be empty, got %q", loader.RecipesDir())
	}

	loader.SetRecipesDir("/test/dir")
	if loader.RecipesDir() != "/test/dir" {
		t.Errorf("RecipesDir() = %q, want %q", loader.RecipesDir(), "/test/dir")
	}
}

func TestLoader_Get_LocalRecipe(t *testing.T) {
	localRecipe := `[metadata]
name = "local-tool"
description = "A local test tool"

[[steps]]
action = "download"
url = "https://example.com/local.tar.gz"

[verify]
command = "local-tool --version"
`

	// Create a temporary local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "local-tool.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	// Set up a registry server that returns 404 for everything
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)

	// Should load from local recipes directory
	recipe, err := loader.Get("local-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Name != "local-tool" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "local-tool")
	}

	if recipe.Metadata.Description != "A local test tool" {
		t.Errorf("recipe.Metadata.Description = %q, want %q", recipe.Metadata.Description, "A local test tool")
	}
}

func TestLoader_Get_LocalRecipePriority(t *testing.T) {
	localRecipe := `[metadata]
name = "priority-tool"
description = "Local version"

[[steps]]
action = "download"
url = "https://example.com/local.tar.gz"

[verify]
command = "priority-tool --version"
`

	registryRecipe := `[metadata]
name = "priority-tool"
description = "Registry version"

[[steps]]
action = "download"
url = "https://example.com/registry.tar.gz"

[verify]
command = "priority-tool --version"
`

	// Create a temporary local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "priority-tool.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	// Set up a registry server that returns the registry recipe
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/p/priority-tool.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(registryRecipe))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)

	// Should load from local recipes directory, not registry
	recipe, err := loader.Get("priority-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Description != "Local version" {
		t.Errorf("recipe.Metadata.Description = %q, want %q (local should take priority)", recipe.Metadata.Description, "Local version")
	}
}

func TestLoader_Get_FallbackToRegistry(t *testing.T) {
	registryRecipe := `[metadata]
name = "registry-tool"
description = "Registry version"

[[steps]]
action = "download"
url = "https://example.com/registry.tar.gz"

[verify]
command = "registry-tool --version"
`

	// Create an empty local recipes directory
	recipesDir := t.TempDir()

	// Set up a registry server that returns a recipe
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/r/registry-tool.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(registryRecipe))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)

	// Should fall back to registry when local doesn't exist
	recipe, err := loader.Get("registry-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Description != "Registry version" {
		t.Errorf("recipe.Metadata.Description = %q, want %q", recipe.Metadata.Description, "Registry version")
	}
}

func TestLoader_Get_LocalRecipeParseError(t *testing.T) {
	// Create a local recipe with invalid TOML
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "invalid-tool.toml"), []byte("invalid toml [[["), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	// Set up a registry server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)

	// Should return parse error, not fallback to registry
	_, err := loader.Get("invalid-tool", LoaderOptions{})
	if err == nil {
		t.Error("Get() should fail for invalid local recipe TOML")
	}
}

func TestLoader_ListAllWithSource(t *testing.T) {
	localRecipe := `[metadata]
name = "local-tool"
description = "A local tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "local-tool --version"
`

	registryRecipe := `[metadata]
name = "registry-tool"
description = "A registry tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "registry-tool --version"
`

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "local-tool.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	// Create registry cache directory with letter subdirectory
	cacheDir := t.TempDir()
	letterDir := filepath.Join(cacheDir, "r")
	if err := os.MkdirAll(letterDir, 0755); err != nil {
		t.Fatalf("Failed to create registry letter directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(letterDir, "registry-tool.toml"), []byte(registryRecipe), 0644); err != nil {
		t.Fatalf("Failed to write registry recipe: %v", err)
	}

	reg := registry.New(cacheDir)
	loader := newTestLoaderNoEmbedded(reg, recipesDir)

	recipes, listErrs := loader.ListAllWithSource()
	if len(listErrs) > 0 {
		t.Fatalf("ListAllWithSource() had errors: %v", listErrs)
	}

	if len(recipes) != 2 {
		t.Errorf("ListAllWithSource() returned %d recipes, want 2", len(recipes))
	}

	// Check that we have both sources
	var hasLocal, hasRegistry bool
	for _, r := range recipes {
		if r.Source == SourceLocal && r.Name == "local-tool" {
			hasLocal = true
		}
		if r.Source == SourceRegistry && r.Name == "registry-tool" {
			hasRegistry = true
		}
	}

	if !hasLocal {
		t.Error("Expected local recipe in results")
	}
	if !hasRegistry {
		t.Error("Expected registry recipe in results")
	}
}

func TestLoader_ListAllWithSource_LocalShadowsRegistry(t *testing.T) {
	localRecipe := `[metadata]
name = "shared-tool"
description = "Local version"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "shared-tool --version"
`

	registryRecipe := `[metadata]
name = "shared-tool"
description = "Registry version"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "shared-tool --version"
`

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "shared-tool.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	// Create registry cache directory
	cacheDir := t.TempDir()
	letterDir := filepath.Join(cacheDir, "s")
	if err := os.MkdirAll(letterDir, 0755); err != nil {
		t.Fatalf("Failed to create registry letter directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(letterDir, "shared-tool.toml"), []byte(registryRecipe), 0644); err != nil {
		t.Fatalf("Failed to write registry recipe: %v", err)
	}

	reg := registry.New(cacheDir)
	loader := newTestLoaderNoEmbedded(reg, recipesDir)

	recipes, listErrs := loader.ListAllWithSource()
	if len(listErrs) > 0 {
		t.Fatalf("ListAllWithSource() had errors: %v", listErrs)
	}

	// Should only have one recipe (local shadows registry)
	if len(recipes) != 1 {
		t.Errorf("ListAllWithSource() returned %d recipes, want 1", len(recipes))
	}

	if len(recipes) > 0 && recipes[0].Source != SourceLocal {
		t.Errorf("Expected local source, got %q", recipes[0].Source)
	}
}

func TestLoader_ListLocal(t *testing.T) {
	localRecipe := `[metadata]
name = "only-local"
description = "A local only tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "only-local --version"
`

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "only-local.toml"), []byte(localRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	loader := newTestLoaderWithLocal(reg, recipesDir)

	recipes, err := loader.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal() failed: %v", err)
	}

	if len(recipes) != 1 {
		t.Errorf("ListLocal() returned %d recipes, want 1", len(recipes))
	}

	if len(recipes) > 0 {
		if recipes[0].Name != "only-local" {
			t.Errorf("Expected recipe name 'only-local', got %q", recipes[0].Name)
		}
		if recipes[0].Source != SourceLocal {
			t.Errorf("Expected local source, got %q", recipes[0].Source)
		}
	}
}

func TestLoader_ListLocal_EmptyDirectory(t *testing.T) {
	recipesDir := t.TempDir()
	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	loader := newTestLoaderWithLocal(reg, recipesDir)

	recipes, err := loader.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal() failed: %v", err)
	}

	if len(recipes) != 0 {
		t.Errorf("ListLocal() returned %d recipes, want 0", len(recipes))
	}
}

func TestLoader_ListLocal_NoRecipesDir(t *testing.T) {
	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	loader := newTestLoaderWithRegistry(reg) // No local provider

	recipes, err := loader.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal() failed: %v", err)
	}

	if len(recipes) != 0 {
		t.Errorf("ListLocal() returned %d recipes, want 0", len(recipes))
	}
}

func TestLoader_RecipesDir(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithLocal(reg, "/some/path")

	if loader.RecipesDir() != "/some/path" {
		t.Errorf("RecipesDir() = %q, want %q", loader.RecipesDir(), "/some/path")
	}
}

func TestIsTomlFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"test.toml", true},
		{"recipe.toml", true},
		{".toml", false}, // too short
		{"toml", false},
		{"test.txt", false},
		{"test.TOML", false}, // case sensitive
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isTomlFile(tc.name)
			if got != tc.want {
				t.Errorf("isTomlFile(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestTrimTomlExtension(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"test.toml", "test"},
		{"recipe.toml", "recipe"},
		{"test.txt", "test.txt"}, // not .toml, unchanged
		{"test", "test"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := trimTomlExtension(tc.name)
			if got != tc.want {
				t.Errorf("trimTomlExtension(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	validRecipe := `[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "test-tool --version"
`

	t.Run("valid recipe file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.toml")
		if err := os.WriteFile(path, []byte(validRecipe), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		recipe, err := ParseFile(path)
		if err != nil {
			t.Fatalf("ParseFile() failed: %v", err)
		}

		if recipe.Metadata.Name != "test-tool" {
			t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "test-tool")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := ParseFile("/nonexistent/path/recipe.toml")
		if err == nil {
			t.Error("ParseFile() should fail for nonexistent file")
		}
	})

	t.Run("invalid TOML", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "invalid.toml")
		if err := os.WriteFile(path, []byte("invalid toml [[["), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		_, err := ParseFile(path)
		if err == nil {
			t.Error("ParseFile() should fail for invalid TOML")
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "invalid-recipe.toml")
		invalidRecipe := `[metadata]
name = ""
`
		if err := os.WriteFile(path, []byte(invalidRecipe), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		_, err := ParseFile(path)
		if err == nil {
			t.Error("ParseFile() should fail for recipe with missing required fields")
		}
	})
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		recipe  Recipe
		wantErr bool
	}{
		{
			name: "valid recipe",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: "download"}},
				Verify:   &VerifySection{Command: "test --version"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			recipe: Recipe{
				Metadata: MetadataSection{Name: ""},
				Steps:    []Step{{Action: "download"}},
				Verify:   &VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing steps",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{},
				Verify:   &VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing action in step",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: ""}},
				Verify:   &VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing verify command",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: "download"}},
				Verify:   &VerifySection{Command: ""},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(&tc.recipe)
			if (err != nil) != tc.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoader_SetConstraintLookup(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Initially nil
	if loader.constraintLookup != nil {
		t.Error("Expected constraintLookup to be nil initially")
	}

	// Set a mock lookup
	called := false
	mockLookup := func(actionName string) (*Constraint, bool) {
		called = true
		return nil, true
	}
	loader.SetConstraintLookup(mockLookup)

	if loader.constraintLookup == nil {
		t.Error("Expected constraintLookup to be set")
	}

	// Verify it's the function we set
	loader.constraintLookup("test")
	if !called {
		t.Error("Expected mock lookup to be called")
	}
}

func TestLoader_StepAnalysisComputation(t *testing.T) {
	// Recipe with apt_install (has implicit constraint)
	recipeWithConstraint := `[metadata]
name = "constrained-tool"
description = "A tool with constrained step"

[[steps]]
action = "apt_install"
packages = ["curl"]

[verify]
command = "echo ok"
`

	// Create a mock lookup that returns debian constraint for apt_install
	mockLookup := func(actionName string) (*Constraint, bool) {
		if actionName == "apt_install" {
			return &Constraint{OS: "linux", LinuxFamily: "debian"}, true
		}
		return nil, true // known, no constraint
	}

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "constrained-tool.toml"), []byte(recipeWithConstraint), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	recipe, err := loader.Get("constrained-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Verify step analysis was computed
	if len(recipe.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(recipe.Steps))
	}

	analysis := recipe.Steps[0].Analysis()
	if analysis == nil {
		t.Fatal("Expected step analysis to be non-nil")
		return
	}

	// Should have debian constraint from apt_install
	if analysis.Constraint == nil {
		t.Fatal("Expected constraint to be non-nil")
	}
	if analysis.Constraint.OS != "linux" {
		t.Errorf("Expected OS=linux, got %q", analysis.Constraint.OS)
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("Expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
}

func TestLoader_StepAnalysisSkippedWithoutLookup(t *testing.T) {
	// Recipe with any action
	recipeContent := `[metadata]
name = "no-analysis-tool"
description = "A tool without step analysis"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "echo ok"
`

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "no-analysis-tool.toml"), []byte(recipeContent), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	// Don't set constraint lookup - analysis should be skipped
	loader := newTestLoaderWithLocal(reg, recipesDir)

	recipe, err := loader.Get("no-analysis-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Verify step analysis is nil (not computed)
	if len(recipe.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(recipe.Steps))
	}

	analysis := recipe.Steps[0].Analysis()
	if analysis != nil {
		t.Error("Expected step analysis to be nil when lookup not configured")
	}
}

func TestLoader_StepAnalysisWithFamilyVarying(t *testing.T) {
	// Recipe with linux_family interpolation
	recipeWithInterpolation := `[metadata]
name = "varying-tool"
description = "A tool with family-varying step"

[[steps]]
action = "download"
url = "https://example.com/{{linux_family}}/tool.tar.gz"

[verify]
command = "echo ok"
`

	mockLookup := func(actionName string) (*Constraint, bool) {
		return nil, true // known, no constraint
	}

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "varying-tool.toml"), []byte(recipeWithInterpolation), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	recipe, err := loader.Get("varying-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	analysis := recipe.Steps[0].Analysis()
	if analysis == nil {
		t.Fatal("Expected step analysis to be non-nil")
		return
	}

	if !analysis.FamilyVarying {
		t.Error("Expected FamilyVarying=true for step with {{linux_family}}")
	}
}

func TestLoader_StepAnalysisConflictError(t *testing.T) {
	// Recipe with conflicting constraint (apt_install + darwin OS)
	conflictRecipe := `[metadata]
name = "conflict-tool"
description = "A tool with conflicting constraints"

[[steps]]
action = "apt_install"
packages = ["curl"]
[steps.when]
os = ["darwin"]

[verify]
command = "echo ok"
`

	// apt_install requires linux/debian, but when clause says darwin
	mockLookup := func(actionName string) (*Constraint, bool) {
		if actionName == "apt_install" {
			return &Constraint{OS: "linux", LinuxFamily: "debian"}, true
		}
		return nil, true
	}

	// Create local recipes directory
	recipesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipesDir, "conflict-tool.toml"), []byte(conflictRecipe), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	loader := newTestLoaderWithLocal(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	_, err := loader.Get("conflict-tool", LoaderOptions{})
	if err == nil {
		t.Fatal("Expected error for conflicting constraints")
	}

	// Should contain error about OS conflict
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("Expected error about conflict, got: %v", err)
	}
}

func TestLoader_Get_RequireEmbedded(t *testing.T) {
	// Test that RequireEmbedded=true only checks embedded recipes
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// Test 1: Embedded recipe should be found with RequireEmbedded=true
	// Use a known embedded recipe (go toolchain)
	recipe, err := loader.Get("go", LoaderOptions{RequireEmbedded: true})
	if err != nil {
		t.Fatalf("Get() with RequireEmbedded=true failed for embedded recipe: %v", err)
	}
	if recipe.Metadata.Name != "go" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "go")
	}

	// Test 2: Non-embedded recipe should fail with RequireEmbedded=true
	// Use a loader without embedded recipes to simulate this
	loaderNoEmbed := newTestLoaderNoEmbedded(reg, "")
	_, err = loaderNoEmbed.Get("nonexistent-recipe", LoaderOptions{RequireEmbedded: true})
	if err == nil {
		t.Error("Get() with RequireEmbedded=true should fail for non-embedded recipe")
	}
	// Check error message contains helpful information
	if !strings.Contains(err.Error(), "not found in embedded registry") {
		t.Errorf("error message should mention 'not found in embedded registry', got: %v", err)
	}
}

func TestLoader_GetWithSource_Registry(t *testing.T) {
	mockRecipe := `[metadata]
name = "source-test"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "source-test --version"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recipes/s/source-test.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockRecipe))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	reg := registry.New(cacheDir)
	reg.BaseURL = server.URL

	// Use a loader with no embedded recipes so the registry provider handles it
	loader := newTestLoaderNoEmbedded(reg, "")

	recipe, source, err := loader.GetWithSource("source-test", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() failed: %v", err)
	}
	if recipe.Metadata.Name != "source-test" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "source-test")
	}
	if source != SourceRegistry {
		t.Errorf("source = %q, want %q", source, SourceRegistry)
	}
}

func TestLoader_GetWithSource_Local(t *testing.T) {
	localDir := t.TempDir()
	recipePath := filepath.Join(localDir, "local-tool.toml")
	err := os.WriteFile(recipePath, []byte(`[metadata]
name = "local-tool"
description = "A local tool"

[[steps]]
action = "download"
url = "https://example.com/local.tar.gz"

[verify]
command = "local-tool --version"
`), 0644)
	if err != nil {
		t.Fatalf("failed to write local recipe: %v", err)
	}

	reg := registry.New(t.TempDir())
	loader := newTestLoaderNoEmbedded(reg, localDir)

	recipe, source, err := loader.GetWithSource("local-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() failed: %v", err)
	}
	if recipe.Metadata.Name != "local-tool" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "local-tool")
	}
	if source != SourceLocal {
		t.Errorf("source = %q, want %q", source, SourceLocal)
	}
}

func TestLoader_GetWithSource_Embedded(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := newTestLoaderWithRegistry(reg)

	// "go" is a known embedded recipe
	recipe, source, err := loader.GetWithSource("go", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() failed: %v", err)
	}
	if recipe.Metadata.Name != "go" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "go")
	}
	if source != SourceEmbedded {
		t.Errorf("source = %q, want %q", source, SourceEmbedded)
	}
}

func TestLoader_GetWithSource_CacheRetainsSource(t *testing.T) {
	localDir := t.TempDir()
	recipePath := filepath.Join(localDir, "cached-tool.toml")
	err := os.WriteFile(recipePath, []byte(`[metadata]
name = "cached-tool"
description = "A tool for cache testing"

[[steps]]
action = "download"
url = "https://example.com/cached.tar.gz"

[verify]
command = "cached-tool --version"
`), 0644)
	if err != nil {
		t.Fatalf("failed to write recipe: %v", err)
	}

	reg := registry.New(t.TempDir())
	loader := newTestLoaderNoEmbedded(reg, localDir)

	// First call
	_, source1, err := loader.GetWithSource("cached-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() first call failed: %v", err)
	}

	// Second call should use cache but still return correct source
	_, source2, err := loader.GetWithSource("cached-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() second call failed: %v", err)
	}

	if source1 != source2 {
		t.Errorf("source changed between calls: %q -> %q", source1, source2)
	}
	if source1 != SourceLocal {
		t.Errorf("source = %q, want %q", source1, SourceLocal)
	}
}

// --- GetFromSource tests ---

// mockProvider is a simple RecipeProvider for testing GetFromSource.
type mockProvider struct {
	source  RecipeSource
	recipes map[string][]byte
}

func (m *mockProvider) Get(_ context.Context, name string) ([]byte, error) {
	data, ok := m.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in %s", name, m.source)
	}
	return data, nil
}

func (m *mockProvider) List(_ context.Context) ([]RecipeInfo, error) {
	var result []RecipeInfo
	for name := range m.recipes {
		result = append(result, RecipeInfo{Name: name, Source: m.source})
	}
	return result, nil
}

func (m *mockProvider) Source() RecipeSource {
	return m.source
}

func TestLoader_GetFromSource_Central_Registry(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "my-tool"
description = "test"

[[steps]]
action = "download"

[verify]
command = "my-tool --version"
`)

	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{"my-tool": recipeData},
	}

	loader := NewLoader(registryProvider)

	data, err := loader.GetFromSource(context.Background(), "my-tool", SourceCentral)
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(recipeData) {
		t.Error("GetFromSource() returned unexpected data")
	}
}

func TestLoader_GetFromSource_Central_Embedded(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "embedded-tool"
description = "test"

[[steps]]
action = "download"

[verify]
command = "embedded-tool --version"
`)

	// Only an embedded provider, no registry
	embeddedProvider := &mockProvider{
		source:  SourceEmbedded,
		recipes: map[string][]byte{"embedded-tool": recipeData},
	}

	loader := NewLoader(embeddedProvider)

	data, err := loader.GetFromSource(context.Background(), "embedded-tool", SourceCentral)
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(recipeData) {
		t.Error("GetFromSource() returned unexpected data")
	}
}

func TestLoader_GetFromSource_Central_PrefersRegistry(t *testing.T) {
	registryData := []byte("registry-version")
	embeddedData := []byte("embedded-version")

	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{"tool": registryData},
	}
	embeddedProvider := &mockProvider{
		source:  SourceEmbedded,
		recipes: map[string][]byte{"tool": embeddedData},
	}

	loader := NewLoader(embeddedProvider, registryProvider)

	data, err := loader.GetFromSource(context.Background(), "tool", SourceCentral)
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(registryData) {
		t.Errorf("expected registry data, got %q", string(data))
	}
}

func TestLoader_GetFromSource_Local(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "local-tool"
description = "local"

[[steps]]
action = "download"

[verify]
command = "local-tool --version"
`)

	localProvider := &mockProvider{
		source:  SourceLocal,
		recipes: map[string][]byte{"local-tool": recipeData},
	}

	loader := NewLoader(localProvider)

	data, err := loader.GetFromSource(context.Background(), "local-tool", "local")
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(recipeData) {
		t.Error("GetFromSource() returned unexpected data")
	}
}

func TestLoader_GetFromSource_Distributed(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "dist-tool"
description = "distributed"

[[steps]]
action = "download"

[verify]
command = "dist-tool --version"
`)

	distProvider := &mockProvider{
		source:  RecipeSource("acme/tools"),
		recipes: map[string][]byte{"dist-tool": recipeData},
	}

	loader := NewLoader(distProvider)

	data, err := loader.GetFromSource(context.Background(), "dist-tool", "acme/tools")
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(recipeData) {
		t.Error("GetFromSource() returned unexpected data")
	}
}

func TestLoader_GetFromSource_UnknownSource(t *testing.T) {
	loader := NewLoader()

	_, err := loader.GetFromSource(context.Background(), "tool", "unknown")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "no provider registered") {
		t.Errorf("expected 'no provider registered' in error, got: %v", err)
	}
}

func TestLoader_GetFromSource_NoMatchingDistributedProvider(t *testing.T) {
	loader := NewLoader()

	_, err := loader.GetFromSource(context.Background(), "tool", "acme/tools")
	if err == nil {
		t.Fatal("expected error for unregistered distributed source")
	}
	if !strings.Contains(err.Error(), "no provider registered") {
		t.Errorf("expected 'no provider registered' in error, got: %v", err)
	}
}

func TestLoader_GetFromSource_CentralNotFound(t *testing.T) {
	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{},
	}

	loader := NewLoader(registryProvider)

	_, err := loader.GetFromSource(context.Background(), "nonexistent", SourceCentral)
	if err == nil {
		t.Fatal("expected error for recipe not found in central")
	}
	if !strings.Contains(err.Error(), "not found in central registry") {
		t.Errorf("expected 'not found in central registry' in error, got: %v", err)
	}
}

func TestLoader_GetFromSource_CentralPropagatesRealErrors(t *testing.T) {
	// Registry returns a real error (not "not found") -- should propagate, not fall through to embedded.
	errorProvider := &mockErrorProvider{
		source: SourceRegistry,
		err:    fmt.Errorf("connection timeout"),
	}
	embeddedProvider := &mockProvider{
		source:  SourceEmbedded,
		recipes: map[string][]byte{"tool": []byte("embedded-data")},
	}

	loader := NewLoader(errorProvider, embeddedProvider)

	_, err := loader.GetFromSource(context.Background(), "tool", SourceCentral)
	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "connection timeout") {
		t.Errorf("expected 'connection timeout' in error, got: %v", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Error("should not be a not-found error")
	}
}

// mockErrorProvider always returns a specific error for Get calls.
type mockErrorProvider struct {
	source RecipeSource
	err    error
}

func (m *mockErrorProvider) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, m.err
}

func (m *mockErrorProvider) List(_ context.Context) ([]RecipeInfo, error) {
	return nil, nil
}

func (m *mockErrorProvider) Source() RecipeSource {
	return m.source
}

func TestLoader_GetFromSource_BypassesCache(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "cached-tool"
description = "test"

[[steps]]
action = "download"

[verify]
command = "cached-tool --version"
`)

	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{"cached-tool": recipeData},
	}

	loader := NewLoader(registryProvider)

	// Pre-populate the in-memory cache with different data
	loader.CacheRecipe("cached-tool", &Recipe{
		Metadata: MetadataSection{Name: "cached-tool", Description: "cached version"},
	})

	// GetFromSource should bypass the cache and return provider data
	data, err := loader.GetFromSource(context.Background(), "cached-tool", SourceCentral)
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}
	if string(data) != string(recipeData) {
		t.Error("GetFromSource() should bypass cache and return fresh data from provider")
	}
}

func TestLoader_GetFromSource_DoesNotWriteToCache(t *testing.T) {
	recipeData := []byte(`[metadata]
name = "no-cache-tool"
description = "test"

[[steps]]
action = "download"

[verify]
command = "no-cache-tool --version"
`)

	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{"no-cache-tool": recipeData},
	}

	loader := NewLoader(registryProvider)

	// Verify cache is empty
	if loader.Count() != 0 {
		t.Fatal("expected empty cache before GetFromSource")
	}

	_, err := loader.GetFromSource(context.Background(), "no-cache-tool", SourceCentral)
	if err != nil {
		t.Fatalf("GetFromSource() failed: %v", err)
	}

	// Cache should still be empty
	if loader.Count() != 0 {
		t.Error("GetFromSource() should not write to the in-memory cache")
	}
}

// --- scenario-19: Qualified name routing to DistributedProvider ---

func TestSplitQualifiedName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantQ  string
		wantR  string
		wantOK bool
	}{
		{"valid qualified name", "acme/tools:my-recipe", "acme/tools", "my-recipe", true},
		{"bare name no colon", "my-recipe", "", "", false},
		{"bare name with slash", "acme/tools", "", "", false},
		{"colon but no slash in qualifier", "tools:my-recipe", "", "", false},
		{"empty recipe name", "acme/tools:", "", "", false},
		{"empty qualifier", ":my-recipe", "", "", false},
		{"multiple colons uses first", "acme/tools:sub:recipe", "acme/tools", "sub:recipe", true},
		{"nested path in qualifier", "acme/sub/tools:recipe", "", "", false},
		{"empty string", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, r, ok := splitQualifiedName(tt.input)
			if ok != tt.wantOK {
				t.Errorf("splitQualifiedName(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
				return
			}
			if ok {
				if q != tt.wantQ {
					t.Errorf("splitQualifiedName(%q) qualifier = %q, want %q", tt.input, q, tt.wantQ)
				}
				if r != tt.wantR {
					t.Errorf("splitQualifiedName(%q) recipeName = %q, want %q", tt.input, r, tt.wantR)
				}
			}
		})
	}
}

func TestLoader_GetWithContext_QualifiedName(t *testing.T) {
	distRecipe := []byte(`[metadata]
name = "dist-tool"
description = "A distributed tool"

[[steps]]
action = "download"

[verify]
command = "dist-tool --version"
`)

	centralRecipe := []byte(`[metadata]
name = "dist-tool"
description = "Central version"

[[steps]]
action = "download"

[verify]
command = "dist-tool --version"
`)

	distProvider := &mockProvider{
		source:  RecipeSource("acme/tools"),
		recipes: map[string][]byte{"dist-tool": distRecipe},
	}

	registryProvider := &mockProvider{
		source:  SourceRegistry,
		recipes: map[string][]byte{"dist-tool": centralRecipe},
	}

	loader := NewLoader(registryProvider, distProvider)

	t.Run("qualified name routes to distributed provider", func(t *testing.T) {
		recipe, err := loader.GetWithContext(context.Background(), "acme/tools:dist-tool", LoaderOptions{})
		if err != nil {
			t.Fatalf("GetWithContext() failed: %v", err)
		}
		if recipe.Metadata.Description != "A distributed tool" {
			t.Errorf("expected distributed version, got description %q", recipe.Metadata.Description)
		}
	})

	t.Run("bare name routes to central provider", func(t *testing.T) {
		recipe, err := loader.GetWithContext(context.Background(), "dist-tool", LoaderOptions{})
		if err != nil {
			t.Fatalf("GetWithContext() failed: %v", err)
		}
		if recipe.Metadata.Description != "Central version" {
			t.Errorf("expected central version, got description %q", recipe.Metadata.Description)
		}
	})

	t.Run("qualified name uses distinct cache key", func(t *testing.T) {
		// Both should be cached now with different keys
		if loader.Count() != 2 {
			t.Errorf("expected 2 cached recipes, got %d", loader.Count())
		}

		// Fetch again -- should hit cache
		recipe, err := loader.GetWithContext(context.Background(), "acme/tools:dist-tool", LoaderOptions{})
		if err != nil {
			t.Fatalf("GetWithContext() failed on cache hit: %v", err)
		}
		if recipe.Metadata.Description != "A distributed tool" {
			t.Error("cache should return distributed version for qualified name")
		}
	})
}

func TestLoader_GetWithContext_QualifiedName_NoProvider(t *testing.T) {
	loader := NewLoader()

	_, err := loader.GetWithContext(context.Background(), "acme/tools:some-recipe", LoaderOptions{})
	if err == nil {
		t.Fatal("expected error for unregistered distributed source")
	}
	if !strings.Contains(err.Error(), "no provider registered") {
		t.Errorf("expected 'no provider registered' in error, got: %v", err)
	}
}

func TestLoader_GetWithContext_QualifiedName_RecipeNotFound(t *testing.T) {
	distProvider := &mockProvider{
		source:  RecipeSource("acme/tools"),
		recipes: map[string][]byte{},
	}

	loader := NewLoader(distProvider)

	_, err := loader.GetWithContext(context.Background(), "acme/tools:missing", LoaderOptions{})
	if err == nil {
		t.Fatal("expected error for missing recipe in distributed source")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestLoader_GetWithSource_QualifiedDistributed(t *testing.T) {
	distRecipe := []byte(`[metadata]
name = "dist-tool"
description = "distributed"

[[steps]]
action = "download"

[verify]
command = "dist-tool --version"
`)

	distProvider := &mockProvider{
		source:  RecipeSource("acme/tools"),
		recipes: map[string][]byte{"dist-tool": distRecipe},
	}

	loader := NewLoader(distProvider)

	// Load via qualified name
	recipe, source, err := loader.GetWithSource("acme/tools:dist-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("GetWithSource() failed: %v", err)
	}
	if recipe.Metadata.Name != "dist-tool" {
		t.Errorf("recipe name = %q, want %q", recipe.Metadata.Name, "dist-tool")
	}
	if source != RecipeSource("acme/tools") {
		t.Errorf("source = %q, want %q", source, "acme/tools")
	}
}

// failingProvider is a test provider that always returns an error on List.
type failingProvider struct{}

func (f *failingProvider) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("always fails")
}

func (f *failingProvider) List(_ context.Context) ([]RecipeInfo, error) {
	return nil, fmt.Errorf("list always fails")
}

func (f *failingProvider) Source() RecipeSource {
	return RecipeSource("failing/source")
}

func TestLoader_ListAllWithSource_ContinuesOnProviderError(t *testing.T) {
	// Create a loader with a working provider and a failing one
	store := NewMemoryStore(map[string][]byte{
		"good-tool.toml": []byte(`[metadata]
name = "good-tool"
description = "A good tool"

[[steps]]
action = "download"
url = "https://example.com/good.tar.gz"

[verify]
command = "good-tool --version"
`),
	})
	goodProvider := NewRegistryProvider("test", SourceLocal, Manifest{Layout: "flat"}, store)

	loader := NewLoader(goodProvider, &failingProvider{})

	recipes, errs := loader.ListAllWithSource()

	// Should have the good tool's recipe
	if len(recipes) != 1 {
		t.Errorf("expected 1 recipe, got %d", len(recipes))
	}
	if len(recipes) > 0 && recipes[0].Name != "good-tool" {
		t.Errorf("expected recipe 'good-tool', got %q", recipes[0].Name)
	}

	// Should have collected the error from the failing provider
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if len(errs) > 0 && !strings.Contains(errs[0].Error(), "failing/source") {
		t.Errorf("expected error to mention failing source, got: %v", errs[0])
	}
}
