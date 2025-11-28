package recipe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/registry"
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
