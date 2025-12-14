package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestLinkDependenciesAction_Name(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	if action.Name() != "link_dependencies" {
		t.Errorf("expected 'link_dependencies', got '%s'", action.Name())
	}
}

func TestLinkDependenciesAction_Execute_Success(t *testing.T) {
	t.Parallel()
	// Create directory structure simulating $TSUKU_HOME
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	// Create tool installation directory
	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library directory with test files
	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library file
	libFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create context and execute
	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify symlink was created
	destSymlink := filepath.Join(toolInstallDir, "lib", "libyaml.so.2.0.9")
	info, err := os.Lstat(destSymlink)
	if err != nil {
		t.Fatalf("failed to stat destination: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}

	// Verify symlink target is relative
	target, err := os.Readlink(destSymlink)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}

	// Should be relative path like ../../../libs/libyaml-0.2.5/lib/libyaml.so.2.0.9
	if filepath.IsAbs(target) {
		t.Errorf("expected relative symlink target, got absolute: %s", target)
	}
}

func TestLinkDependenciesAction_Execute_PreservesSymlinkChain(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create actual library file
	realFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(realFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink chain: libyaml.so.2 -> libyaml.so.2.0.9
	symlinkFile := filepath.Join(libVersionDir, "libyaml.so.2")
	if err := os.Symlink("libyaml.so.2.0.9", symlinkFile); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify the symlink chain is preserved in destination
	destSymlink := filepath.Join(toolInstallDir, "lib", "libyaml.so.2")
	target, err := os.Readlink(destSymlink)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}

	// Should preserve the same relative target within lib/
	if target != "libyaml.so.2.0.9" {
		t.Errorf("expected symlink target 'libyaml.so.2.0.9', got '%s'", target)
	}
}

func TestLinkDependenciesAction_Execute_CollisionError(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	toolLibDir := filepath.Join(toolInstallDir, "lib")
	if err := os.MkdirAll(toolLibDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library file in source
	libFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a regular file at destination (collision)
	collisionFile := filepath.Join(toolLibDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(collisionFile, []byte("existing file"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for collision, got none")
	}
}

func TestLinkDependenciesAction_Execute_SkipsExistingCorrectSymlink(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	toolLibDir := filepath.Join(toolInstallDir, "lib")
	if err := os.MkdirAll(toolLibDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library file
	libFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Calculate expected relative path
	relPath, _ := filepath.Rel(toolLibDir, libVersionDir)
	expectedTarget := filepath.Join(relPath, "libyaml.so.2.0.9")

	// Create correct symlink at destination
	destSymlink := filepath.Join(toolLibDir, "libyaml.so.2.0.9")
	if err := os.Symlink(expectedTarget, destSymlink); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	// Should succeed without error (skips existing correct symlink)
	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestLinkDependenciesAction_Execute_MissingLibraryDir(t *testing.T) {
	t.Parallel()
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Don't create libs directory - it should fail

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for missing library directory")
	}
}

func TestLinkDependenciesAction_Execute_MissingParameters(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		Recipe:   &recipe.Recipe{},
		ToolsDir: "/nonexistent/tools", // No libs dir, so version discovery will fail
	}

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
	}{
		{"missing library", map[string]interface{}{"version": "0.2.5"}, true},
		// Version is now optional - when missing, it tries to discover from libs dir
		// With a nonexistent ToolsDir, discovery will fail, so we still expect an error
		{"missing version (discovery fails)", map[string]interface{}{"library": "libyaml"}, true},
		{"empty params", map[string]interface{}{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := action.Execute(ctx, tc.params)
			if tc.expectError && err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

func TestLinkDependenciesAction_ValidateLibraryName(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}

	tests := []struct {
		name        string
		library     string
		shouldError bool
	}{
		{"valid name", "libyaml", false},
		{"valid with hyphen", "lib-yaml", false},
		{"valid with number", "openssl3", false},
		{"empty", "", true},
		{"path traversal", "../etc", true},
		{"contains slash", "lib/yaml", true},
		{"contains backslash", "lib\\yaml", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := action.validateLibraryName(tc.library)
			if tc.shouldError && err == nil {
				t.Errorf("expected error for %q", tc.library)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.library, err)
			}
		})
	}
}

func TestLinkDependenciesAction_ValidateVersion(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}

	tests := []struct {
		name        string
		version     string
		shouldError bool
	}{
		{"valid semver", "0.2.5", false},
		{"valid with v prefix", "v1.0.0", false},
		{"valid prerelease", "1.0.0-beta.1", false},
		{"empty", "", true},
		{"path traversal", "../etc", true},
		{"contains slash", "1.0/0", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := action.validateVersion(tc.version)
			if tc.shouldError && err == nil {
				t.Errorf("expected error for %q", tc.version)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.version, err)
			}
		})
	}
}

func TestLinkDependenciesAction_ValidateSymlinkTarget(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}

	tests := []struct {
		name        string
		target      string
		shouldError bool
	}{
		{"valid same directory", "libyaml.so.2.0.9", false},
		{"valid with extension", "libfoo.so.1.2.3", false},
		{"absolute path", "/etc/passwd", true},
		{"path traversal simple", "../evil", true},
		{"path traversal deep", "../../../etc/passwd", true},
		{"path traversal middle", "foo/../../../etc", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := action.validateSymlinkTarget(tc.target, "test.so")
			if tc.shouldError && err == nil {
				t.Errorf("expected error for target %q", tc.target)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("unexpected error for target %q: %v", tc.target, err)
			}
		})
	}
}

