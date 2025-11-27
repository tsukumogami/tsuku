package actions

import (
	"context"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/recipe"
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
