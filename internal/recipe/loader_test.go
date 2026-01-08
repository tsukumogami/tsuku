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

func TestNew(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := New(reg)

	if loader == nil {
		t.Fatal("New() returned nil")
	}
	if loader.registry != reg {
		t.Error("loader.registry not set correctly")
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

	loader := New(reg)

	recipe, err := loader.Get("test-tool")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if recipe.Metadata.Name != "test-tool" {
		t.Errorf("recipe.Metadata.Name = %q, want %q", recipe.Metadata.Name, "test-tool")
	}

	// Second call should use in-memory cache
	recipe2, err := loader.Get("test-tool")
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

	loader := New(reg)

	_, err := loader.Get("nonexistent")
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

	loader := New(reg)

	ctx := context.Background()
	recipe, err := loader.GetWithContext(ctx, "ctx-tool")
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

	loader := New(reg)

	// Initially empty
	names := loader.List()
	if len(names) != 0 {
		t.Errorf("List() returned %d items, want 0", len(names))
	}

	// Load a recipe
	_, _ = loader.Get("list-tool")

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

	loader := New(reg)

	if loader.Count() != 0 {
		t.Errorf("Count() = %d, want 0", loader.Count())
	}

	_, _ = loader.Get("count-tool")

	if loader.Count() != 1 {
		t.Errorf("Count() = %d, want 1", loader.Count())
	}
}

func TestLoader_Registry(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := New(reg)

	if loader.Registry() != reg {
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

	loader := New(reg)

	// Load a recipe
	_, _ = loader.Get("cache-tool")
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
	loader := New(reg)

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

func TestNewWithLocalRecipes(t *testing.T) {
	reg := registry.New(t.TempDir())
	recipesDir := "/some/recipes/dir"
	loader := NewWithLocalRecipes(reg, recipesDir)

	if loader == nil {
		t.Fatal("NewWithLocalRecipes() returned nil")
	}
	if loader.registry != reg {
		t.Error("loader.registry not set correctly")
	}
	if loader.recipesDir != recipesDir {
		t.Errorf("loader.recipesDir = %q, want %q", loader.recipesDir, recipesDir)
	}
}

func TestLoader_SetRecipesDir(t *testing.T) {
	reg := registry.New(t.TempDir())
	loader := New(reg)

	if loader.recipesDir != "" {
		t.Errorf("initial recipesDir should be empty, got %q", loader.recipesDir)
	}

	loader.SetRecipesDir("/test/dir")
	if loader.recipesDir != "/test/dir" {
		t.Errorf("recipesDir = %q, want %q", loader.recipesDir, "/test/dir")
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

	loader := NewWithLocalRecipes(reg, recipesDir)

	// Should load from local recipes directory
	recipe, err := loader.Get("local-tool")
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

	loader := NewWithLocalRecipes(reg, recipesDir)

	// Should load from local recipes directory, not registry
	recipe, err := loader.Get("priority-tool")
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

	loader := NewWithLocalRecipes(reg, recipesDir)

	// Should fall back to registry when local doesn't exist
	recipe, err := loader.Get("registry-tool")
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

	loader := NewWithLocalRecipes(reg, recipesDir)

	// Should return parse error, not fallback to registry
	_, err := loader.Get("invalid-tool")
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
	loader := NewWithoutEmbedded(reg, recipesDir) // Use NewWithoutEmbedded to exclude embedded recipes

	recipes, err := loader.ListAllWithSource()
	if err != nil {
		t.Fatalf("ListAllWithSource() failed: %v", err)
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
	loader := NewWithoutEmbedded(reg, recipesDir) // Use NewWithoutEmbedded to exclude embedded recipes

	recipes, err := loader.ListAllWithSource()
	if err != nil {
		t.Fatalf("ListAllWithSource() failed: %v", err)
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
	loader := NewWithLocalRecipes(reg, recipesDir)

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
	loader := NewWithLocalRecipes(reg, recipesDir)

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
	loader := New(reg) // No recipes dir set

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
	loader := NewWithLocalRecipes(reg, "/some/path")

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
				Verify:   VerifySection{Command: "test --version"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			recipe: Recipe{
				Metadata: MetadataSection{Name: ""},
				Steps:    []Step{{Action: "download"}},
				Verify:   VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing steps",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{},
				Verify:   VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing action in step",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: ""}},
				Verify:   VerifySection{Command: "test --version"},
			},
			wantErr: true,
		},
		{
			name: "missing verify command",
			recipe: Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: "download"}},
				Verify:   VerifySection{Command: ""},
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
	loader := New(reg)

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

	loader := NewWithLocalRecipes(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	recipe, err := loader.Get("constrained-tool")
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
	loader := NewWithLocalRecipes(reg, recipesDir)

	recipe, err := loader.Get("no-analysis-tool")
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

	loader := NewWithLocalRecipes(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	recipe, err := loader.Get("varying-tool")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	analysis := recipe.Steps[0].Analysis()
	if analysis == nil {
		t.Fatal("Expected step analysis to be non-nil")
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

	loader := NewWithLocalRecipes(reg, recipesDir)
	loader.SetConstraintLookup(mockLookup)

	_, err := loader.Get("conflict-tool")
	if err == nil {
		t.Fatal("Expected error for conflicting constraints")
	}

	// Should contain error about OS conflict
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("Expected error about conflict, got: %v", err)
	}
}
