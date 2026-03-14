package actions

import (
	"context"
	"testing"

	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestGitHubArchiveAction_VerificationEnforcement tests that directory mode requires verification
func TestGitHubArchiveAction_VerificationEnforcement(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

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
			name:        "directory_wrapped mode with verify (allowed)",
			installMode: "directory_wrapped",
			hasVerify:   true,
			shouldErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create context with recipe
			ctx := &ExecutionContext{
				Context: context.Background(),
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{
						Name: "test-tool",
					},
					Verify: &recipe.VerifySection{
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
				"repo":           "test/repo",
				"asset_pattern":  "test-{version}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"test-tool"},
				"install_mode":   tt.installMode,
			}

			// Execute action (will fail early due to verification enforcement)
			err := action.Execute(ctx, params)

			// Check if error matches expectation
			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				// For allowed cases, error might occur later (e.g., download failure)
				// Only fail if it's the verification error
				if tt.errContains != "" && contains(err.Error(), tt.errContains) {
					t.Errorf("unexpected verification error: %v", err)
				}
			}

			if tt.shouldErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

func TestCompositeAction_Name(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action Action
		want   string
	}{
		{&DownloadArchiveAction{}, "download_archive"},
		{&GitHubArchiveAction{}, "github_archive"},
		{&GitHubFileAction{}, "github_file"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.action.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// HomebrewAction tests moved to homebrew_test.go

func TestExtractSourceFiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: 0,
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: 0,
		},
		{
			name:     "string array",
			input:    []interface{}{"file1", "file2", "file3"},
			expected: 3,
		},
		{
			name: "map array with src",
			input: []interface{}{
				map[string]interface{}{"src": "source1", "dest": "dest1"},
				map[string]interface{}{"src": "source2", "dest": "dest2"},
			},
			expected: 2,
		},
		{
			name: "mixed array",
			input: []interface{}{
				"simple_file",
				map[string]interface{}{"src": "source1", "dest": "dest1"},
			},
			expected: 2,
		},
		{
			name: "map without src",
			input: []interface{}{
				map[string]interface{}{"dest": "dest1"},
			},
			expected: 0,
		},
		{
			name:     "non-array input",
			input:    "string",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSourceFiles(tt.input)
			if len(result) != tt.expected {
				t.Errorf("extractSourceFiles() returned %d items, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestDownloadArchiveAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing url",
			params: map[string]interface{}{},
		},
		{
			name: "missing archive_format with undetectable URL",
			params: map[string]interface{}{
				"url": "https://example.com/download?file=tool",
			},
		},
		{
			name: "missing binaries",
			params: map[string]interface{}{
				"url":            "https://example.com/file.tar.gz",
				"archive_format": "tar.gz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should fail with missing required params")
			}
		})
	}
}

func TestGitHubArchiveAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing repo",
			params: map[string]interface{}{},
		},
		{
			name: "missing asset_pattern",
			params: map[string]interface{}{
				"repo": "owner/repo",
			},
		},
		{
			name: "missing archive_format with undetectable pattern",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}",
			},
		},
		{
			name: "missing binaries",
			params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "file-{version}.tar.gz",
				"archive_format": "tar.gz",
			},
		},
		{
			name: "invalid install_mode",
			params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "file-{version}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"bin"},
				"install_mode":   "invalid_mode",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should fail with missing/invalid params")
			}
		})
	}
}

func TestGitHubFileAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing repo",
			params: map[string]interface{}{},
		},
		{
			name: "missing asset_pattern",
			params: map[string]interface{}{
				"repo": "owner/repo",
			},
		},
		{
			name: "missing binary/binaries",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}",
			},
		},
		{
			name: "binaries with missing src",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}",
				"binaries":      []interface{}{map[string]interface{}{"dest": "output"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should fail with missing required params")
			}
		})
	}
}

// HomebrewAction Execute tests moved to homebrew_test.go

