package recipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestWriteRecipe_Success(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test-tool.toml")

	recipe := &Recipe{
		Metadata: MetadataSection{
			Name:        "test-tool",
			Description: "A test tool",
			Homepage:    "https://example.com",
		},
		Version: VersionSection{
			Source: "github_releases",
		},
		Steps: []Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{
					"crate":       "test-tool",
					"executables": []string{"test-tool"},
				},
			},
		},
		Verify: VerifySection{
			Command: "test-tool --version",
		},
	}

	err := WriteRecipe(recipe, path)
	if err != nil {
		t.Fatalf("WriteRecipe() failed: %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Recipe file was not created")
	}

	// Verify no temp files remain
	matches, _ := filepath.Glob(filepath.Join(tmpDir, ".recipe-*.tmp"))
	if len(matches) > 0 {
		t.Errorf("Temporary files remain: %v", matches)
	}
}

func TestWriteRecipe_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roundtrip-tool.toml")

	original := &Recipe{
		Metadata: MetadataSection{
			Name:        "roundtrip-tool",
			Description: "Testing round-trip serialization",
			Homepage:    "https://example.com/roundtrip",
		},
		Version: VersionSection{
			Source:     "crates_io:roundtrip-tool",
			GitHubRepo: "example/roundtrip",
		},
		Steps: []Step{
			{
				Action:      "cargo_install",
				Description: "Install from crates.io",
				Params: map[string]interface{}{
					"crate":       "roundtrip-tool",
					"executables": []string{"rt", "roundtrip"},
				},
			},
		},
		Verify: VerifySection{
			Command: "rt --version",
			Pattern: "roundtrip",
		},
	}

	// Write the recipe
	err := WriteRecipe(original, path)
	if err != nil {
		t.Fatalf("WriteRecipe() failed: %v", err)
	}

	// Read it back
	var loaded Recipe
	if _, err := toml.DecodeFile(path, &loaded); err != nil {
		t.Fatalf("Failed to decode written recipe: %v", err)
	}

	// Verify key fields match
	if loaded.Metadata.Name != original.Metadata.Name {
		t.Errorf("Name mismatch: got %q, want %q", loaded.Metadata.Name, original.Metadata.Name)
	}
	if loaded.Metadata.Description != original.Metadata.Description {
		t.Errorf("Description mismatch: got %q, want %q", loaded.Metadata.Description, original.Metadata.Description)
	}
	if loaded.Version.Source != original.Version.Source {
		t.Errorf("Version.Source mismatch: got %q, want %q", loaded.Version.Source, original.Version.Source)
	}
	if len(loaded.Steps) != len(original.Steps) {
		t.Fatalf("Steps count mismatch: got %d, want %d", len(loaded.Steps), len(original.Steps))
	}
	if loaded.Steps[0].Action != original.Steps[0].Action {
		t.Errorf("Step action mismatch: got %q, want %q", loaded.Steps[0].Action, original.Steps[0].Action)
	}
	if loaded.Verify.Command != original.Verify.Command {
		t.Errorf("Verify.Command mismatch: got %q, want %q", loaded.Verify.Command, original.Verify.Command)
	}
}

func TestWriteRecipe_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "recipe.toml")

	recipe := &Recipe{
		Metadata: MetadataSection{Name: "nested-tool"},
		Steps:    []Step{{Action: "download"}},
		Verify:   VerifySection{Command: "nested-tool --version"},
	}

	err := WriteRecipe(recipe, nestedPath)
	if err != nil {
		t.Fatalf("WriteRecipe() failed: %v", err)
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Fatal("Recipe file was not created in nested directory")
	}
}

func TestWriteRecipe_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "overwrite.toml")

	// Write initial recipe
	initial := &Recipe{
		Metadata: MetadataSection{Name: "initial-name"},
		Steps:    []Step{{Action: "download"}},
		Verify:   VerifySection{Command: "test --version"},
	}
	if err := WriteRecipe(initial, path); err != nil {
		t.Fatalf("Initial WriteRecipe() failed: %v", err)
	}

	// Overwrite with new recipe
	updated := &Recipe{
		Metadata: MetadataSection{Name: "updated-name"},
		Steps:    []Step{{Action: "cargo_install"}},
		Verify:   VerifySection{Command: "updated --version"},
	}
	if err := WriteRecipe(updated, path); err != nil {
		t.Fatalf("Updated WriteRecipe() failed: %v", err)
	}

	// Verify the updated content
	var loaded Recipe
	if _, err := toml.DecodeFile(path, &loaded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if loaded.Metadata.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", loaded.Metadata.Name, "updated-name")
	}
}

func TestWriteRecipe_AtomicBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "atomic.toml")

	recipe := &Recipe{
		Metadata: MetadataSection{Name: "atomic-test"},
		Steps:    []Step{{Action: "download"}},
		Verify:   VerifySection{Command: "atomic --version"},
	}

	// Write successfully
	if err := WriteRecipe(recipe, path); err != nil {
		t.Fatalf("WriteRecipe() failed: %v", err)
	}

	// Verify no temp files remain after successful write
	matches, _ := filepath.Glob(filepath.Join(tmpDir, ".recipe-*.tmp"))
	if len(matches) > 0 {
		t.Errorf("Temporary files remain after successful write: %v", matches)
	}
}

func TestWriteRecipe_InvalidDirectory(t *testing.T) {
	// Create a file where we expect a directory
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocking-file")
	if err := os.WriteFile(blockingFile, []byte("block"), 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Try to write to a path that requires creating a directory where a file exists
	path := filepath.Join(blockingFile, "nested", "recipe.toml")

	recipe := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Action: "download"}},
		Verify:   VerifySection{Command: "test --version"},
	}

	err := WriteRecipe(recipe, path)
	if err == nil {
		t.Error("WriteRecipe() should fail when directory creation is blocked")
	}
}

func TestWriteRecipe_WithComplexStep(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "complex.toml")

	recipe := &Recipe{
		Metadata: MetadataSection{
			Name:         "complex-tool",
			Dependencies: []string{"dep1", "dep2"},
		},
		Version: VersionSection{
			Source:     "github_releases",
			GitHubRepo: "owner/repo",
			TagPrefix:  "v",
		},
		Steps: []Step{
			{
				Action:      "github_archive",
				Description: "Download from GitHub",
				Note:        "Supports multiple platforms",
				When:        &WhenClause{Platform: []string{"linux/amd64"}},
				Params: map[string]interface{}{
					"repo":     "owner/repo",
					"binaries": []string{"bin1", "bin2"},
				},
			},
		},
		Verify: VerifySection{
			Command: "complex --version",
			Pattern: `v\d+\.\d+\.\d+`,
		},
	}

	err := WriteRecipe(recipe, path)
	if err != nil {
		t.Fatalf("WriteRecipe() failed: %v", err)
	}

	// Read back and verify complex fields
	var loaded Recipe
	if _, err := toml.DecodeFile(path, &loaded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if len(loaded.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(loaded.Steps))
	}

	step := loaded.Steps[0]
	if step.Action != "github_archive" {
		t.Errorf("Step action = %q, want %q", step.Action, "github_archive")
	}
	if step.Description != "Download from GitHub" {
		t.Errorf("Step description = %q, want %q", step.Description, "Download from GitHub")
	}
}
