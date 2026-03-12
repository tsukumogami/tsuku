package actions

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests to close coverage gaps in various files.

// -- configure_make.go: Preflight --

func TestConfigureMakeAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}

	tests := []struct {
		name         string
		params       map[string]any
		wantErrors   int
		wantWarnings int
	}{
		{
			name:       "valid params",
			params:     map[string]any{"source_dir": "src"},
			wantErrors: 0,
		},
		{
			name:       "missing source_dir",
			params:     map[string]any{},
			wantErrors: 1,
		},
		{
			name: "skip_configure without make_args",
			params: map[string]any{
				"source_dir":     "src",
				"skip_configure": true,
			},
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name: "skip_configure with make_args",
			params: map[string]any{
				"source_dir":     "src",
				"skip_configure": true,
				"make_args":      []any{"install"},
			},
			wantErrors:   0,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantWarnings > 0 && len(result.Warnings) != tt.wantWarnings {
				t.Errorf("Preflight() warnings = %v, want %d", result.Warnings, tt.wantWarnings)
			}
		})
	}
}

// -- decomposable.go: DownloadResult.Cleanup --

func TestDownloadResult_Cleanup(t *testing.T) {
	t.Parallel()

	t.Run("empty path", func(t *testing.T) {
		r := &DownloadResult{AssetPath: ""}
		if err := r.Cleanup(); err != nil {
			t.Errorf("Cleanup() error = %v, want nil", err)
		}
	})

	t.Run("valid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "downloads")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		filePath := filepath.Join(subDir, "file.tar.gz")
		if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}

		r := &DownloadResult{AssetPath: filePath}
		if err := r.Cleanup(); err != nil {
			t.Errorf("Cleanup() error = %v", err)
		}

		// Verify the parent directory was removed
		if _, err := os.Stat(subDir); !os.IsNotExist(err) {
			t.Error("Expected directory to be removed after Cleanup()")
		}
	})
}

// -- download_cache.go: SetSkipSecurityChecks --

func TestDownloadCache_SetSkipSecurityChecks(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/tmp/test-cache")

	cache.SetSkipSecurityChecks(true)
	if !cache.skipSecurityChecks {
		t.Error("Expected skipSecurityChecks to be true")
	}

	cache.SetSkipSecurityChecks(false)
	if cache.skipSecurityChecks {
		t.Error("Expected skipSecurityChecks to be false")
	}
}

func TestDownloadCache_SkipSecurityChecks_SaveAndCheck(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create cache dir with overly permissive mode that would normally be rejected
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0777); err != nil {
		t.Fatal(err)
	}

	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	// Create a file to cache
	srcFile := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcFile, []byte("cached content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save should succeed even with permissive permissions
	err := cache.Save("https://example.com/test.tar.gz", srcFile, "")
	if err != nil {
		t.Fatalf("Save with skip security checks failed: %v", err)
	}

	// Check should also succeed
	destPath := filepath.Join(tmpDir, "dest.txt")
	found, err := cache.Check("https://example.com/test.tar.gz", destPath, "", "")
	if err != nil {
		t.Fatalf("Check with skip security checks failed: %v", err)
	}
	if !found {
		t.Error("Expected cache hit")
	}
}

// -- preflight.go: RegisteredNames, ValidateAction --

func TestRegisteredNames_NotEmpty(t *testing.T) {
	t.Parallel()
	names := RegisteredNames()
	if len(names) == 0 {
		t.Error("RegisteredNames() returned empty list")
	}

	// Verify names are sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("RegisteredNames() not sorted: %q comes after %q", names[i], names[i-1])
			break
		}
	}
}

func TestValidateAction_AllRegisteredActions(t *testing.T) {
	t.Parallel()
	// This just verifies ValidateAction doesn't panic for known actions with empty params.
	// Many will have errors (missing required params), which is fine.
	names := RegisteredNames()
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			result := ValidateAction(name, map[string]any{})
			// Result should be non-nil
			if result == nil {
				t.Errorf("ValidateAction(%q) returned nil", name)
			}
		})
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