// TestGitHubArchiveAction_Decompose tests the Decompose method
func TestGitHubArchiveAction_Decompose(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	// Create basic context
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     nil,
		Resolver:   nil,
		Downloader: nil, // No downloader means no checksum computation
	}

	// Create params
	params := map[string]interface{}{
		"repo":           "owner/repo",
		"asset_pattern":  "tool-{version}-{os}-{arch}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
		"strip_dirs":     1,
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Should return 4 primitive steps
	if len(steps) != 4 {
		t.Fatalf("Decompose() returned %d steps, want 4", len(steps))
	}

	// Step 1: download_file
	if steps[0].Action != "download_file" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download_file")
	}
	expectedURL := "https://github.com/owner/repo/releases/download/v1.0.0/tool-1.0.0-linux-amd64.tar.gz"
	if url, ok := steps[0].Params["url"].(string); !ok || url != expectedURL {
		t.Errorf("steps[0].Params[url] = %q, want %q", url, expectedURL)
	}
	if dest, ok := steps[0].Params["dest"].(string); !ok || dest != "tool-1.0.0-linux-amd64.tar.gz" {
		t.Errorf("steps[0].Params[dest] = %q, want %q", dest, "tool-1.0.0-linux-amd64.tar.gz")
	}

	// Step 2: extract
	if steps[1].Action != "extract" {
		t.Errorf("steps[1].Action = %q, want %q", steps[1].Action, "extract")
	}
	if format, ok := steps[1].Params["format"].(string); !ok || format != "tar.gz" {
		t.Errorf("steps[1].Params[format] = %q, want %q", format, "tar.gz")
	}
	if stripDirs, ok := steps[1].Params["strip_dirs"].(int); !ok || stripDirs != 1 {
		t.Errorf("steps[1].Params[strip_dirs] = %v, want 1", stripDirs)
	}

	// Step 3: chmod
	if steps[2].Action != "chmod" {
		t.Errorf("steps[2].Action = %q, want %q", steps[2].Action, "chmod")
	}

	// Step 4: install_binaries
	if steps[3].Action != "install_binaries" {
		t.Errorf("steps[3].Action = %q, want %q", steps[3].Action, "install_binaries")
	}
}

func TestGitHubArchiveAction_Decompose_MissingParams(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing repo",
			params: map[string]interface{}{},
		},
		{
			name: "missing asset_pattern",
			params: map[string]interface{}{
				"repo": "owner/repo",
			},
		},
		{
			name: "missing archive_format with undetectable pattern",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}",
			},
		},
		{
			name: "missing binaries",
			params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "file-{version}.tar.gz",
				"archive_format": "tar.gz",
			},
		},
		{
			name: "invalid install_mode",
			params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "file-{version}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"bin"},
				"install_mode":   "invalid_mode",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.Decompose(ctx, tt.params)
			if err == nil {
				t.Error("Decompose() should fail with missing/invalid params")
			}
		})
	}
}

func TestGitHubArchiveAction_Decompose_OSArchMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "darwin",
		Arch:       "arm64",
	}

	params := map[string]interface{}{
		"repo":           "owner/repo",
		"asset_pattern":  "tool-{version}-{os}-{arch}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
		"os_mapping": map[string]interface{}{
			"darwin": "macos",
		},
		"arch_mapping": map[string]interface{}{
			"arm64": "aarch64",
		},
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Check that URL uses mapped values
	expectedURL := "https://github.com/owner/repo/releases/download/v1.0.0/tool-1.0.0-macos-aarch64.tar.gz"
	if url, ok := steps[0].Params["url"].(string); !ok || url != expectedURL {
		t.Errorf("steps[0].Params[url] = %q, want %q", url, expectedURL)
	}
}

