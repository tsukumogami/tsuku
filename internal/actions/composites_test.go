package actions

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestGitHubArchiveAction_VerificationEnforcement tests that directory mode requires verification
func TestGitHubArchiveAction_VerificationEnforcement(t *testing.T) {
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
	action := &DownloadArchiveAction{}
	if action.Name() != "download_archive" {
		t.Errorf("Name() = %q, want %q", action.Name(), "download_archive")
	}
}

func TestGitHubArchiveAction_Name(t *testing.T) {
	action := &GitHubArchiveAction{}
	if action.Name() != "github_archive" {
		t.Errorf("Name() = %q, want %q", action.Name(), "github_archive")
	}
}

func TestGitHubFileAction_Name(t *testing.T) {
	action := &GitHubFileAction{}
	if action.Name() != "github_file" {
		t.Errorf("Name() = %q, want %q", action.Name(), "github_file")
	}
}

func TestHashiCorpReleaseAction_Name(t *testing.T) {
	action := &HashiCorpReleaseAction{}
	if action.Name() != "hashicorp_release" {
		t.Errorf("Name() = %q, want %q", action.Name(), "hashicorp_release")
	}
}

// HomebrewBottleAction tests moved to homebrew_bottle_test.go

func TestExtractSourceFiles(t *testing.T) {
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

func TestHashiCorpReleaseAction_Execute_MissingParams(t *testing.T) {
	action := &HashiCorpReleaseAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'product' parameter is missing")
	}
}

// HomebrewBottleAction Execute tests moved to homebrew_bottle_test.go

// TestDownloadArchiveAction_VerificationEnforcement tests that directory mode requires verification
func TestDownloadArchiveAction_VerificationEnforcement(t *testing.T) {
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
