package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestRecipe(t *testing.T, dir, name, content string) {
	t.Helper()
	letter := string(strings.ToLower(name)[0])
	letterDir := filepath.Join(dir, letter)
	if err := os.MkdirAll(letterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(letterDir, name+".toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGraduateEntries_NoRecipesDir(t *testing.T) {
	entries := []SeedEntry{{Name: "bat", Builder: "github", Source: "sharkdp/bat"}}
	result, err := GraduateEntries(entries, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Kept) != 1 {
		t.Errorf("expected 1 kept, got %d", len(result.Kept))
	}
	if len(result.Graduated) != 0 {
		t.Errorf("expected 0 graduated, got %d", len(result.Graduated))
	}
}

func TestGraduateEntries_BasicGraduation(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "jq", `[metadata]
name = "jq"
description = "JSON processor"
homepage = "https://jqlang.github.io/jq/"
`)

	entries := []SeedEntry{
		{Name: "jq", Builder: "homebrew", Source: "jq", Description: "JSON processor"},
		{Name: "bat", Builder: "github", Source: "sharkdp/bat"},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Kept) != 1 {
		t.Errorf("expected 1 kept, got %d", len(result.Kept))
	}
	if result.Kept[0].Name != "bat" {
		t.Errorf("expected bat to be kept, got %s", result.Kept[0].Name)
	}
	if len(result.Graduated) != 1 {
		t.Errorf("expected 1 graduated, got %d", len(result.Graduated))
	}
	if result.Graduated[0].Name != "jq" {
		t.Errorf("expected jq to graduate, got %s", result.Graduated[0].Name)
	}
}

func TestGraduateEntries_DisambiguationKept(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "bat", `[metadata]
name = "bat"
description = "A cat clone with wings"
`)

	entries := []SeedEntry{
		{Name: "bat", Builder: "github", Source: "sharkdp/bat", Disambiguation: true},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Kept) != 1 {
		t.Errorf("expected 1 kept (disambiguation), got %d", len(result.Kept))
	}
	if len(result.Graduated) != 0 {
		t.Errorf("expected 0 graduated, got %d", len(result.Graduated))
	}
}

func TestGraduateEntries_CaseInsensitive(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "jq", `[metadata]
name = "jq"
`)

	entries := []SeedEntry{
		{Name: "JQ", Builder: "homebrew", Source: "jq"},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Graduated) != 1 {
		t.Errorf("expected case-insensitive match to graduate, got %d graduated", len(result.Graduated))
	}
}

func TestGraduateEntries_BackfillDescription(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "mytool", `[metadata]
name = "mytool"

[version]
source = "homebrew"
`)

	entries := []SeedEntry{
		{Name: "mytool", Builder: "homebrew", Source: "mytool", Description: "A useful tool", Homepage: "https://mytool.dev"},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Graduated) != 1 {
		t.Fatalf("expected 1 graduated, got %d", len(result.Graduated))
	}
	if len(result.Backfills) != 1 {
		t.Fatalf("expected 1 backfill, got %d", len(result.Backfills))
	}
	if !result.Backfills[0].Description {
		t.Error("expected description backfill")
	}
	if !result.Backfills[0].Homepage {
		t.Error("expected homepage backfill")
	}

	// Verify the file was updated
	data, err := os.ReadFile(filepath.Join(recipesDir, "m", "mytool.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `description = "A useful tool"`) {
		t.Error("description not found in updated recipe")
	}
	if !strings.Contains(content, `homepage = "https://mytool.dev"`) {
		t.Error("homepage not found in updated recipe")
	}
	// Original content preserved
	if !strings.Contains(content, `source = "homebrew"`) {
		t.Error("original content lost during backfill")
	}
}

func TestGraduateEntries_NoBackfillWhenRecipeHasMetadata(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "jq", `[metadata]
name = "jq"
description = "Existing description"
homepage = "https://existing.com"
`)

	entries := []SeedEntry{
		{Name: "jq", Builder: "homebrew", Source: "jq", Description: "New description", Homepage: "https://new.com"},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Backfills) != 0 {
		t.Errorf("expected no backfills when recipe has metadata, got %d", len(result.Backfills))
	}

	// Verify recipe wasn't modified
	data, err := os.ReadFile(filepath.Join(recipesDir, "j", "jq.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "New description") {
		t.Error("recipe should not have been modified")
	}
}

func TestGraduateEntries_PartialBackfill(t *testing.T) {
	recipesDir := t.TempDir()
	writeTestRecipe(t, recipesDir, "mytool", `[metadata]
name = "mytool"
description = "Already has description"
`)

	entries := []SeedEntry{
		{Name: "mytool", Builder: "homebrew", Source: "mytool", Description: "Other desc", Homepage: "https://mytool.dev"},
	}
	result, err := GraduateEntries(entries, recipesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Backfills) != 1 {
		t.Fatalf("expected 1 backfill, got %d", len(result.Backfills))
	}
	if result.Backfills[0].Description {
		t.Error("should not backfill description when recipe already has one")
	}
	if !result.Backfills[0].Homepage {
		t.Error("expected homepage backfill")
	}
}

func TestGenerateWithGraduation(t *testing.T) {
	dir := t.TempDir()

	// Set up seeds
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "existing-tool", "builder": "homebrew", "source": "existing-tool"},
			{"name": "new-tool", "builder": "github", "source": "owner/new-tool"},
			{"name": "disambig-tool", "builder": "github", "source": "owner/disambig", "disambiguation": true}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up recipes dir with existing-tool and disambig-tool
	recipesDir := filepath.Join(dir, "recipes")
	writeTestRecipe(t, recipesDir, "existing-tool", `[metadata]
name = "existing-tool"
description = "Already exists"
`)
	writeTestRecipe(t, recipesDir, "disambig-tool", `[metadata]
name = "disambig-tool"
`)

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:   seedsDir,
		RecipesDir: recipesDir,
		OutputDir:  outputDir,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("total = %d, want 3", result.Total)
	}
	if result.Graduated != 1 {
		t.Errorf("graduated = %d, want 1", result.Graduated)
	}
	if result.Valid != 2 {
		t.Errorf("valid = %d, want 2 (new-tool + disambig-tool)", result.Valid)
	}

	// new-tool should be in output
	if _, err := LoadRegistryEntry(outputDir, "new-tool"); err != nil {
		t.Error("new-tool should be in output")
	}
	// disambig-tool should be in output
	if _, err := LoadRegistryEntry(outputDir, "disambig-tool"); err != nil {
		t.Error("disambig-tool should be in output (disambiguation)")
	}
	// existing-tool should NOT be in output
	if _, err := LoadRegistryEntry(outputDir, "existing-tool"); err == nil {
		t.Error("existing-tool should have graduated")
	}
}