func TestGitHubArchiveAction_Decompose_InstallMode(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	params := map[string]interface{}{
		"repo":           "owner/repo",
		"asset_pattern":  "tool-{version}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
		"install_mode":   "directory",
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Check that install_mode is passed to install_binaries step
	if mode, ok := steps[3].Params["install_mode"].(string); !ok || mode != "directory" {
		t.Errorf("steps[3].Params[install_mode] = %q, want %q", mode, "directory")
	}
}

func TestGitHubArchiveAction_Decompose_AllStepsArePrimitives(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	params := map[string]interface{}{
		"repo":           "owner/repo",
		"asset_pattern":  "tool-{version}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Verify all returned steps are primitives
	for i, step := range steps {
		if !IsPrimitive(step.Action) {
			t.Errorf("steps[%d].Action = %q is not a primitive", i, step.Action)
		}
	}
}

func TestGitHubArchiveAction_Decompose_BinariesFormats(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	t.Run("simple string binaries", func(t *testing.T) {
		params := map[string]interface{}{
			"repo":           "owner/repo",
			"asset_pattern":  "tool.tar.gz",
			"archive_format": "tar.gz",
			"binaries":       []interface{}{"bin1", "bin2"},
		}

		steps, err := action.Decompose(ctx, params)
		if err != nil {
			t.Fatalf("Decompose() error = %v", err)
		}

		// Check chmod step has correct files
		files, ok := steps[2].Params["files"].([]interface{})
		if !ok || len(files) != 2 {
			t.Errorf("chmod files = %v, want 2 files", files)
		}
	})

	t.Run("src/dest map binaries", func(t *testing.T) {
		params := map[string]interface{}{
			"repo":           "owner/repo",
			"asset_pattern":  "tool.tar.gz",
			"archive_format": "tar.gz",
			"binaries": []interface{}{
				map[string]interface{}{"src": "source1", "dest": "dest1"},
				map[string]interface{}{"src": "source2", "dest": "dest2"},
			},
		}

		steps, err := action.Decompose(ctx, params)
		if err != nil {
			t.Fatalf("Decompose() error = %v", err)
		}

		// Check chmod step extracts src correctly
		files, ok := steps[2].Params["files"].([]interface{})
		if !ok || len(files) != 2 {
			t.Errorf("chmod files = %v, want 2 files", files)
		}
	})
}

// TestDownloadArchiveAction_VerificationEnforcement tests that directory mode requires verification
func TestDownloadArchiveAction_VerificationEnforcement(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}

	tests := []struct {
		name        string
		installMode string // Optional parameter
		hasVerify   bool
		shouldErr   bool
		errContains string
	}{
		{
			name:        "no install_mode without verify (allowed)",
			installMode: "",
			hasVerify:   false,
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
			name:        "directory_wrapped mode with verify (allowed)",
			installMode: "directory_wrapped",
			hasVerify:   true,
			shouldErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create context with recipe
			ctx := &ExecutionContext{
				Context: context.Background(),
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{
						Name: "test-tool",
					},
					Verify: &recipe.VerifySection{
						Command: "",
					},
				},
			}

			// Set verification command if test requires it
			if tt.hasVerify {
				ctx.Recipe.Verify.Command = "test-tool --version"
			}

			// Create params
			params := map[string]interface{}{
				"url":            "https://example.com/test.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"test-tool"},
			}

			// Add install_mode if specified
			if tt.installMode != "" {
				params["install_mode"] = tt.installMode
			}

			// Execute action (will fail early due to verification enforcement)
			err := action.Execute(ctx, params)

			// Check if error matches expectation
			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldErr && err != nil {
				// For allowed cases, error might occur later (e.g., download failure)
				// Only fail if it's the verification error
				if tt.errContains != "" && contains(err.Error(), tt.errContains) {
					t.Errorf("unexpected verification error: %v", err)
				}
			}

			if tt.shouldErr && tt.errContains != "" {
				if err == nil || !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

// -- composites.go: GitHubArchiveAction.Preflight additional warning paths --

func TestGitHubArchiveAction_Preflight_ValidMinimal(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
	})
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v", result.Errors)
	}
}

func TestGitHubArchiveAction_Preflight_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"asset_pattern": "tool-{version}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubFileAction_Preflight_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":   "owner/repo",
		"binary": "tool",
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubFileAction_Preflight_ValidMinimal(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
	})
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v", result.Errors)
	}
}

// -- composites.go: extractSourceFiles helper --

func TestExtractSourceFiles_StringSlice(t *testing.T) {
	t.Parallel()
	result := extractSourceFiles([]any{"bin/tool1", "bin/tool2"})
	if len(result) != 2 {
		t.Errorf("extractSourceFiles() returned %d files, want 2", len(result))
	}
}

