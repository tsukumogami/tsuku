package recipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

var testRecipeTOML = `[metadata]
name = "test-tool"
description = "A test tool"

[metadata.satisfies]
homebrew = ["test-tool@3"]

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "test-tool --version"
`

var testLibraryTOML = `[metadata]
name = "test-lib"
type = "library"

[metadata.satisfies]
homebrew = ["test-lib@2"]
crates-io = ["libtest"]

[[steps]]
action = "download"
url = "https://example.com/lib.tar.gz"
`

func TestRegistryProvider_GetFlat(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"test-tool.toml": []byte(testRecipeTOML),
	})
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{Layout: "flat"}, store)

	data, err := p.Get(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if string(data) != testRecipeTOML {
		t.Error("unexpected data returned")
	}
}

func TestRegistryProvider_GetGrouped(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"t/test-tool.toml": []byte(testRecipeTOML),
	})
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{Layout: "grouped"}, store)

	data, err := p.Get(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if string(data) != testRecipeTOML {
		t.Error("unexpected data returned")
	}
}

func TestRegistryProvider_GetNotFound(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{})
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{Layout: "flat"}, store)

	_, err := p.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing recipe")
	}
}

func TestRegistryProvider_List(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"test-tool.toml": []byte(testRecipeTOML),
		"test-lib.toml":  []byte(testLibraryTOML),
	})
	p := NewRegistryProvider("test", SourceLocal, Manifest{Layout: "flat"}, store)

	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 recipes, got %d", len(infos))
	}

	found := make(map[string]bool)
	for _, info := range infos {
		found[info.Name] = true
		if info.Source != SourceLocal {
			t.Errorf("expected source %q, got %q", SourceLocal, info.Source)
		}
	}
	if !found["test-tool"] || !found["test-lib"] {
		t.Error("expected both test-tool and test-lib in list")
	}
}

func TestRegistryProvider_Source(t *testing.T) {
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{}, NewMemoryStore(nil))
	if p.Source() != SourceEmbedded {
		t.Errorf("expected %q, got %q", SourceEmbedded, p.Source())
	}
}

func TestRegistryProvider_SatisfiesEntries(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"test-tool.toml": []byte(testRecipeTOML),
		"test-lib.toml":  []byte(testLibraryTOML),
	})
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{Layout: "flat"}, store)

	entries, err := p.SatisfiesEntries(context.Background())
	if err != nil {
		t.Fatalf("SatisfiesEntries() failed: %v", err)
	}

	if entries["test-tool@3"] != "test-tool" {
		t.Errorf("expected test-tool@3 -> test-tool, got %q", entries["test-tool@3"])
	}
	if entries["test-lib@2"] != "test-lib" {
		t.Errorf("expected test-lib@2 -> test-lib, got %q", entries["test-lib@2"])
	}
	if entries["libtest"] != "test-lib" {
		t.Errorf("expected libtest -> test-lib, got %q", entries["libtest"])
	}
}

func TestRegistryProvider_Has(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"test-tool.toml": []byte(testRecipeTOML),
	})
	p := NewRegistryProvider("test", SourceEmbedded, Manifest{Layout: "flat"}, store)

	if !p.Has(context.Background(), "test-tool") {
		t.Error("expected Has(test-tool) to return true")
	}
	if p.Has(context.Background(), "missing") {
		t.Error("expected Has(missing) to return false")
	}
}

func TestRegistryProvider_RecipePath(t *testing.T) {
	tests := []struct {
		layout string
		name   string
		want   string
	}{
		{"flat", "go", "go.toml"},
		{"flat", "cargo-audit", "cargo-audit.toml"},
		{"", "go", "go.toml"}, // empty defaults to flat
		{"grouped", "go", "g/go.toml"},
		{"grouped", "cargo-audit", "c/cargo-audit.toml"},
	}

	for _, tc := range tests {
		p := &RegistryProvider{manifest: Manifest{Layout: tc.layout}}
		got := p.recipePath(tc.name)
		if got != tc.want {
			t.Errorf("recipePath(%q, layout=%q) = %q, want %q", tc.name, tc.layout, got, tc.want)
		}
	}
}

