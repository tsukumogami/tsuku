package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChmodAction_Name(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	if action.Name() != "chmod" {
		t.Errorf("Name() = %q, want %q", action.Name(), "chmod")
	}
}

func TestChmodAction_Execute(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tmpDir, "script1.sh")
	file2 := filepath.Join(tmpDir, "script2.sh")
	if err := os.WriteFile(file1, []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"files": []interface{}{"script1.sh", "script2.sh"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify permissions
	info1, _ := os.Stat(file1)
	info2, _ := os.Stat(file2)
	if info1.Mode().Perm() != 0755 {
		t.Errorf("script1.sh mode = %o, want 0755", info1.Mode().Perm())
	}
	if info2.Mode().Perm() != 0755 {
		t.Errorf("script2.sh mode = %o, want 0755", info2.Mode().Perm())
	}
}

func TestChmodAction_Execute_CustomMode(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "readonly.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"files": []interface{}{"readonly.txt"},
		"mode":  "0644",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0644 {
		t.Errorf("file mode = %o, want 0644", info.Mode().Perm())
	}
}

func TestChmodAction_Execute_MissingFiles(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'files' parameter is missing")
	}
}

func TestChmodAction_Execute_InvalidMode(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"files": []interface{}{"test.txt"},
		"mode":  "invalid",
	})
	if err == nil {
		t.Error("Execute() should fail with invalid mode")
	}
}

func TestChmodAction_Execute_NonExistentFile(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"files": []interface{}{"nonexistent.sh"},
	})
	if err == nil {
		t.Error("Execute() should fail for non-existent file")
	}
}

func TestChmodAction_Execute_VariableExpansion(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "tool-1.0.0.sh")
	if err := os.WriteFile(testFile, []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"files": []interface{}{"tool-{version}.sh"},
	})
	if err != nil {
		t.Fatalf("Execute() with variable expansion error = %v", err)
	}

	info, _ := os.Stat(testFile)
	if info.Mode().Perm() != 0755 {
		t.Errorf("file mode = %o, want 0755", info.Mode().Perm())
	}
}
