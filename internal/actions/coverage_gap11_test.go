package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// -- utils.go: CopyDirectoryExcluding, CopySymlink, CopyFile --

func TestCopyDirectoryExcluding_WithExclude(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	// Create source structure with an excluded dir
	if err := os.MkdirAll(filepath.Join(srcDir, "keep", "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "exclude_me"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "keep", "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "exclude_me", "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectoryExcluding(srcDir, dstDir, "exclude_me"); err != nil {
		t.Fatalf("CopyDirectoryExcluding() error = %v", err)
	}

	// Kept files should exist
	if _, err := os.Stat(filepath.Join(dstDir, "keep", "file.txt")); err != nil {
		t.Error("Expected keep/file.txt to be copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "root.txt")); err != nil {
		t.Error("Expected root.txt to be copied")
	}

	// Excluded dir should not exist
	if _, err := os.Stat(filepath.Join(dstDir, "exclude_me")); !os.IsNotExist(err) {
		t.Error("Expected exclude_me directory to be excluded")
	}
}

func TestCopyDirectoryExcluding_WithSymlinks(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	// Create a file and a symlink to it
	if err := os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("real"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(srcDir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectoryExcluding(srcDir, dstDir, ""); err != nil {
		t.Fatalf("CopyDirectoryExcluding() error = %v", err)
	}

	// Symlink should be preserved
	target, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink error: %v", err)
	}
	if target != "real.txt" {
		t.Errorf("symlink target = %q, want %q", target, "real.txt")
	}
}

func TestCopyFile_Success(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "test.txt")
	dstPath := filepath.Join(dstDir, "sub", "test.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(srcPath, dstPath, 0755); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Errorf("file content = %q, want %q", string(data), "content")
	}

	info, _ := os.Stat(dstPath)
	if info.Mode().Perm() != 0755 {
		t.Errorf("file mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	t.Parallel()
	err := CopyFile("/nonexistent/file", filepath.Join(t.TempDir(), "dst"), 0644)
	if err == nil {
		t.Error("Expected error for nonexistent source")
	}
}

func TestCopySymlink_Success(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source symlink
	if err := os.WriteFile(filepath.Join(srcDir, "target.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	srcLink := filepath.Join(srcDir, "link")
	if err := os.Symlink("target.txt", srcLink); err != nil {
		t.Fatal(err)
	}

	dstLink := filepath.Join(dstDir, "sub", "link")
	if err := CopySymlink(srcLink, dstLink); err != nil {
		t.Fatalf("CopySymlink() error = %v", err)
	}

	target, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatal(err)
	}
	if target != "target.txt" {
		t.Errorf("symlink target = %q, want %q", target, "target.txt")
	}
}

func TestCopyDirectory_NoExclude(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dst")

	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDirectory() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dstDir, "a.txt")); err != nil {
		t.Error("Expected a.txt to be copied")
	}
}

// -- apply_patch.go: Execute early validation --

func TestApplyPatchAction_Execute_MissingBothURLAndData(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Errorf("Expected 'either url or data' error, got %v", err)
	}
}

func TestApplyPatchAction_Execute_BothURLAndData(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url":  "https://example.com/patch",
		"data": "some patch data",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot specify both") {
		t.Errorf("Expected 'cannot specify both' error, got %v", err)
	}
}

func TestApplyPatchAction_Execute_NonHTTPSURL(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url": "http://insecure.example.com/patch",
	})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("Expected HTTPS error, got %v", err)
	}
}

// -- apply_patch.go: Decompose validation --

func TestApplyPatchAction_Decompose_MissingBothURLAndData(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "either") {
		t.Errorf("Expected 'either' error, got %v", err)
	}
}

func TestApplyPatchAction_Decompose_BothURLAndData(t *testing.T) {
	t.Parallel()
	action := &ApplyPatchAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"url":  "https://example.com/patch",
		"data": "some data",
	})
	if err == nil || !strings.Contains(err.Error(), "cannot specify both") {
		t.Errorf("Expected 'cannot specify both' error, got %v", err)
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

// -- eval_deps.go: CheckEvalDeps --

func TestCheckEvalDeps_EmptyDeps(t *testing.T) {
	t.Parallel()
	result := CheckEvalDeps(nil)
	if result != nil {
		t.Errorf("CheckEvalDeps(nil) = %v, want nil", result)
	}
}

// -- gem_install.go: Execute additional validation --

func TestGemInstallAction_Execute_ControlCharInExecutable(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"evil\x00name"},
	})
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Errorf("Expected control characters error, got %v", err)
	}
}