func TestRegistryProvider_RecipeNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"go.toml", "go"},
		{"g/go.toml", "go"},
		{"cargo-audit.toml", "cargo-audit"},
		{"c/cargo-audit.toml", "cargo-audit"},
	}

	for _, tc := range tests {
		got := recipeNameFromPath(tc.path)
		if got != tc.want {
			t.Errorf("recipeNameFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// Test that the factory functions produce working providers.
func TestNewEmbeddedProvider_ReturnsRegistryProvider(t *testing.T) {
	er, err := NewEmbeddedRegistry()
	if err != nil {
		t.Fatalf("NewEmbeddedRegistry() failed: %v", err)
	}

	p := NewEmbeddedProvider(er)
	if p == nil {
		t.Fatal("NewEmbeddedProvider() returned nil")
	}
	if p.Source() != SourceEmbedded {
		t.Errorf("expected source %q, got %q", SourceEmbedded, p.Source())
	}

	// Should be able to list embedded recipes
	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(infos) == 0 {
		t.Error("expected at least one embedded recipe")
	}

	// Should be able to get an embedded recipe
	data, err := p.Get(context.Background(), infos[0].Name)
	if err != nil {
		t.Fatalf("Get(%s) failed: %v", infos[0].Name, err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty recipe data")
	}
}

func TestNewEmbeddedProvider_NilRegistry(t *testing.T) {
	p := NewEmbeddedProvider(nil)
	if p != nil {
		t.Error("expected nil for nil registry")
	}
}

func TestNewLocalProvider_ReturnsRegistryProvider(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "my-tool.toml"), []byte(testRecipeTOML), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewLocalProvider(dir)
	if p == nil {
		t.Fatal("NewLocalProvider() returned nil")
	}
	if p.Source() != SourceLocal {
		t.Errorf("expected source %q, got %q", SourceLocal, p.Source())
	}

	data, err := p.Get(context.Background(), "my-tool")
	if err != nil {
		t.Fatalf("Get(my-tool) failed: %v", err)
	}
	if string(data) != testRecipeTOML {
		t.Error("unexpected data returned")
	}

	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "my-tool" {
		t.Errorf("expected [my-tool], got %v", infos)
	}
}

func TestNewLocalProvider_EmptyDir(t *testing.T) {
	p := NewLocalProvider("")
	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if infos != nil {
		t.Errorf("expected nil for empty dir, got %v", infos)
	}
}

// Test that RegistryProvider integrates with the Loader correctly.
func TestRegistryProvider_LoaderIntegration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "custom-tool.toml"), []byte(`[metadata]
name = "custom-tool"

[[steps]]
action = "download"
url = "https://example.com/tool.tar.gz"

[verify]
command = "custom-tool --version"
`), 0644)

	local := NewLocalProvider(dir)
	er, _ := NewEmbeddedRegistry()
	embedded := NewEmbeddedProvider(er)

	var providers []RecipeProvider
	providers = append(providers, local)
	if embedded != nil {
		providers = append(providers, embedded)
	}
	loader := NewLoader(providers...)

	// Should find the local recipe
	r, err := loader.Get("custom-tool", LoaderOptions{})
	if err != nil {
		t.Fatalf("Get(custom-tool) failed: %v", err)
	}
	if r.Metadata.Name != "custom-tool" {
		t.Errorf("expected custom-tool, got %q", r.Metadata.Name)
	}

	// Should still find embedded recipes
	r, err = loader.Get("openssl", LoaderOptions{RequireEmbedded: true})
	if err != nil {
		t.Fatalf("Get(openssl) failed: %v", err)
	}
	if r.Metadata.Name != "openssl" {
		t.Errorf("expected openssl, got %q", r.Metadata.Name)
	}
}