func TestExtractSourceFiles_MapSlice(t *testing.T) {
	t.Parallel()
	result := extractSourceFiles([]any{
		map[string]any{"src": "build/tool", "dest": "bin/tool"},
	})
	if len(result) != 1 || result[0] != "build/tool" {
		t.Errorf("extractSourceFiles() = %v, want [build/tool]", result)
	}
}

// -- composites.go: DownloadArchiveAction Decompose with specific params --

func TestDownloadArchiveAction_Decompose_WithOSMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "darwin",
		Arch:       "arm64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"url":        "https://nonexistent.invalid/tool-{version}-{os}-{arch}.tar.gz",
		"binaries":   []any{"bin/tool"},
		"os_mapping": map[string]any{"darwin": "macOS"},
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) == 0 {
		t.Error("Decompose() returned 0 steps")
	}
}

func TestDownloadArchiveAction_Decompose_WithArchMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"url":          "https://nonexistent.invalid/tool-{version}-{os}-{arch}.tar.gz",
		"binaries":     []any{"bin/tool"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) == 0 {
		t.Error("Decompose() returned 0 steps")
	}
}

// -- composites.go: GitHubArchiveAction.Execute additional validation paths --

func TestGitHubArchiveAction_Execute_CannotDetectFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
	})
	if err == nil || !strings.Contains(err.Error(), "archive format") {
		t.Errorf("Expected archive format error, got %v", err)
	}
}

func TestGitHubArchiveAction_Execute_DirectoryModeWithoutVerify(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}.tar.gz",
		"binaries":      []any{"tool"},
		"install_mode":  "directory",
	})
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Errorf("Expected verify error, got %v", err)
	}
}

// -- composites.go: DownloadArchiveAction.Execute with explicit dest --

func TestDownloadArchiveAction_Execute_WithDest(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Should fail at download, not at parsing
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries": []any{"bin/tool"},
		"dest":     "custom-name.tar.gz",
	})
	if err == nil {
		t.Error("Expected download error")
	}
	// Should not be a format detection error
	if strings.Contains(err.Error(), "archive format") {
		t.Error("Did not expect format detection error")
	}
}

// -- composites.go: resolveAssetName without wildcards --

func TestGitHubArchiveAction_resolveAssetName_NoWildcards(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	name, err := action.resolveAssetName(ctx, map[string]any{
		"os_mapping":   map[string]any{"linux": "Linux"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	}, "tool-{version}-{os}-{arch}.tar.gz", "owner/repo")
	if err != nil {
		t.Fatalf("resolveAssetName() error = %v", err)
	}
	if name != "tool-1.0.0-Linux-x86_64.tar.gz" {
		t.Errorf("resolveAssetName() = %q, want %q", name, "tool-1.0.0-Linux-x86_64.tar.gz")
	}
}

// -- composites.go: GitHubArchiveAction.Execute with OS/arch mapping --

func TestGitHubArchiveAction_Execute_WithOSMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{Verify: &recipe.VerifySection{Command: "tool --version"}},
	}
	// Should fail at download but get past the mapping logic
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []any{"tool"},
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	// Should fail at download, not at param validation
	if err == nil {
		t.Error("Expected error (download failure)")
	}
	if strings.Contains(err.Error(), "repo") || strings.Contains(err.Error(), "asset_pattern") ||
		strings.Contains(err.Error(), "binaries") || strings.Contains(err.Error(), "archive format") {
		t.Errorf("Failed too early at parameter validation: %v", err)
	}
}

// -- composites.go: GitHubFileAction.Execute additional validation --

func TestGitHubFileAction_Execute_WithOSMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Should fail at download but get past mapping logic
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binary":        "tool",
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if err == nil {
		t.Error("Expected error (download failure)")
	}
}

// -- composites.go: DownloadArchiveAction.Execute with strip_dirs --

func TestDownloadArchiveAction_Execute_WithStripDirs(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Should fail at download, not at parsing
	err := action.Execute(ctx, map[string]any{
		"url":        "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries":   []any{"bin/tool"},
		"strip_dirs": 1,
	})
	if err == nil {
		t.Error("Expected download error")
	}
}

