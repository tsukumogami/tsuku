package actions

import (
	"context"
	"testing"

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

func TestDownloadArchiveAction_Name(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	if action.Name() != "download_archive" {
		t.Errorf("Name() = %q, want %q", action.Name(), "download_archive")
	}
}

func TestGitHubArchiveAction_Name(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	if action.Name() != "github_archive" {
		t.Errorf("Name() = %q, want %q", action.Name(), "github_archive")
	}
}

func TestGitHubFileAction_Name(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	if action.Name() != "github_file" {
		t.Errorf("Name() = %q, want %q", action.Name(), "github_file")
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
			name: "missing archive_format",
			params: map[string]interface{}{
				"url": "https://example.com/file.tar.gz",
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
			name: "missing archive_format",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}.tar.gz",
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

	// Step 1: download
	if steps[0].Action != "download" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download")
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
			name: "missing archive_format",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "file-{version}.tar.gz",
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
					Verify: recipe.VerifySection{
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
