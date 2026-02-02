package seed

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRecipe(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("[metadata]\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFilterExistingRecipes_RegistryMatch(t *testing.T) {
	recipesDir := t.TempDir()
	writeRecipe(t, recipesDir, "b/bat.toml")

	packages := []Package{
		{ID: "bat", Name: "bat"},
		{ID: "fd", Name: "fd"},
	}
	kept, skipped := FilterExistingRecipes(packages, recipesDir, "")
	if len(kept) != 1 || kept[0].Name != "fd" {
		t.Errorf("expected [fd], got %v", kept)
	}
	if len(skipped) != 1 || skipped[0] != "bat" {
		t.Errorf("expected [bat] skipped, got %v", skipped)
	}
}

func TestFilterExistingRecipes_EmbeddedMatch(t *testing.T) {
	embeddedDir := t.TempDir()
	writeRecipe(t, embeddedDir, "cmake.toml")

	packages := []Package{
		{ID: "cmake", Name: "cmake"},
		{ID: "fd", Name: "fd"},
	}
	kept, skipped := FilterExistingRecipes(packages, "", embeddedDir)
	if len(kept) != 1 || kept[0].Name != "fd" {
		t.Errorf("expected [fd], got %v", kept)
	}
	if len(skipped) != 1 || skipped[0] != "cmake" {
		t.Errorf("expected [cmake] skipped, got %v", skipped)
	}
}

func TestFilterExistingRecipes_BothDirs(t *testing.T) {
	recipesDir := t.TempDir()
	embeddedDir := t.TempDir()
	writeRecipe(t, recipesDir, "b/bat.toml")
	writeRecipe(t, embeddedDir, "go.toml")

	packages := []Package{
		{ID: "bat", Name: "bat"},
		{ID: "go", Name: "go"},
		{ID: "fd", Name: "fd"},
	}
	kept, skipped := FilterExistingRecipes(packages, recipesDir, embeddedDir)
	if len(kept) != 1 || kept[0].Name != "fd" {
		t.Errorf("expected [fd], got %v", kept)
	}
	if len(skipped) != 2 {
		t.Errorf("expected 2 skipped, got %d", len(skipped))
	}
}

func TestFilterExistingRecipes_NoDirs(t *testing.T) {
	packages := []Package{
		{ID: "bat", Name: "bat"},
	}
	kept, skipped := FilterExistingRecipes(packages, "", "")
	if len(kept) != 1 {
		t.Errorf("expected all kept when no dirs, got %d", len(kept))
	}
	if len(skipped) != 0 {
		t.Errorf("expected none skipped, got %d", len(skipped))
	}
}

func TestFilterExistingRecipes_NoMatch(t *testing.T) {
	recipesDir := t.TempDir()
	embeddedDir := t.TempDir()

	packages := []Package{
		{ID: "bat", Name: "bat"},
		{ID: "fd", Name: "fd"},
	}
	kept, skipped := FilterExistingRecipes(packages, recipesDir, embeddedDir)
	if len(kept) != 2 {
		t.Errorf("expected all kept, got %d", len(kept))
	}
	if len(skipped) != 0 {
		t.Errorf("expected none skipped, got %d", len(skipped))
	}
}

func TestFilterExistingRecipes_EmptyInput(t *testing.T) {
	kept, skipped := FilterExistingRecipes(nil, "recipes", "embedded")
	if len(kept) != 0 {
		t.Errorf("expected empty kept, got %d", len(kept))
	}
	if len(skipped) != 0 {
		t.Errorf("expected empty skipped, got %d", len(skipped))
	}
}
