package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestValidateBinaryPath tests path traversal security validation
func TestValidateBinaryPath(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	tests := []struct {
		name      string
		path      string
		shouldErr bool
	}{
		{
			name:      "valid relative path",
			path:      "bin/java",
			shouldErr: false,
		},
		{
			name:      "path traversal with ..",
			path:      "../../../etc/passwd",
			shouldErr: true,
		},
		{
			name:      "path with .. in middle",
			path:      "bin/../lib/java",
			shouldErr: true,
		},
		{
			name:      "absolute path",
			path:      "/usr/bin/java",
			shouldErr: true,
		},
		{
			name:      "complex relative path",
			path:      "foo/bar/baz/binary",
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.validateBinaryPath(tt.path)
			if tt.shouldErr && err == nil {
				t.Errorf("expected error for path %q, got nil", tt.path)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error for path %q, got %v", tt.path, err)
			}
		})
	}
}

// TestCreateSymlink tests symlink creation with relative paths
func TestCreateSymlink(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	// Create temp directories
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "tools", "liberica-25.0.1", "bin")
	linkDir := filepath.Join(tmpDir, "tools", ".install", "bin")

	// Create target file
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	targetFile := filepath.Join(targetDir, "java")
	if err := os.WriteFile(targetFile, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	// Create symlink
	linkFile := filepath.Join(linkDir, "java")
	if err := action.createSymlink(targetFile, linkFile); err != nil {
		t.Fatalf("createSymlink failed: %v", err)
	}

	// Verify symlink exists
	info, err := os.Lstat(linkFile)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink, got regular file")
	}

	// Verify symlink target is relative
	target, err := os.Readlink(linkFile)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}

	if filepath.IsAbs(target) {
		t.Errorf("expected relative symlink, got absolute: %s", target)
	}

	// Verify symlink resolves to correct file
	resolved, err := filepath.EvalSymlinks(linkFile)
	if err != nil {
		t.Fatalf("failed to resolve symlink: %v", err)
	}

	if resolved != targetFile {
		t.Errorf("symlink resolves to %s, expected %s", resolved, targetFile)
	}
}

// TestInstallDirectoryWithSymlinks tests the full directory installation flow
func TestInstallDirectoryWithSymlinks(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	// Create temp directories
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")
	installDir := filepath.Join(tmpDir, ".install")

	// Create work directory with mock JDK structure
	binDir := filepath.Join(workDir, "bin")
	libDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Create mock binaries
	javaFile := filepath.Join(binDir, "java")
	javacFile := filepath.Join(binDir, "javac")
	if err := os.WriteFile(javaFile, []byte("#!/bin/sh\necho java"), 0755); err != nil {
		t.Fatalf("failed to create java file: %v", err)
	}
	if err := os.WriteFile(javacFile, []byte("#!/bin/sh\necho javac"), 0755); err != nil {
		t.Fatalf("failed to create javac file: %v", err)
	}

	// Create mock library
	libFile := filepath.Join(libDir, "libjli.so")
	if err := os.WriteFile(libFile, []byte("mock library"), 0644); err != nil {
		t.Fatalf("failed to create lib file: %v", err)
	}

	// Create execution context with verification (required for directory mode)
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Version:    "25.0.1",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "liberica",
			},
			Verify: recipe.VerifySection{
				Command: "java --version",
			},
		},
	}

	// Create binaries list
	binaries := []recipe.BinaryMapping{
		{Src: "bin/java", Dest: "bin/java"},
		{Src: "bin/javac", Dest: "bin/javac"},
	}

	// Execute directory installation
	if err := action.installDirectoryWithSymlinks(ctx, binaries); err != nil {
		t.Fatalf("installDirectoryWithSymlinks failed: %v", err)
	}

	// Verify directory tree was copied to .install
	copiedJava := filepath.Join(installDir, "bin", "java")
	if _, err := os.Stat(copiedJava); err != nil {
		t.Errorf("java binary not copied: %v", err)
	}

	copiedLib := filepath.Join(installDir, "lib", "libjli.so")
	if _, err := os.Stat(copiedLib); err != nil {
		t.Errorf("library not copied: %v", err)
	}
}

// TestInstallDirectoryWithSymlinks_AtomicRollback tests cleanup behavior on failure
func TestInstallDirectoryWithSymlinks_AtomicRollback(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	// Create temp directories
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")
	installDir := filepath.Join(tmpDir, ".install")

	// Create work directory with mock binary
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	javaFile := filepath.Join(binDir, "java")
	if err := os.WriteFile(javaFile, []byte("#!/bin/sh\necho java"), 0755); err != nil {
		t.Fatalf("failed to create java file: %v", err)
	}

	// Create execution context
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Version:    "25.0.1",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "liberica",
			},
		},
	}

	// Create binaries list with invalid path (contains ..)
	binaries := []recipe.BinaryMapping{
		{Src: "bin/java", Dest: "bin/java"},
		{Src: "../../../etc/passwd", Dest: "bin/bad"}, // This will fail validation
	}

	// Execute directory installation (should fail due to security validation)
	err := action.installDirectoryWithSymlinks(ctx, binaries)
	if err == nil {
		t.Fatal("expected error for path traversal attempt, got nil")
	}

	// Verify .install directory was not created (or is empty)
	if _, err := os.Stat(installDir); err == nil {
		// If it exists, it should be empty (only created but copy failed)
		entries, _ := os.ReadDir(installDir)
		if len(entries) > 0 {
			t.Errorf("install directory should be empty after security validation failure, got %d entries", len(entries))
		}
	}
}

