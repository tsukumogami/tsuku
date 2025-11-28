package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempDir(t *testing.T) {
	dir, cleanup := TempDir(t)
	defer cleanup()

	if dir == "" {
		t.Error("TempDir() returned empty string")
	}

	// Check directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("TempDir() directory does not exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("TempDir() did not create a directory")
	}
}

func TestNewTestConfig(t *testing.T) {
	cfg, cleanup := NewTestConfig(t)
	defer cleanup()

	// Check config fields are set
	if cfg.HomeDir == "" {
		t.Error("NewTestConfig() HomeDir is empty")
	}

	if cfg.ToolsDir == "" {
		t.Error("NewTestConfig() ToolsDir is empty")
	}

	// Check directories exist
	if _, err := os.Stat(cfg.ToolsDir); err != nil {
		t.Errorf("NewTestConfig() ToolsDir does not exist: %v", err)
	}

	if _, err := os.Stat(cfg.CurrentDir); err != nil {
		t.Errorf("NewTestConfig() CurrentDir does not exist: %v", err)
	}

	if _, err := os.Stat(cfg.RecipesDir); err != nil {
		t.Errorf("NewTestConfig() RecipesDir does not exist: %v", err)
	}
}

func TestNewTestRecipe(t *testing.T) {
	r := NewTestRecipe("test-tool")

	if r.Metadata.Name != "test-tool" {
		t.Errorf("NewTestRecipe() Name = %q, want %q", r.Metadata.Name, "test-tool")
	}

	if r.Metadata.Description == "" {
		t.Error("NewTestRecipe() Description is empty")
	}

	if len(r.Steps) == 0 {
		t.Error("NewTestRecipe() Steps is empty")
	}

	if r.Verify.Command == "" {
		t.Error("NewTestRecipe() Verify.Command is empty")
	}
}

func TestNewTestRecipeWithDeps(t *testing.T) {
	deps := []string{"dep1", "dep2"}
	r := NewTestRecipeWithDeps("test-tool", deps)

	if r.Metadata.Name != "test-tool" {
		t.Errorf("NewTestRecipeWithDeps() Name = %q, want %q", r.Metadata.Name, "test-tool")
	}

	if len(r.Metadata.Dependencies) != 2 {
		t.Errorf("NewTestRecipeWithDeps() Dependencies count = %d, want 2", len(r.Metadata.Dependencies))
	}

	if r.Metadata.Dependencies[0] != "dep1" {
		t.Errorf("NewTestRecipeWithDeps() Dependencies[0] = %q, want %q", r.Metadata.Dependencies[0], "dep1")
	}
}

func TestFileExists(t *testing.T) {
	// Test with existing file
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if !FileExists(existingFile) {
		t.Error("FileExists() returned false for existing file")
	}

	// Test with non-existing file
	nonExisting := filepath.Join(tmpDir, "not-exists.txt")
	if FileExists(nonExisting) {
		t.Error("FileExists() returned true for non-existing file")
	}
}

func TestAssertFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// This should not fail
	AssertFileExists(t, existingFile)
}

func TestAssertFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	nonExisting := filepath.Join(tmpDir, "not-exists.txt")

	// This should not fail
	AssertFileNotExists(t, nonExisting)
}