// -- composites.go: DownloadArchiveAction.Execute with directory install_mode --

func TestDownloadArchiveAction_Execute_DirectoryModeWithoutVerify(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":          "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries":     []any{"tool"},
		"install_mode": "directory",
	})
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Errorf("Expected verify section error, got %v", err)
	}
}

func TestDownloadArchiveAction_Execute_DirectoryWrappedModeWithoutVerify(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":          "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries":     []any{"tool"},
		"install_mode": "directory_wrapped",
	})
	if err == nil || !strings.Contains(err.Error(), "verify") {
		t.Errorf("Expected verify section error, got %v", err)
	}
}

// -- composites.go: GitHubFileAction.Execute with binaries format --

func TestGitHubFileAction_Execute_NewBinariesFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Uses new binaries format with src/dest maps
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binaries": []any{
			map[string]any{"src": "tool", "dest": "bin/tool"},
		},
	})
	// Should fail at download, not at param parsing
	if err == nil {
		t.Error("Expected error (download failure)")
	}
}

func TestGitHubFileAction_Execute_BinariesMissingSrc(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
		"binaries":      []any{map[string]any{"dest": "bin/tool"}},
	})
	if err == nil || !strings.Contains(err.Error(), "binaries[0].src") {
		t.Errorf("Expected binaries[0].src error, got %v", err)
	}
}

func TestGitHubFileAction_Execute_NoBinaryOrBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
	})
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Errorf("Expected binary error, got %v", err)
	}
}

// -- composites.go: DownloadArchiveAction.Execute with OS/arch mapping --

func TestDownloadArchiveAction_Execute_WithOSMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Should fail at download, exercising the mapping code paths
	err := action.Execute(ctx, map[string]any{
		"url":          "https://nonexistent.invalid/tool-{version}-{os}-{arch}.tar.gz",
		"binaries":     []any{"tool"},
		"os_mapping":   map[string]any{"linux": "Linux"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	})
	if err == nil {
		t.Error("Expected error (download failure)")
	}
	// Should fail at download, not at params
	if strings.Contains(err.Error(), "archive format") || strings.Contains(err.Error(), "binaries") {
		t.Errorf("Failed too early: %v", err)
	}
}

// -- GitHubArchiveAction.Decompose with OS/arch mapping --

func TestGitHubArchiveAction_Decompose_WithMappings(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []any{"tool"},
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) == 0 {
		t.Error("Decompose() returned no steps")
	}
}

// -- GitHubFileAction.Decompose with binaries format --

func TestGitHubFileAction_Decompose_WithBinariesFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binaries": []any{
			map[string]any{"src": "tool", "dest": "bin/tool"},
		},
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) == 0 {
		t.Error("Decompose() returned no steps")
	}
}

// -- composites.go: DownloadArchiveAction Execute error paths --

func TestDownloadArchiveAction_Execute_NoFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"url":      "https://example.com/tool",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error when format cannot be auto-detected")
	}
}

func TestDownloadArchiveAction_Execute_NoBinaries(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"url": "https://example.com/tool.tar.gz",
	})
	if err == nil {
		t.Error("Expected error when binaries is missing")
	}
}

// -- composites.go: GitHubArchiveAction Execute error paths --

func TestGitHubArchiveAction_Execute_NoRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{os}-{arch}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error when repo is missing")
	}
}

func TestGitHubArchiveAction_Execute_NoAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"repo":     "cli/cli",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error when asset_pattern is missing")
	}
}

// -- composites.go: GitHubFileAction Execute error paths --

func TestGitHubFileAction_Execute_NoRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{os}",
		"binary_name":   "tool",
	})
	if err == nil {
		t.Error("Expected error when repo is missing")
	}
}

func TestGitHubFileAction_Execute_NoAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	err := action.Execute(ctx, map[string]any{
		"repo":        "cli/cli",
		"binary_name": "tool",
	})
	if err == nil {
		t.Error("Expected error when asset_pattern is missing")
	}
}

// -- composites.go: Preflight --