// TestInstallBinaries_ModeRouting tests that install_mode parameter routes to correct implementation
// Note: directory modes require verification to be set
func TestInstallBinaries_ModeRouting(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	tests := []struct {
		name        string
		installMode string
		hasVerify   bool
		shouldErr   bool
		errContains string
	}{
		{
			name:        "binaries mode (default)",
			installMode: "",
			hasVerify:   false,
			shouldErr:   false,
		},
		{
			name:        "binaries mode (explicit)",
			installMode: "binaries",
			hasVerify:   false,
			shouldErr:   false,
		},
		{
			name:        "directory mode with verify",
			installMode: "directory",
			hasVerify:   true,
			shouldErr:   false, // Should succeed - just copies directory tree
		},
		{
			name:        "directory_wrapped mode with verify",
			installMode: "directory_wrapped",
			hasVerify:   true,
			shouldErr:   true,
			errContains: "not yet implemented",
		},
		{
			name:        "invalid mode",
			installMode: "invalid",
			hasVerify:   false,
			shouldErr:   true,
			errContains: "invalid install_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			workDir := filepath.Join(tmpDir, "work")
			installDir := filepath.Join(tmpDir, ".install")

			// Create work directory with mock binary
			binDir := filepath.Join(workDir, "bin")
			if err := os.MkdirAll(binDir, 0755); err != nil {
				t.Fatalf("failed to create bin dir: %v", err)
			}

			testFile := filepath.Join(binDir, "test")
			if err := os.WriteFile(testFile, []byte("test"), 0755); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			ctx := &ExecutionContext{
				Context:    context.Background(),
				WorkDir:    workDir,
				InstallDir: installDir,
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{
						Name: "test-tool",
					},
					Verify: recipe.VerifySection{
						Command: "",
					},
				},
			}

			// Set verification command if test requires it
			if tt.hasVerify {
				ctx.Recipe.Verify.Command = "test-tool --version"
			}

			params := map[string]interface{}{
				"binaries": []interface{}{"bin/test"},
			}

			if tt.installMode != "" {
				params["install_mode"] = tt.installMode
			}

			err := action.Execute(ctx, params)

			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if tt.shouldErr && tt.errContains != "" && !contains(err.Error(), tt.errContains) {
				t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
			}
		})
	}
}