func TestFossilArchiveAction_IsDeterministic_Direct(t *testing.T) {
	t.Parallel()
	action := FossilArchiveAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

// -- download.go: IsDeterministic --

func TestDownloadAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := DownloadAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

// -- apply_patch.go: IsDeterministic, Preflight --

func TestApplyPatchAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := ApplyPatchAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestApplyPatchAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}

	tests := []struct {
		name       string
		params     map[string]any
		wantErrors int
	}{
		{
			name:       "valid URL patch",
			params:     map[string]any{"url": "https://example.com/patch.diff", "sha256": "abc123"},
			wantErrors: 0,
		},
		{
			name:       "valid data patch",
			params:     map[string]any{"data": "--- a/file\n+++ b/file\n"},
			wantErrors: 0,
		},
		{
			name:       "missing both url and data",
			params:     map[string]any{},
			wantErrors: 1,
		},
		{
			name:       "both url and data",
			params:     map[string]any{"url": "https://example.com/p.diff", "data": "patch data", "sha256": "abc"},
			wantErrors: 1,
		},
		{
			name:       "url without sha256",
			params:     map[string]any{"url": "https://example.com/patch.diff"},
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d errors", result.Errors, tt.wantErrors)
			}
		})
	}
}

// -- extract.go: Name, IsDeterministic --

func TestExtractAction_Name(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	if got := action.Name(); got != "extract" {
		t.Errorf("Name() = %q, want %q", got, "extract")
	}
}

func TestExtractAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := ExtractAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

// -- gem_common.go: createGemWrapper --

func TestCreateGemWrapper_SameDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a fake gem script
	srcScript := filepath.Join(tmpDir, "mygem")
	if err := os.WriteFile(srcScript, []byte("#!/usr/bin/env ruby\nputs 'hello'"), 0755); err != nil {
		t.Fatal(err)
	}

	err := createGemWrapper(srcScript, tmpDir, "mygem", "/usr/bin", ".")
	if err != nil {
		t.Fatalf("createGemWrapper() error = %v", err)
	}

	// .gem file should exist
	gemPath := filepath.Join(tmpDir, "mygem.gem")
	if _, err := os.Stat(gemPath); os.IsNotExist(err) {
		t.Error("Expected .gem file to exist")
	}

	// wrapper should exist
	wrapperPath := filepath.Join(tmpDir, "mygem")
	wrapperContent, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(wrapperContent) == 0 {
		t.Error("Wrapper script is empty")
	}
}

func TestCreateGemWrapper_CrossDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}

	srcScript := filepath.Join(srcDir, "mygem")
	if err := os.WriteFile(srcScript, []byte("#!/usr/bin/env ruby\nputs 'hello'"), 0755); err != nil {
		t.Fatal(err)
	}

	err := createGemWrapper(srcScript, dstDir, "mygem", "/usr/bin", "ruby/3.2")
	if err != nil {
		t.Fatalf("createGemWrapper() error = %v", err)
	}

	// .gem file should exist in dst
	gemPath := filepath.Join(dstDir, "mygem.gem")
	if _, err := os.Stat(gemPath); os.IsNotExist(err) {
		t.Error("Expected .gem file in dst dir")
	}

	// wrapper should exist
	wrapperPath := filepath.Join(dstDir, "mygem")
	if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
		t.Error("Expected wrapper script in dst dir")
	}
}

// -- fossil_archive.go: Execute error paths --

func TestFossilArchiveAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}

	t.Run("missing repo", func(t *testing.T) {
		err := action.Execute(ctx, map[string]any{
			"project_name": "test",
		})
		if err == nil {
			t.Error("Expected error for missing repo")
		}
	})

	t.Run("missing project_name", func(t *testing.T) {
		err := action.Execute(ctx, map[string]any{
			"repo": "https://example.com/src",
		})
		if err == nil {
			t.Error("Expected error for missing project_name")
		}
	})
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