func TestGitHubArchiveAction_Preflight_AllWarnings(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}.tar.gz",
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})

	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings for unused os/arch mappings, got %d: %v",
			len(result.Warnings), result.Warnings)
	}
}

func TestGitHubFileAction_Preflight_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}

	result := action.Preflight(map[string]any{
		"asset_pattern": "tool-{os}",
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubFileAction_Preflight_InvalidRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}

	result := action.Preflight(map[string]any{
		"repo":          "invalid",
		"asset_pattern": "tool-{os}",
	})
	hasRepoError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "owner/repository") {
			hasRepoError = true
			break
		}
	}
	if !hasRepoError {
		t.Errorf("Expected repo format error, got %v", result.Errors)
	}
}

// -- composites.go: GitHubArchiveAction.resolveAssetName --

func TestGitHubArchiveAction_ResolveAssetName(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	name, err := action.resolveAssetName(ctx, map[string]any{}, "tool-{version}-{os}-{arch}.tar.gz", "owner/repo")
	if err != nil {
		t.Fatalf("resolveAssetName() error: %v", err)
	}
	if name != "tool-1.0.0-linux-amd64.tar.gz" {
		t.Errorf("resolveAssetName() = %q, want tool-1.0.0-linux-amd64.tar.gz", name)
	}
}

func TestGitHubArchiveAction_ResolveAssetName_WithMappings(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "darwin",
		Arch:       "arm64",
	}

	params := map[string]any{
		"os_mapping":   map[string]any{"darwin": "macOS"},
		"arch_mapping": map[string]any{"arm64": "aarch64"},
	}

	name, err := action.resolveAssetName(ctx, params, "tool-{os}-{arch}.tar.gz", "owner/repo")
	if err != nil {
		t.Fatalf("resolveAssetName() error: %v", err)
	}
	if name != "tool-macOS-aarch64.tar.gz" {
		t.Errorf("resolveAssetName() = %q, want tool-macOS-aarch64.tar.gz", name)
	}
}

// -- composites.go: DownloadArchiveAction.Decompose --

func TestDownloadArchiveAction_Decompose_Basic(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url":      "https://example.com/{version}/tool-{os}-{arch}.tar.gz",
		"binaries": []any{"bin/tool"},
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	if len(steps) < 3 {
		t.Errorf("Decompose() returned %d steps, want >= 3", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("first step = %q, want download_file", steps[0].Action)
	}
}

func TestDownloadArchiveAction_Decompose_MissingURL(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing URL")
	}
}

func TestDownloadArchiveAction_Decompose_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"url": "https://example.com/tool.tar.gz",
	})
	if err == nil {
		t.Error("Expected error for missing binaries")
	}
}

func TestDownloadArchiveAction_Decompose_UndetectableFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"url":      "https://example.com/tool",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error when format cannot be detected")
	}
}

func TestDownloadArchiveAction_Decompose_WithStripDirs(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url":        "https://example.com/tool.tar.gz",
		"binaries":   []any{"bin/tool"},
		"strip_dirs": 1,
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	// Find extract step and check strip_dirs
	for _, s := range steps {
		if s.Action == "extract" {
			sd := s.Params["strip_dirs"]
			if sd != 1 {
				t.Errorf("extract step strip_dirs = %v, want 1", sd)
			}
		}
	}
}

// -- composites.go: GitHubArchiveAction.Decompose error paths --

func TestGitHubArchiveAction_Decompose_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"asset_pattern": "tool-{version}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubArchiveAction_Decompose_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"repo":     "owner/repo",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubArchiveAction_Decompose_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool.tar.gz",
	})
	if err == nil {
		t.Error("Expected error for missing binaries")
	}
}

// -- composites.go: GitHubFileAction.Decompose error paths --

func TestGitHubFileAction_Decompose_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"asset_pattern": "tool-{os}",
		"binary":        "tool",
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubFileAction_Decompose_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"repo":   "owner/repo",
		"binary": "tool",
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubFileAction_Decompose_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", VersionTag: "v1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{os}",
	})
	if err == nil {
		t.Error("Expected error for missing binary/binaries")
	}
}