// TestInstallBinaries_VerificationEnforcement tests that directory mode requires verification
// This is the defense-in-depth check that prevents bypassing composite action verification
func TestInstallBinaries_VerificationEnforcement(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	tests := []struct {
		name        string
		installMode string
		hasVerify   bool
		shouldErr   bool
		errContains string
	}{
		{
			name:        "binaries mode without verify (allowed)",
			installMode: "binaries",
			hasVerify:   false,
			shouldErr:   false,
		},
		{
			name:        "binaries mode with verify (allowed)",
			installMode: "binaries",
			hasVerify:   true,
			shouldErr:   false,
		},
		{
			name:        "directory mode without verify (blocked)",
			installMode: "directory",
			hasVerify:   false,
			shouldErr:   true,
			errContains: "must include a [verify] section",
		},
		{
			name:        "directory mode with verify (allowed)",
			installMode: "directory",
			hasVerify:   true,
			shouldErr:   false,
		},
		{
			name:        "directory_wrapped mode without verify (blocked)",
			installMode: "directory_wrapped",
			hasVerify:   false,
			shouldErr:   true,
			errContains: "must include a [verify] section",
		},
		{
			name:        "directory_wrapped mode with verify (blocked by not implemented)",
			installMode: "directory_wrapped",
			hasVerify:   true,
			shouldErr:   true,
			errContains: "not yet implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			workDir := filepath.Join(tmpDir, "work")
			installDir := filepath.Join(tmpDir, ".install")

			// Create work directory with mock binary
			binDir := filepath.Join(workDir, "bin")
			if err := os.MkdirAll(binDir, 0755); err != nil {
				t.Fatalf("failed to create bin dir: %v", err)
			}

			testFile := filepath.Join(binDir, "test")
			if err := os.WriteFile(testFile, []byte("test"), 0755); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Create context with recipe
			ctx := &ExecutionContext{
				Context:    context.Background(),
				WorkDir:    workDir,
				InstallDir: installDir,
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{
						Name: "test-tool",
					},
					Verify: recipe.VerifySection{
						Command: "",
					},
				},
			}

			// Set verification command if test requires it
			if tt.hasVerify {
				ctx.Recipe.Verify.Command = "test-tool --version"
			}

			// Create params with install_mode
			params := map[string]interface{}{
				"binaries":     []interface{}{"bin/test"},
				"install_mode": tt.installMode,
			}

			// Execute action
			err := action.Execute(ctx, params)

			// Check if error matches expectation
			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if tt.shouldErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

// TestInstallBinaries_SecurityValidation tests that binaries mode blocks path traversal attacks
// This test verifies the fix for Issue #90 - security validation must apply to binaries mode
func TestInstallBinaries_SecurityValidation(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	tests := []struct {
		name        string
		binaries    []recipe.BinaryMapping
		shouldErr   bool
		errContains string
	}{
		{
			name: "valid relative paths",
			binaries: []recipe.BinaryMapping{
				{Src: "bin/kubectl", Dest: "bin/kubectl"},
				{Src: "dist/sam", Dest: "bin/sam"},
			},
			shouldErr: false,
		},
		{
			name: "path traversal with ..",
			binaries: []recipe.BinaryMapping{
				{Src: "../../../etc/passwd", Dest: "bin/bad"},
			},
			shouldErr:   true,
			errContains: "cannot contain '..'",
		},
		{
			name: "path with .. in middle",
			binaries: []recipe.BinaryMapping{
				{Src: "bin/../lib/evil", Dest: "bin/evil"},
			},
			shouldErr:   true,
			errContains: "cannot contain '..'",
		},
		{
			name: "absolute path",
			binaries: []recipe.BinaryMapping{
				{Src: "/usr/bin/evil", Dest: "bin/evil"},
			},
			shouldErr:   true,
			errContains: "must be relative",
		},
		{
			name: "mixed valid and invalid",
			binaries: []recipe.BinaryMapping{
				{Src: "bin/good", Dest: "bin/good"},
				{Src: "../../etc/passwd", Dest: "bin/bad"}, // Should fail on this one
			},
			shouldErr:   true,
			errContains: "cannot contain '..'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			workDir := filepath.Join(tmpDir, "work")
			installDir := filepath.Join(tmpDir, ".install")

			// Create work directory with mock files
			binDir := filepath.Join(workDir, "bin")
			distDir := filepath.Join(workDir, "dist")
			if err := os.MkdirAll(binDir, 0755); err != nil {
				t.Fatalf("failed to create bin dir: %v", err)
			}
			if err := os.MkdirAll(distDir, 0755); err != nil {
				t.Fatalf("failed to create dist dir: %v", err)
			}

			// Create mock binary files
			for _, binary := range tt.binaries {
				// Create file in expected location (ignore invalid paths)
				if !strings.Contains(binary.Src, "..") && !filepath.IsAbs(binary.Src) {
					filePath := filepath.Join(workDir, binary.Src)
					if err := os.WriteFile(filePath, []byte("test"), 0755); err != nil {
						// Skip if can't create (e.g., deep path)
						continue
					}
				}
			}

			ctx := &ExecutionContext{
				Context:    context.Background(),
				WorkDir:    workDir,
				InstallDir: installDir,
				Version:    "1.0.0",
			}

			// Execute binaries installation
			err := action.installBinaries(ctx, tt.binaries)

			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if tt.shouldErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			}

			// If error expected, verify nothing was installed
			if tt.shouldErr {
				if _, err := os.Stat(installDir); err == nil {
					entries, _ := os.ReadDir(installDir)
					if len(entries) > 0 {
						t.Errorf("install directory should be empty after security validation failure, got %d entries", len(entries))
					}
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParseBinaries tests the binaries parameter parsing
func TestParseBinaries(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	tests := []struct {
		name        string
		input       interface{}
		expectCount int
		shouldErr   bool
	}{
		{
			name:        "string slice",
			input:       []interface{}{"bin/java", "bin/javac"},
			expectCount: 2,
			shouldErr:   false,
		},
		{
			name: "map with src and dest",
			input: []interface{}{
				map[string]interface{}{"src": "bin/java", "dest": "bin/java"},
				map[string]interface{}{"src": "bin/javac", "dest": "bin/javac"},
			},
			expectCount: 2,
			shouldErr:   false,
		},
		{
			name:        "empty slice",
			input:       []interface{}{},
			expectCount: 0,
			shouldErr:   false,
		},
		{
			name:      "invalid type",
			input:     "not an array",
			shouldErr: true,
		},
		{
			name:      "invalid array item type",
			input:     []interface{}{123},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := action.parseBinaries(tt.input)
			if tt.shouldErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(result) != tt.expectCount {
				t.Errorf("expected %d binaries, got %d", tt.expectCount, len(result))
			}
		})
	}
}

// TestInstallBinariesAction_Name tests the Name method
func TestInstallBinariesAction_Name(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	if action.Name() != "install_binaries" {
		t.Errorf("Name() = %q, want %q", action.Name(), "install_binaries")
	}
}

// TestInstallBinariesAction_Execute_MissingBinaries tests missing parameter
func TestInstallBinariesAction_Execute_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'binaries' parameter is missing")
	}
}