func TestLinkDependenciesAction_Execute_MaliciousSymlinkBlocked(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a malicious symlink that tries to escape
	maliciousSymlink := filepath.Join(libVersionDir, "evil.so")
	if err := os.Symlink("../../../etc/passwd", maliciousSymlink); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for malicious symlink target, got none")
	}
}

func TestLinkDependenciesAction_Execute_AbsoluteSymlinkBlocked(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink with absolute target
	absSymlink := filepath.Join(libVersionDir, "abs.so")
	if err := os.Symlink("/etc/passwd", absSymlink); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for absolute symlink target, got none")
	}
}

func TestLinkDependenciesAction_DiscoverLibraryVersion(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	// Create tools and libs directories
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library directory
	if err := os.MkdirAll(filepath.Join(libsDir, "libyaml-0.2.5"), 0755); err != nil {
		t.Fatal(err)
	}

	// Test discovery
	version, err := action.discoverLibraryVersion(toolsDir, "libyaml")
	if err != nil {
		t.Fatalf("discoverLibraryVersion failed: %v", err)
	}
	if version != "0.2.5" {
		t.Errorf("got version %q, want %q", version, "0.2.5")
	}
}

func TestLinkDependenciesAction_DiscoverLibraryVersion_NotFound(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	// Create tools and libs directories (but no library)
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test discovery fails for missing library
	_, err := action.discoverLibraryVersion(toolsDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent library")
	}
}

func TestLinkDependenciesAction_DiscoverLibraryVersion_NoLibsDir(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")

	// Create only tools directory (no libs)
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test discovery fails when libs dir doesn't exist
	_, err := action.discoverLibraryVersion(toolsDir, "libyaml")
	if err == nil {
		t.Error("expected error when libs directory doesn't exist")
	}
}

func TestLinkDependenciesAction_DiscoverLibraryVersion_MultipleVersions(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	// Create tools and libs directories
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple versions of the same library
	if err := os.MkdirAll(filepath.Join(libsDir, "libyaml-0.2.4"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libsDir, "libyaml-0.2.5"), 0755); err != nil {
		t.Fatal(err)
	}

	// Test discovery returns one of them (with warning)
	version, err := action.discoverLibraryVersion(toolsDir, "libyaml")
	if err != nil {
		t.Fatalf("discoverLibraryVersion failed: %v", err)
	}
	if version != "0.2.4" && version != "0.2.5" {
		t.Errorf("got unexpected version %q", version)
	}
}

func TestLinkDependenciesAction_Execute_NoWorkDirOrToolInstallDir(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library file
	libFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Context with neither WorkDir nor ToolInstallDir
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Recipe:   &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error when neither WorkDir nor ToolInstallDir is set")
	}
}

func TestLinkDependenciesAction_Execute_WithWorkDir(t *testing.T) {
	t.Parallel()
	// Create directory structure simulating pre-install scenario
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")
	workDir := t.TempDir()

	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library directory with test files
	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create library file
	libFile := filepath.Join(libVersionDir, "libyaml.so.2.0.9")
	if err := os.WriteFile(libFile, []byte("library content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create context with WorkDir (pre-install scenario)
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		WorkDir:  workDir,
		Version:  "3.4.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "ruby",
			},
		},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify symlink was created in WorkDir
	destSymlink := filepath.Join(workDir, "lib", "libyaml.so.2.0.9")
	info, err := os.Lstat(destSymlink)
	if err != nil {
		t.Fatalf("failed to stat destination: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}
}

func TestLinkDependenciesAction_Execute_EmptyLibraryDir(t *testing.T) {
	t.Parallel()
	// Create directory structure
	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	libsDir := filepath.Join(tsukuHome, "libs")

	toolInstallDir := filepath.Join(toolsDir, "ruby-3.4.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create EMPTY library directory
	libVersionDir := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	if err := os.MkdirAll(libVersionDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: toolInstallDir,
		Recipe:         &recipe.Recipe{},
	}

	action := &LinkDependenciesAction{}
	params := map[string]interface{}{
		"library": "libyaml",
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for empty library directory")
	}
}

func TestLinkDependenciesAction_Execute_InvalidLibraryType(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolInstallDir: t.TempDir(),
		ToolsDir:       t.TempDir(),
		Recipe:         &recipe.Recipe{},
	}

	// Test with non-string library parameter
	params := map[string]interface{}{
		"library": 123, // Should be string
		"version": "0.2.5",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for non-string library parameter")
	}
}

func TestLinkDependenciesAction_Execute_InvalidVersionType(t *testing.T) {
	t.Parallel()
	action := &LinkDependenciesAction{}
	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolInstallDir: t.TempDir(),
		ToolsDir:       t.TempDir(),
		Recipe:         &recipe.Recipe{},
	}

	// Test with non-string version parameter
	params := map[string]interface{}{
		"library": "libyaml",
		"version": 123, // Should be string
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("expected error for non-string version parameter")
	}
}
