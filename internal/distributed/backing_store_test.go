package distributed

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestGitHubBackingStore_ImplementsBackingStore(t *testing.T) {
	var _ recipe.BackingStore = &GitHubBackingStore{}
}

func TestGitHubBackingStore_Get(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	// Pre-populate source meta and recipe cache
	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	recipeMeta := &RecipeMeta{FetchedAt: time.Now()}
	if err := cache.PutRecipe("acme", "tools", "my-tool", []byte(validRecipeTOML), recipeMeta); err != nil {
		t.Fatalf("PutRecipe: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	store := NewGitHubBackingStore("acme", "tools", gc)

	// Get with flat path
	data, err := store.Get(context.Background(), "my-tool.toml")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != validRecipeTOML {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestGitHubBackingStore_Get_GroupedPath(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	recipeMeta := &RecipeMeta{FetchedAt: time.Now()}
	if err := cache.PutRecipe("acme", "tools", "my-tool", []byte(validRecipeTOML), recipeMeta); err != nil {
		t.Fatalf("PutRecipe: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	store := NewGitHubBackingStore("acme", "tools", gc)

	// Get with grouped path (directory prefix stripped)
	data, err := store.Get(context.Background(), "m/my-tool.toml")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != validRecipeTOML {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestGitHubBackingStore_Get_NotFound(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	meta := &SourceMeta{
		Branch:    "main",
		Files:     map[string]string{},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	store := NewGitHubBackingStore("acme", "tools", gc)

	_, err := store.Get(context.Background(), "missing.toml")
	if err == nil {
		t.Fatal("expected error for missing recipe")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %s", err)
	}
}

func TestGitHubBackingStore_List(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"alpha": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/alpha.toml",
			"beta":  "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/beta.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("HTTP client should not be called, got: %s", req.URL)
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	store := NewGitHubBackingStore("acme", "tools", gc)

	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	sort.Strings(paths)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "alpha.toml" || paths[1] != "beta.toml" {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestRecipeNameFromStorePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"go.toml", "go"},
		{"g/go.toml", "go"},
		{"my-tool.toml", "my-tool"},
		{"m/my-tool.toml", "my-tool"},
		{"deep/nested/tool.toml", "tool"},
	}

	for _, tt := range tests {
		got := recipeNameFromStorePath(tt.path)
		if got != tt.want {
			t.Errorf("recipeNameFromStorePath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