func TestGemInstallAction_Execute_ShellMetacharInExecutable(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"evil$(cmd)"},
	})
	if err == nil || !strings.Contains(err.Error(), "shell metacharacters") {
		t.Errorf("Expected shell metacharacters error, got %v", err)
	}
}

func TestGemInstallAction_Execute_TooLongExecutableName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{strings.Repeat("a", 257)},
	})
	if err == nil || !strings.Contains(err.Error(), "length") {
		t.Errorf("Expected length error, got %v", err)
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

// -- download_cache.go: Save, Clear, Info paths --

func TestDownloadCache_Save_CreatesDir(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "new-cache")
	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	// Create a file to save
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.bin")
	if err := os.WriteFile(srcFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := cache.Save("https://example.com/test.bin", srcFile, "")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify cache dir was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Expected cache directory to be created")
	}
}

func TestDownloadCache_Clear_Success(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}

	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	// Save a file first
	srcFile := filepath.Join(t.TempDir(), "test.bin")
	if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("https://example.com/test.bin", srcFile, ""); err != nil {
		t.Fatal(err)
	}

	// Clear should work
	err := cache.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
}

func TestDownloadCache_Info_Empty(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}

	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("Info().EntryCount = %d, want 0", info.EntryCount)
	}
}

func TestDownloadCache_Info_WithEntries(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}

	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	// Save a couple files
	for i, name := range []string{"a.bin", "b.bin"} {
		srcFile := filepath.Join(t.TempDir(), name)
		content := strings.Repeat("x", (i+1)*100)
		if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := cache.Save("https://example.com/"+name, srcFile, ""); err != nil {
			t.Fatal(err)
		}
	}

	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount < 2 {
		t.Errorf("Info().EntryCount = %d, want >= 2", info.EntryCount)
	}
	if info.TotalSize == 0 {
		t.Error("Info().TotalSize = 0, want > 0")
	}
}

// -- download_cache.go: Check with checksum --

func TestDownloadCache_CheckAndSave_WithChecksum(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}

	cache := NewDownloadCache(cacheDir)
	cache.SetSkipSecurityChecks(true)

	// Save a file
	srcFile := filepath.Join(t.TempDir(), "test.bin")
	if err := os.WriteFile(srcFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	err := cache.Save("https://example.com/test.bin", srcFile, "")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check with correct empty checksum (should match by URL)
	destFile := filepath.Join(t.TempDir(), "restored.bin")
	found, err := cache.Check("https://example.com/test.bin", destFile, "", "sha256")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !found {
		t.Error("Check() = false, want true")
	}

	// File should have been restored
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("restored content = %q, want %q", string(data), "hello world")
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

// -- nix_install.go: NixInstallAction.Execute with invalid executable containing metachar --

func TestNixInstallAction_Execute_ShellMetacharExecutable(t *testing.T) {
	t.Parallel()
	action := &NixInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package":     "hello",
		"executables": []any{"evil;cmd"},
	})
	if err == nil {
		t.Error("Expected error for shell metacharacter in executable")
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

// -- gem_exec.go: extractBundlerVersion --

func TestExtractBundlerVersion_Found(t *testing.T) {
	t.Parallel()
	lockData := `GEM
  remote: https://rubygems.org/
  specs:
    rake (13.0.6)

BUNDLED WITH
   2.4.22
`
	version := extractBundlerVersion(lockData)
	if version != "2.4.22" {
		t.Errorf("extractBundlerVersion() = %q, want %q", version, "2.4.22")
	}
}

func TestExtractBundlerVersion_NotFound(t *testing.T) {
	t.Parallel()
	version := extractBundlerVersion("no bundled with section")
	if version != "" {
		t.Errorf("extractBundlerVersion() = %q, want empty", version)
	}
}

// -- pipx_install.go: Execute early validation --

func TestPipxInstallAction_Execute_MissingPackage(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "package") {
		t.Errorf("Expected package error, got %v", err)
	}
}

func TestPipxInstallAction_Execute_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "evil;inject",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestPipxInstallAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

// -- pipx_install.go: Decompose additional paths --