// -- composites.go: GitHubFileAction.Preflight additional warnings --

func TestGitHubFileAction_Preflight_ArchiveExtensionWarning(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{os}.tar.gz",
	})
	hasArchiveWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "archive extension") {
			hasArchiveWarning = true
			break
		}
	}
	if !hasArchiveWarning {
		t.Errorf("Expected archive extension warning, got %v", result.Warnings)
	}
}

func TestGitHubFileAction_Preflight_UnusedMappings(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x64"},
	})
	if len(result.Warnings) < 2 {
		t.Errorf("Expected at least 2 warnings for unused mappings, got %d", len(result.Warnings))
	}
}

// -- composites.go: GitHubArchiveAction.Preflight more paths --

func TestGitHubArchiveAction_Preflight_MissingAll(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{})
	if len(result.Errors) < 2 {
		t.Errorf("Expected at least 2 errors (repo + asset_pattern), got %d", len(result.Errors))
	}
}

func TestGitHubArchiveAction_Preflight_InvalidRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "invalid-repo",
		"asset_pattern": "tool.tar.gz",
	})
	hasRepoError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "owner/repository") {
			hasRepoError = true
			break
		}
	}
	if !hasRepoError {
		t.Errorf("Expected repo format error, got %v", result.Errors)
	}
}

// -- composites.go: GitHubArchiveAction.Decompose with full success path --

func TestGitHubArchiveAction_Decompose_Success(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	if len(steps) < 3 {
		t.Errorf("Decompose() returned %d steps, want >= 3", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("first step = %q, want download_file", steps[0].Action)
	}
	// Verify URL contains owner/repo
	url, _ := GetString(steps[0].Params, "url")
	if !strings.Contains(url, "owner/repo") {
		t.Errorf("URL %q should contain owner/repo", url)
	}
}

// -- composites.go: GitHubFileAction.Decompose with binary param (backward compat) --

func TestGitHubFileAction_Decompose_WithBinaryParam(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{os}-{arch}",
		"binary":        "tool",
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	if len(steps) < 2 {
		t.Errorf("Decompose() returned %d steps, want >= 2", len(steps))
	}
}

// -- composites.go: GitHubArchiveAction.Execute param validation --

func TestGitHubArchiveAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubArchiveAction_Execute_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":     "owner/repo",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubArchiveAction_Execute_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
	})
	if err == nil {
		t.Error("Expected error for missing binaries")
	}
}

func TestGitHubArchiveAction_Execute_InvalidInstallMode(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool.tar.gz",
		"binaries":      []any{"bin/tool"},
		"install_mode":  "invalid",
	})
	if err == nil {
		t.Error("Expected error for invalid install_mode")
	}
}

func TestGitHubArchiveAction_Execute_UndetectableFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for undetectable archive format")
	}
}

// -- composites.go: GitHubFileAction.Execute param validation --

func TestGitHubFileAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binary":        "tool",
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubFileAction_Execute_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":   "owner/repo",
		"binary": "tool",
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubFileAction_Execute_MissingBinary(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
	})
	if err == nil {
		t.Error("Expected error for missing binary/binaries")
	}
}

// -- composites.go: DownloadArchiveAction Execute additional paths --

func TestDownloadArchiveAction_Execute_LibraryExempt(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Type: "library"},
		},
	}
	// Library type should not require verify section even with directory mode
	err := action.Execute(ctx, map[string]any{
		"url":            "https://nonexistent.invalid/lib.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []any{"lib/libfoo.so"},
		"install_mode":   "directory",
	})
	// Should fail at download, not at install_mode validation
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
	// Verify the error is about download, not about install_mode
	errMsg := err.Error()
	if strings.Contains(errMsg, "verify") || strings.Contains(errMsg, "install_mode") {
		t.Errorf("Error should be about download, not verify: %s", errMsg)
	}
}

// -- composites.go: GitHubFileAction Execute with mappings --

func TestGitHubFileAction_Execute_WithBinariesArray(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// binaries with invalid structure should fail
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
		"binaries":      []any{map[string]any{"dest": "bin/tool"}}, // Missing src
	})
	if err == nil {
		t.Error("Expected error for binaries without src")
	}
}

