package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestDownloadFileAction_Execute_ValidationErrors tests that Execute rejects invalid parameters
func TestDownloadFileAction_Execute_ValidationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		params      map[string]any
		errContains string
	}{
		{
			name:        "missing url",
			params:      map[string]any{},
			errContains: "url",
		},
		{
			name:        "missing checksum",
			params:      map[string]any{"url": "https://example.com/tool.bin"},
			errContains: "checksum",
		},
		{
			name:        "empty checksum",
			params:      map[string]any{"url": "https://example.com/tool.bin", "checksum": ""},
			errContains: "checksum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action := &DownloadFileAction{}
			ctx := &ExecutionContext{
				Context: context.Background(),
				WorkDir: t.TempDir(),
				Version: "1.0.0",
				OS:      "linux",
				Arch:    "amd64",
				Recipe:  &recipe.Recipe{},
			}
			err := action.Execute(ctx, tt.params)
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Expected error containing %q, got %v", tt.errContains, err)
			}
		})
	}
}

// -- download_file.go: Execute with valid params but unreachable URL --
// Covers: dest default path, logger, checksum_algo default, download attempt

func TestDownloadFileAction_Execute_DownloadFailure(t *testing.T) {
	t.Parallel()
	action := &DownloadFileAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"checksum": "abc123def456",
	})
	// Should fail at download, not at parameter validation
	if err == nil {
		t.Error("Expected download error")
	}
	if strings.Contains(err.Error(), "requires") {
		t.Errorf("Expected download error, got validation error: %v", err)
	}
}

// -- download_file.go: Execute with explicit dest param --

func TestDownloadFileAction_Execute_WithDest(t *testing.T) {
	t.Parallel()
	action := &DownloadFileAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"checksum": "abc123def456",
		"dest":     "custom-name.tar.gz",
	})
	if err == nil {
		t.Error("Expected download error")
	}
}

// -- download_file.go: Execute with checksum_algo param --

func TestDownloadFileAction_Execute_WithChecksumAlgo(t *testing.T) {
	t.Parallel()
	action := &DownloadFileAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":           "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"checksum":      "abc123def456",
		"checksum_algo": "sha512",
	})
	if err == nil {
		t.Error("Expected download error")
	}
}

// -- download_file.go: Execute with download cache --

func TestDownloadFileAction_Execute_WithCache(t *testing.T) {
	t.Parallel()
	action := &DownloadFileAction{}
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context:                 context.Background(),
		WorkDir:                 tmpDir,
		Version:                 "1.0.0",
		OS:                      "linux",
		Arch:                    "amd64",
		Recipe:                  &recipe.Recipe{},
		DownloadCacheDir:        cacheDir,
		SkipCacheSecurityChecks: true,
	}
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"checksum": "abc123def456",
	})
	if err == nil {
		t.Error("Expected download error")
	}
}

// -- download_file.go: Execute with URL containing query params (dest detection) --

func TestDownloadFileAction_Execute_URLWithQueryParams(t *testing.T) {
	t.Parallel()
	action := &DownloadFileAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool.tar.gz?token=abc",
		"checksum": "abc123def456",
	})
	if err == nil {
		t.Error("Expected download error")
	}
}