func TestPipxInstallAction_Decompose_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "evil;inject",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package":     "flask",
		"executables": []any{"flask"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestPipxInstallAction_Decompose_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package": "flask",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestPipxInstallAction_Decompose_EmptyVersion(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "",
		VersionTag: "",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package":     "flask",
		"executables": []any{"flask"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

// -- install_binaries.go: createSymlink --

func TestInstallBinariesAction_createSymlink(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	tmpDir := t.TempDir()

	targetDir := filepath.Join(tmpDir, "tools", "tool-1.0.0", "bin")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "tool")
	if err := os.WriteFile(targetFile, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(tmpDir, "tools", ".install", "bin")
	linkPath := filepath.Join(linkDir, "tool")

	err := action.createSymlink(targetFile, linkPath)
	if err != nil {
		t.Fatalf("createSymlink() error = %v", err)
	}

	// Verify symlink exists and is relative
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("createSymlink created absolute symlink: %s", target)
	}
}

// -- fossil_archive.go: Execute early validation --

func TestFossilArchiveAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Errorf("Expected repo error, got %v", err)
	}
}

// -- go_build.go: Execute early validation --

// -- pipx_install.go: isValidPyPIPackage additional cases --

func TestIsValidPyPIPackage_WithSpaces(t *testing.T) {
	t.Parallel()
	if isValidPyPIPackage("evil package") {
		t.Error("Expected false for package with spaces")
	}
}

func TestIsValidPyPIPackage_WithNewline(t *testing.T) {
	t.Parallel()
	if isValidPyPIPackage("evil\npackage") {
		t.Error("Expected false for package with newline")
	}
}

// -- go_build.go: Execute with cgo_enabled flag --

func TestGoBuildAction_Execute_WithCgoFlag(t *testing.T) {
	t.Parallel()
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Will fail at go_sum, but exercises cgo_enabled path
	err := action.Execute(ctx, map[string]any{
		"module":      "example.com/tool",
		"version":     "1.0.0",
		"executables": []any{"tool"},
		"cgo_enabled": true,
	})
	if err == nil || !strings.Contains(err.Error(), "go_sum") {
		t.Errorf("Expected go_sum error, got %v", err)
	}
}

// -- go_build.go: Execute with build_flags --

func TestGoBuildAction_Execute_WithBuildFlags(t *testing.T) {
	t.Parallel()
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"module":      "example.com/tool",
		"version":     "1.0.0",
		"executables": []any{"tool"},
		"build_flags": []any{"-trimpath"},
	})
	if err == nil || !strings.Contains(err.Error(), "go_sum") {
		t.Errorf("Expected go_sum error, got %v", err)
	}
}

// -- app_bundle.go: Execute on non-darwin --

func TestAppBundleAction_Execute_NonDarwin(t *testing.T) {
	t.Parallel()
	action := &AppBundleAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// On Linux, app_bundle should be a no-op (skip)
	err := action.Execute(ctx, map[string]any{
		"url":      "https://example.com/app.dmg",
		"checksum": "abc123",
		"app_name": "Test.app",
	})
	if err != nil {
		t.Errorf("Expected no error on non-darwin, got %v", err)
	}
}

// -- download_file.go: Execute early validation --

func TestDownloadFileAction_Execute_MissingURL(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "url") {
		t.Errorf("Expected url error, got %v", err)
	}
}

func TestDownloadFileAction_Execute_MissingChecksum(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{
		"url": "https://example.com/tool.bin",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Expected checksum error, got %v", err)
	}
}

func TestDownloadFileAction_Execute_EmptyChecksum(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{
		"url":      "https://example.com/tool.bin",
		"checksum": "",
	})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Expected checksum error, got %v", err)
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

// -- install_binaries.go: validateBinaryPath --

func TestInstallBinariesAction_validateBinaryPath(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}

	t.Run("valid path", func(t *testing.T) {
		err := action.validateBinaryPath("bin/tool")
		if err != nil {
			t.Errorf("validateBinaryPath(\"bin/tool\") error = %v", err)
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		err := action.validateBinaryPath("/etc/passwd")
		if err == nil {
			t.Error("Expected error for absolute path")
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		err := action.validateBinaryPath("../../../etc/passwd")
		if err == nil {
			t.Error("Expected error for path traversal")
		}
	})
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

// -- cargo_install.go: Execute further validation --

func TestCargoInstallAction_Execute_EmptyExecutable(t *testing.T) {
	t.Parallel()
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"crate":       "ripgrep",
		"executables": []any{""},
	})
	if err == nil {
		t.Error("Expected error for empty executable name")
	}
}

// -- NixInstallAction Decompose with executables --

func TestNixInstallAction_Decompose_NoExecutables(t *testing.T) {
	t.Parallel()
	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

// -- NixInstallAction Decompose with invalid executable --

func TestNixInstallAction_Decompose_BadExecutable(t *testing.T) {
	t.Parallel()
	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package":     "hello",
		"executables": []any{"../evil"},
	})
	if err == nil {
		t.Error("Expected error for invalid executable")
	}
}
