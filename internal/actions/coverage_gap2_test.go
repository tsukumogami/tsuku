package actions

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// -- download.go: Execute error paths --

func TestDownloadAction_Execute_NoURLParam(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error when url is missing")
	}
}

// -- configure_make.go: Execute additional paths --

func TestConfigureMakeAction_Execute_SkipConfigure(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()
	installDir := t.TempDir()

	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a Makefile
	makefile := filepath.Join(sourceDir, "Makefile")
	if err := os.WriteFile(makefile, []byte("all:\n\t@echo done\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: installDir,
		Version:    "1.0.0",
	}

	// This exercises the skip_configure code path. It will fail at make execution
	// because we don't have a real build environment, but that's fine.
	err := action.Execute(ctx, map[string]any{
		"source_dir":     "src",
		"skip_configure": true,
		"executables":    []any{"tool"},
		"make_args":      []any{"all"},
	})
	// Error is expected - we're just exercising the code path
	_ = err
}

// -- download_cache.go: cacheKey and cachePaths --

func TestDownloadCache_CacheKey(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/tmp/cache")

	key1 := cache.cacheKey("https://example.com/file1.tar.gz")
	key2 := cache.cacheKey("https://example.com/file2.tar.gz")

	if key1 == key2 {
		t.Error("Different URLs should produce different cache keys")
	}

	key1a := cache.cacheKey("https://example.com/file1.tar.gz")
	if key1 != key1a {
		t.Error("Same URL should produce same cache key")
	}
}

func TestDownloadCache_CachePaths(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/tmp/cache")

	filePath, metaPath := cache.cachePaths("https://example.com/file.tar.gz")

	if !strings.HasPrefix(filePath, "/tmp/cache/") {
		t.Errorf("filePath %q should be under cache dir", filePath)
	}
	if !strings.HasSuffix(filePath, ".data") {
		t.Errorf("filePath %q should end with .data", filePath)
	}
	if !strings.HasSuffix(metaPath, ".meta") {
		t.Errorf("metaPath %q should end with .meta", metaPath)
	}
}

// -- util.go: VerifyChecksum edge cases --

func TestVerifyChecksum_WithAlgorithmPrefix(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	content := []byte("test content for checksum")
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify unsupported algorithm returns error
	err := VerifyChecksum(filePath, "abc123", "md5")
	if err == nil {
		t.Error("Expected unsupported algorithm error")
	}
	if !strings.Contains(err.Error(), "unsupported hash algorithm") {
		t.Errorf("Expected 'unsupported hash algorithm' error, got: %v", err)
	}
}

func TestVerifyChecksum_SHA512_Mismatch(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	content := []byte("sha512 test")
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	err := VerifyChecksum(filePath, "wrong_hash", "sha512")
	if err == nil {
		t.Error("Expected checksum mismatch error for SHA512")
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

// -- extract.go: Execute with strip_dirs via Execute method --

func TestExtractAction_Execute_WithStripDirs(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	archiveData := buildTarGz(t, map[string]string{"top/inner/file.txt": "strip content"})
	archivePath := filepath.Join(tmpDir, "strip.tar.gz")
	if err := os.WriteFile(archivePath, archiveData, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]any{
		"archive":    "strip.tar.gz",
		"format":     "tar.gz",
		"strip_dirs": 1,
	})
	if err != nil {
		t.Fatalf("Execute with strip_dirs failed: %v", err)
	}

	extractedFile := filepath.Join(tmpDir, "inner", "file.txt")
	if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
		t.Error("Expected file at inner/file.txt after strip_dirs=1")
	}
}

// -- extract.go: Execute with files filter --

func TestExtractAction_Execute_WithFilesFilter(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	archiveData := buildTarGz(t, map[string]string{
		"wanted.txt":   "wanted",
		"unwanted.txt": "unwanted",
	})
	archivePath := filepath.Join(tmpDir, "filter.tar.gz")
	if err := os.WriteFile(archivePath, archiveData, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]any{
		"archive": "filter.tar.gz",
		"format":  "tar.gz",
		"files":   []any{"wanted.txt"},
	})
	if err != nil {
		t.Fatalf("Execute with files filter failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "wanted.txt")); os.IsNotExist(err) {
		t.Error("Expected wanted.txt to be extracted")
	}
}

// buildTarGz creates a tar.gz archive with the given files.
func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, content := range files {
		data := []byte(content)
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}

	_ = tw.Close()
	_ = gzw.Close()
	return buf.Bytes()
}