func TestGitHubFileAction_Execute_WithBinaryString(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: filepath.Join(t.TempDir(), ".install"),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Should pass validation but fail at download
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binary":        "tool",
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- composites.go: GitHubArchiveAction Preflight warnings --

func TestGitHubArchiveAction_Preflight_UnusedOSMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{arch}.tar.gz",
		"os_mapping":    map[string]any{"linux": "Linux"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused os_mapping")
	}
}

func TestGitHubArchiveAction_Preflight_UnusedArchMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}.tar.gz",
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused arch_mapping")
	}
}

func TestGitHubArchiveAction_Preflight_RedundantFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":           "owner/repo",
		"asset_pattern":  "tool.tar.gz",
		"archive_format": "tar.gz",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for redundant archive_format")
	}
}

// -- composites.go: DownloadArchiveAction Execute with mappings --

func TestDownloadArchiveAction_Execute_WithMappings(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// This tests the OS/arch mapping code paths. Download will fail.
	err := action.Execute(ctx, map[string]any{
		"url":            "https://nonexistent.invalid/{os}/{arch}/tool.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []any{"bin/tool"},
		"os_mapping":     map[string]any{"linux": "Linux"},
		"arch_mapping":   map[string]any{"amd64": "x86_64"},
	})
	// Should fail at download, not at mapping
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- composites.go: DownloadArchiveAction.Execute with format detection --

func TestDownloadArchiveAction_Execute_AutoDetectFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// URL has tar.gz extension so format should be auto-detected
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries": []any{"bin/tool"},
	})
	// Should fail at download, not at format detection
	if err == nil || strings.Contains(err.Error(), "archive format") {
		t.Error("Expected download error, not format detection error")
	}
}

func TestDownloadArchiveAction_Execute_CannotDetectFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool",
		"binaries": []any{"bin/tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "archive format") {
		t.Errorf("Expected format detection error, got %v", err)
	}
}

// -- composites.go: DownloadArchiveAction Preflight warning paths --

func TestDownloadArchiveAction_Preflight_UnusedOSMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{
		"url":        "https://nonexistent.invalid/tool-{version}-{arch}.tar.gz",
		"binaries":   []any{"bin/tool"},
		"os_mapping": map[string]any{"linux": "Linux"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused os_mapping")
	}
}

func TestDownloadArchiveAction_Preflight_UnusedArchMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{
		"url":          "https://nonexistent.invalid/tool-{version}-{os}.tar.gz",
		"binaries":     []any{"bin/tool"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused arch_mapping")
	}
}

// -- composites.go: DownloadArchiveAction Preflight missing binaries --

func TestDownloadArchiveAction_Preflight_MissingAll(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{})
	if len(result.Errors) == 0 {
		t.Error("Expected at least 1 error for missing params")
	}
}

// -- composites.go: GitHubFileAction Preflight additional paths --

func TestGitHubFileAction_Preflight_ValidWithBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binaries":      []any{map[string]any{"src": "tool", "dest": "bin/tool"}},
	})
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v", result.Errors)
	}
}

// -- composites.go: IsDeterministic for various actions --

func TestDownloadArchiveAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := DownloadArchiveAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestGitHubArchiveAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := GitHubArchiveAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestGitHubFileAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := GitHubFileAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

// -- composites.go: DownloadArchiveAction.Preflight with redundant archive_format --

func TestDownloadArchiveAction_Preflight_RedundantFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}

	result := action.Preflight(map[string]any{
		"url":            "https://example.com/file.tar.gz",
		"archive_format": "tar.gz",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for redundant archive_format")
	}
}

func TestDownloadArchiveAction_Preflight_MissingURL(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}

	result := action.Preflight(map[string]any{})
	if len(result.Errors) == 0 {
		t.Error("Expected error for missing URL")
	}
}

// newTestExecCtx creates a minimal ExecutionContext for tests that call Execute.
func newTestExecCtx(t *testing.T) *ExecutionContext {
	t.Helper()
	tmpDir := t.TempDir()
	return &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: filepath.Join(tmpDir, ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
}
