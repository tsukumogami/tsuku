package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- download_cache.go: Clear empty, Clear nonexistent, Info nonexistent, Invalidate, Save with checksum --

func TestDownloadCache_Clear_EmptyDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cache := NewDownloadCache(tmpDir)
	if err := cache.Clear(); err != nil {
		t.Errorf("Clear() on empty dir error: %v", err)
	}
}

func TestDownloadCache_Clear_NonexistentDir(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/nonexistent/cache/dir")
	if err := cache.Clear(); err != nil {
		t.Errorf("Clear() on nonexistent dir error: %v", err)
	}
}

func TestDownloadCache_Info_NonexistentDir(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/nonexistent/cache/dir")
	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() on nonexistent dir error: %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("EntryCount = %d, want 0", info.EntryCount)
	}
}

func TestDownloadCache_Invalidate(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	// Cache directory must have 0700 permissions for security checks
	if err := os.Chmod(tmpDir, 0700); err != nil {
		t.Fatal(err)
	}
	cache := NewDownloadCache(tmpDir)

	srcFile := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	url := "https://example.com/inval.tar.gz"
	if err := cache.Save(url, srcFile, ""); err != nil {
		t.Fatal(err)
	}

	cache.invalidate(url)

	destPath := filepath.Join(tmpDir, "dest.txt")
	found, err := cache.Check(url, destPath, "", "")
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if found {
		t.Error("Expected cache miss after invalidate")
	}
}

func TestDownloadCache_SaveAndCheckNoChecksum(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	cache := NewDownloadCache(cacheDir)

	srcFile := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(srcFile, []byte("checksum content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := cache.Save("https://example.com/cs.tar.gz", srcFile, "stored_checksum"); err != nil {
		t.Fatal(err)
	}

	// Check without providing checksum (should find via URL only)
	destPath := filepath.Join(tmpDir, "dest.txt")
	found, err := cache.Check("https://example.com/cs.tar.gz", destPath, "", "")
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if !found {
		t.Error("Expected cache hit without checksum verification")
	}
}

// -- computeSHA256 nonexistent --

func TestComputeSHA256_NonexistentFile(t *testing.T) {
	t.Parallel()
	_, err := computeSHA256("/nonexistent/file")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// -- copyFile nonexistent source --

func TestCopyFile_NonexistentSource(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	err := copyFile("/nonexistent/file", filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Error("Expected error for nonexistent source")
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

// -- action.go: Log --

func TestExecutionContext_Log(t *testing.T) {
	t.Parallel()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	// Should return a non-nil logger
	logger := ctx.Log()
	if logger == nil {
		t.Error("Log() returned nil logger")
	}
}

// -- download.go: Preflight valid URL, no errors --

func TestDownloadAction_Preflight_ValidURL(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url": "https://example.com/{version}/tool-{os}-{arch}.tar.gz",
	})
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v, want 0", result.Errors)
	}
}

// -- configure_make.go: findMake --

func TestFindMake(t *testing.T) {
	t.Parallel()
	result := findMake()
	if result == "" {
		t.Error("findMake() returned empty string")
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

// -- apply_patch.go: Decompose error paths --

func TestApplyPatchAction_Decompose_URLWithSHA(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url":    "https://example.com/patch.diff",
		"sha256": "abc123",
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	if len(steps) < 2 {
		t.Errorf("Decompose() returned %d steps, want >= 2", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("first step = %q, want download_file", steps[0].Action)
	}
}

func TestApplyPatchAction_Decompose_HTTPNotHTTPS(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{
		"url": "http://example.com/patch.diff",
	})
	if err == nil {
		t.Error("Expected error for non-https URL")
	}
}

func TestApplyPatchAction_Decompose_MissingBoth(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing both url and data")
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
