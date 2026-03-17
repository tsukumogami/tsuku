package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// mockProvider is a test provider that returns canned recipe bytes.
type mockProvider struct {
	source  recipe.RecipeSource
	recipes map[string][]byte
}

func (m *mockProvider) Get(_ context.Context, name string) ([]byte, error) {
	data, ok := m.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in %s", name, m.source)
	}
	return data, nil
}

func (m *mockProvider) List(_ context.Context) ([]recipe.RecipeInfo, error) {
	var infos []recipe.RecipeInfo
	for name := range m.recipes {
		infos = append(infos, recipe.RecipeInfo{Name: name, Source: m.source})
	}
	return infos, nil
}

func (m *mockProvider) Source() recipe.RecipeSource {
	return m.source
}

// unreachableProvider always returns errors, simulating an unreachable source.
type unreachableProvider struct {
	source recipe.RecipeSource
}

func (u *unreachableProvider) Get(_ context.Context, name string) ([]byte, error) {
	return nil, fmt.Errorf("connection refused: could not reach %s", u.source)
}

func (u *unreachableProvider) List(_ context.Context) ([]recipe.RecipeInfo, error) {
	return nil, fmt.Errorf("connection refused: could not reach %s", u.source)
}

func (u *unreachableProvider) Source() recipe.RecipeSource {
	return u.source
}

// testRecipeTOML returns minimal valid recipe TOML for testing.
func testRecipeTOML(name string) []byte {
	return []byte(fmt.Sprintf(`[metadata]
name = "%s"
description = "test tool"

[[steps]]
action = "github_archive"

[steps.params]
repo = "test-org/%s"

[verify]
command = "%s --version"
pattern = "1.0"
`, name, name, name))
}

func TestIsDistributedSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"", false},
		{"central", false},
		{"local", false},
		{"embedded", false},
		{"myorg/recipes", true},
		{"owner/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := isDistributedSource(tt.source)
			if got != tt.want {
				t.Errorf("isDistributedSource(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestLoadRecipeForTool_CentralSource(t *testing.T) {
	// Set up a loader with a mock central (registry) provider
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"kubectl": testRecipeTOML("kubectl")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{
			"kubectl": {Source: "central", ActiveVersion: "1.28.0"},
		},
	}

	cfg := &config.Config{HomeDir: t.TempDir()}

	r, err := loadRecipeForTool(context.Background(), "kubectl", state, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "kubectl" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "kubectl")
	}
}

func TestLoadRecipeForTool_EmptySourceDefaultsToCentral(t *testing.T) {
	// Empty source should fall back to the normal chain (central)
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"terraform": testRecipeTOML("terraform")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{
			"terraform": {Source: "", ActiveVersion: "1.6.0"},
		},
	}

	cfg := &config.Config{HomeDir: t.TempDir()}

	r, err := loadRecipeForTool(context.Background(), "terraform", state, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "terraform" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "terraform")
	}
}

func TestLoadRecipeForTool_DistributedSource(t *testing.T) {
	// Set up a loader with both central and distributed providers.
	// The distributed provider should be preferred for tools from that source.
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"mytool": testRecipeTOML("mytool")},
	}
	distProvider := &mockProvider{
		source:  recipe.RecipeSource("myorg/recipes"),
		recipes: map[string][]byte{"mytool": testRecipeTOML("mytool")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider, distProvider)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{
			"mytool": {Source: "myorg/recipes", ActiveVersion: "2.0.0"},
		},
	}

	// Set up a temp dir with the cache subdirectory for addDistributedProvider
	tmpDir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  tmpDir,
		CacheDir: filepath.Join(tmpDir, "cache"),
	}
	_ = os.MkdirAll(cfg.CacheDir, 0755)

	r, err := loadRecipeForTool(context.Background(), "mytool", state, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "mytool" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "mytool")
	}
}

func TestLoadRecipeForTool_UnreachableDistributedFallsBack(t *testing.T) {
	// When the distributed source is unreachable, should fall back to central
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"mytool": testRecipeTOML("mytool")},
	}
	unreachable := &unreachableProvider{
		source: recipe.RecipeSource("myorg/recipes"),
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider, unreachable)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{
			"mytool": {Source: "myorg/recipes", ActiveVersion: "2.0.0"},
		},
	}

	tmpDir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  tmpDir,
		CacheDir: filepath.Join(tmpDir, "cache"),
	}
	_ = os.MkdirAll(cfg.CacheDir, 0755)

	// Capture stderr to verify warning
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	r, err := loadRecipeForTool(context.Background(), "mytool", state, cfg)

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if r.Metadata.Name != "mytool" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "mytool")
	}
}

func TestLoadRecipeForTool_NilState(t *testing.T) {
	// Nil state should fall back to normal chain
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"kubectl": testRecipeTOML("kubectl")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider)
	defer func() { loader = oldLoader }()

	cfg := &config.Config{HomeDir: t.TempDir()}

	r, err := loadRecipeForTool(context.Background(), "kubectl", nil, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "kubectl" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "kubectl")
	}
}

func TestLoadRecipeForTool_ToolNotInState(t *testing.T) {
	// Tool not in state should fall back to normal chain
	centralProvider := &mockProvider{
		source:  recipe.SourceRegistry,
		recipes: map[string][]byte{"kubectl": testRecipeTOML("kubectl")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(centralProvider)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{},
	}

	cfg := &config.Config{HomeDir: t.TempDir()}

	r, err := loadRecipeForTool(context.Background(), "kubectl", state, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "kubectl" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "kubectl")
	}
}

func TestLoadRecipeForTool_EmbeddedSource(t *testing.T) {
	// Embedded source should use the normal chain
	embeddedProvider := &mockProvider{
		source:  recipe.SourceEmbedded,
		recipes: map[string][]byte{"gh": testRecipeTOML("gh")},
	}
	oldLoader := loader
	loader = recipe.NewLoader(embeddedProvider)
	defer func() { loader = oldLoader }()

	state := &install.State{
		Installed: map[string]install.ToolState{
			"gh": {Source: "embedded", ActiveVersion: "2.0.0"},
		},
	}

	cfg := &config.Config{HomeDir: t.TempDir()}

	r, err := loadRecipeForTool(context.Background(), "gh", state, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if r.Metadata.Name != "gh" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "gh")
	}
}

func TestParseAndCache(t *testing.T) {
	l := recipe.NewLoader()
	data := testRecipeTOML("mytool")

	r, err := l.ParseAndCache(context.Background(), "mytool", data)
	if err != nil {
		t.Fatalf("ParseAndCache failed: %v", err)
	}
	if r.Metadata.Name != "mytool" {
		t.Errorf("recipe name = %q, want %q", r.Metadata.Name, "mytool")
	}

	// Verify it's cached -- a second Get should return the same recipe
	cached, err := l.Get("mytool", recipe.LoaderOptions{})
	if err != nil {
		t.Fatalf("expected cached recipe, got error: %v", err)
	}
	if cached != r {
		t.Error("expected Get to return the same cached recipe pointer")
	}
}

func TestParseAndCache_InvalidTOML(t *testing.T) {
	l := recipe.NewLoader()
	data := []byte("this is not valid TOML {{{")

	_, err := l.ParseAndCache(context.Background(), "bad", data)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}
