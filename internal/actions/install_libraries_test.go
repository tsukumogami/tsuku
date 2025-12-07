package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestInstallLibrariesAction_Name(t *testing.T) {
	action := &InstallLibrariesAction{}
	if action.Name() != "install_libraries" {
		t.Errorf("expected 'install_libraries', got '%s'", action.Name())
	}
}

func TestInstallLibrariesAction_Execute_Success(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory with test files
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular library file
	libFile := filepath.Join(libDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create context and execute
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*"},
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify file was copied
	destFile := filepath.Join(installDir, "lib", "libyaml.so.2.0.9")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", destFile)
	}
}

func TestInstallLibrariesAction_Execute_PreservesSymlinks(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create the actual library file
	realFile := filepath.Join(libDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(realFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink: libyaml.so.2 -> libyaml.so.2.0.9
	symlinkFile := filepath.Join(libDir, "libyaml.so.2")
	if err := os.Symlink("libyaml.so.2.0.9", symlinkFile); err != nil {
		t.Fatal(err)
	}

	// Create context and execute
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*"},
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify symlink was preserved (not dereferenced)
	destSymlink := filepath.Join(installDir, "lib", "libyaml.so.2")
	info, err := os.Lstat(destSymlink)
	if err != nil {
		t.Fatalf("failed to stat destination symlink: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink to be preserved, but it's a regular file")
	}

	// Verify symlink target is correct
	target, err := os.Readlink(destSymlink)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if target != "libyaml.so.2.0.9" {
		t.Errorf("expected symlink target 'libyaml.so.2.0.9', got '%s'", target)
	}
}

func TestInstallLibrariesAction_Execute_MultiplePatterns(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .so file
	soFile := filepath.Join(libDir, "libyaml.so.2")
	if err := os.WriteFile(soFile, []byte("so content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .dylib file
	dylibFile := filepath.Join(libDir, "libyaml.dylib")
	if err := os.WriteFile(dylibFile, []byte("dylib content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create context and execute
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*", "lib/*.dylib"},
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify both files were copied
	if _, err := os.Stat(filepath.Join(installDir, "lib", "libyaml.so.2")); os.IsNotExist(err) {
		t.Error("expected .so file to be copied")
	}
	if _, err := os.Stat(filepath.Join(installDir, "lib", "libyaml.dylib")); os.IsNotExist(err) {
		t.Error("expected .dylib file to be copied")
	}
}

func TestInstallLibrariesAction_Execute_MissingPatterns(t *testing.T) {
	action := &InstallLibrariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		Recipe:  &recipe.Recipe{},
	}

	params := map[string]interface{}{}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for missing patterns parameter")
	}
}

func TestInstallLibrariesAction_Execute_EmptyPatterns(t *testing.T) {
	action := &InstallLibrariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		Recipe:  &recipe.Recipe{},
	}

	params := map[string]interface{}{
		"patterns": []interface{}{},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for empty patterns list")
	}
}

func TestInstallLibrariesAction_Execute_PathTraversal(t *testing.T) {
	action := &InstallLibrariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		Recipe:  &recipe.Recipe{},
	}

	params := map[string]interface{}{
		"patterns": []interface{}{"../etc/passwd"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for path traversal pattern")
	}
}

func TestInstallLibrariesAction_Execute_AbsolutePath(t *testing.T) {
	action := &InstallLibrariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		Recipe:  &recipe.Recipe{},
	}

	params := map[string]interface{}{
		"patterns": []interface{}{"/etc/passwd"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for absolute path pattern")
	}
}

func TestInstallLibrariesAction_Execute_NoMatches(t *testing.T) {
	workDir := t.TempDir()
	installDir := t.TempDir()

	action := &InstallLibrariesAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.nonexistent"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error when no files match patterns")
	}
}

func TestInstallLibrariesAction_ParsePatterns_InvalidType(t *testing.T) {
	action := &InstallLibrariesAction{}

	_, err := action.parsePatterns("not an array")
	if err == nil {
		t.Error("expected error for non-array patterns")
	}
}

func TestInstallLibrariesAction_ParsePatterns_InvalidElement(t *testing.T) {
	action := &InstallLibrariesAction{}

	_, err := action.parsePatterns([]interface{}{123})
	if err == nil {
		t.Error("expected error for non-string pattern element")
	}
}

func TestInstallLibrariesAction_Execute_SymlinkEscapeBlocked(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a malicious symlink that escapes the install directory
	maliciousSymlink := filepath.Join(libDir, "evil.so")
	if err := os.Symlink("../../../etc/passwd", maliciousSymlink); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for symlink that escapes install directory")
	}
}

func TestInstallLibrariesAction_Execute_AbsoluteSymlinkBlocked(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink with absolute target
	absSymlink := filepath.Join(libDir, "abs.so")
	if err := os.Symlink("/etc/passwd", absSymlink); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for symlink with absolute target")
	}
}

func TestInstallLibrariesAction_Execute_DeduplicatesMatches(t *testing.T) {
	// Create temp directories
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file that will match multiple patterns
	libFile := filepath.Join(libDir, "libtest.so.1")
	if err := os.WriteFile(libFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Recipe:     &recipe.Recipe{},
	}

	action := &InstallLibrariesAction{}
	// Both patterns match the same file
	params := map[string]interface{}{
		"patterns": []interface{}{"lib/*.so*", "lib/libtest*"},
	}

	// Should succeed without error (file only copied once)
	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify file exists
	destFile := filepath.Join(installDir, "lib", "libtest.so.1")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Error("expected file to be copied")
	}
}

func TestInstallLibrariesAction_ValidateSymlinkTarget(t *testing.T) {
	action := &InstallLibrariesAction{}
	workDir := t.TempDir()
	installDir := t.TempDir()

	// Create lib directory in both work and install dirs
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	installLibDir := filepath.Join(installDir, "lib")
	if err := os.MkdirAll(installLibDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		target      string
		shouldError bool
	}{
		{"safe relative same dir", "libyaml.so.2.0.9", false},
		{"safe relative subdir", "subdir/lib.so", false},
		{"escape with dotdot", "../../../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create symlink in workDir
			symlinkPath := filepath.Join(libDir, "test.so")
			os.Remove(symlinkPath) // Clean up from previous iteration
			if err := os.Symlink(tc.target, symlinkPath); err != nil {
				t.Fatal(err)
			}

			destPath := filepath.Join(installLibDir, "test.so")
			err := action.validateSymlinkTarget(symlinkPath, destPath, installDir)

			if tc.shouldError && err == nil {
				t.Errorf("expected error for target %q but got none", tc.target)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("unexpected error for target %q: %v", tc.target, err)
			}
		})
	}
}
