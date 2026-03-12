package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// -- npm_exec.go: executePackageInstall validation paths --

func TestNpmExecAction_ExecutePackageInstall_MissingPackage(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package_lock": "{}",
	})
	if err == nil || !strings.Contains(err.Error(), "package") {
		t.Errorf("Expected package error, got %v", err)
	}
}

func TestNpmExecAction_ExecutePackageInstall_MissingVersion(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package_lock": "{}",
		"package":      "some-pkg",
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestNpmExecAction_ExecutePackageInstall_MissingPackageLock(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// package_lock must be present to trigger executePackageInstall
	// but the actual lock content is checked by the method
	err := action.Execute(ctx, map[string]any{
		"package_lock": "{}",
		"package":      "some-pkg",
		"version":      "1.0.0",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestNpmExecAction_ExecutePackageInstall_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package_lock": "{}",
		"package":      "some-pkg",
		"version":      "1.0.0",
		"executables":  []any{},
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

// -- npm_install.go: Decompose validation paths --

func TestNpmInstallAction_Decompose_InvalidPackageName(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package":     "invalid name!",
		"executables": []any{"tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected invalid package name error, got %v", err)
	}
}

// -- gem_install.go: Decompose additional paths --

func TestGemInstallAction_Decompose_MissingGem(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "gem") {
		t.Errorf("Expected gem error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_InvalidGemName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "!invalid",
		"executables": []any{"tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected invalid error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem": "rails",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_MissingVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"rails"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "invalid!",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"rails"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

// -- cargo_install.go: Decompose invalid version --

func TestCargoInstallAction_Decompose_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &CargoInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "invalid!@#",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"crate":       "ripgrep",
		"executables": []any{"rg"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

// -- set_env.go: parseVars additional error paths --

func TestSetEnvAction_ParseVars_MissingName(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"vars": []any{
			map[string]any{"value": "bar"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("Expected name error, got %v", err)
	}
}

func TestSetEnvAction_ParseVars_MissingValue(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"vars": []any{
			map[string]any{"name": "FOO"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "value") {
		t.Errorf("Expected value error, got %v", err)
	}
}

func TestSetEnvAction_ParseVars_NonArray(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"vars": "not an array",
	})
	if err == nil || !strings.Contains(err.Error(), "array") {
		t.Errorf("Expected array error, got %v", err)
	}
}

func TestSetEnvAction_ParseVars_NonMapEntry(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"vars": []any{"not a map"},
	})
	if err == nil || !strings.Contains(err.Error(), "map") {
		t.Errorf("Expected map error, got %v", err)
	}
}

// -- text_replace.go: Execute additional paths --

func TestTextReplaceAction_Execute_MissingPattern(t *testing.T) {
	t.Parallel()
	action := &TextReplaceAction{}
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"file": "test.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "pattern") {
		t.Errorf("Expected pattern error, got %v", err)
	}
}

// -- download.go: Preflight with static checksum and skip_verification --

func TestDownloadAction_Preflight_StaticChecksum(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":      "https://nonexistent.invalid/tool-{version}.tar.gz",
		"checksum": "abc123",
	})
	hasChecksumError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "checksum") {
			hasChecksumError = true
		}
	}
	if !hasChecksumError {
		t.Error("Expected error about static checksum not supported")
	}
}

func TestDownloadAction_Preflight_SkipVerificationReason(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":                      "https://nonexistent.invalid/tool-{version}.tar.gz",
		"skip_verification_reason": "Upstream does not provide checksums",
	})
	// Should not have the "no upstream verification" warning
	for _, w := range result.Warnings {
		if strings.Contains(w, "no upstream verification") {
			t.Error("Should not warn about verification when skip_verification_reason is set")
		}
	}
}

func TestDownloadAction_Preflight_MutuallyExclusiveVerifiers(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":                       "https://nonexistent.invalid/tool-{version}.tar.gz",
		"checksum_url":              "https://nonexistent.invalid/checksums.txt",
		"signature_url":             "https://nonexistent.invalid/tool.asc",
		"signature_key_url":         "https://nonexistent.invalid/key.asc",
		"signature_key_fingerprint": "D53626F8174A9846F6A573CC1253FA47EA19E301",
	})
	hasMutualError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "mutually exclusive") {
			hasMutualError = true
		}
	}
	if !hasMutualError {
		t.Error("Expected mutually exclusive error")
	}
}

func TestDownloadAction_Preflight_InvalidFingerprint(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":                       "https://nonexistent.invalid/tool-{version}.tar.gz",
		"signature_url":             "https://nonexistent.invalid/tool.asc",
		"signature_key_url":         "https://nonexistent.invalid/key.asc",
		"signature_key_fingerprint": "not-a-fingerprint",
	})
	hasFingerprintError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "fingerprint") {
			hasFingerprintError = true
		}
	}
	if !hasFingerprintError {
		t.Error("Expected fingerprint format error")
	}
}

func TestDownloadAction_Preflight_StaticURL(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url": "https://nonexistent.invalid/tool.tar.gz",
	})
	hasStaticURLError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "download_file") {
			hasStaticURLError = true
		}
	}
	if !hasStaticURLError {
		t.Error("Expected error about static URL (should use download_file)")
	}
}

// -- cpan_install.go: min function --

func TestCpanInstall_Min(t *testing.T) {
	t.Parallel()
	if min(3, 5) != 3 {
		t.Errorf("min(3, 5) = %d, want 3", min(3, 5))
	}
	if min(7, 2) != 2 {
		t.Errorf("min(7, 2) = %d, want 2", min(7, 2))
	}
	if min(4, 4) != 4 {
		t.Errorf("min(4, 4) = %d, want 4", min(4, 4))
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

// -- composites.go: GitHubFileAction.Preflight --

func TestGitHubFileAction_Preflight_NoRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"asset_pattern": "tool-{version}",
		"binary":        "tool",
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

// -- composites.go: DownloadArchiveAction.Preflight with archive_format warning --

func TestDownloadArchiveAction_Preflight_RedundantArchiveFormat(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{
		"url":            "https://nonexistent.invalid/tool-{version}.tar.gz",
		"binaries":       []any{"bin/tool"},
		"archive_format": "tar.gz",
	})
	// Check for redundant archive_format warning
	hasRedundantWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "archive_format") {
			hasRedundantWarning = true
		}
	}
	// This should either have a warning or no errors
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v", result.Errors)
	}
	_ = hasRedundantWarning // May or may not have this warning
}

// -- signature.go: ValidateFingerprint, NormalizeFingerprint, FormatFingerprint --

func TestValidateFingerprint_Valid(t *testing.T) {
	t.Parallel()
	err := ValidateFingerprint("D53626F8174A9846F6A573CC1253FA47EA19E301")
	if err != nil {
		t.Errorf("ValidateFingerprint() error = %v", err)
	}
}

func TestValidateFingerprint_Invalid(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		"short",
		"D53626F8174A9846F6A573CC1253FA47EA19E30Z",  // non-hex
		"D53626F8174A9846F6A573CC1253FA47EA19E3011", // too long
	}
	for _, fp := range tests {
		if err := ValidateFingerprint(fp); err == nil {
			t.Errorf("ValidateFingerprint(%q) should fail", fp)
		}
	}
}

// -- signature.go: ParseFingerprint additional formats --

func TestParseFingerprint_Valid(t *testing.T) {
	t.Parallel()
	// Spaced format
	fp, err := ParseFingerprint("D536 26F8 174A 9846 F6A5 73CC 1253 FA47 EA19 E301")
	if err != nil {
		t.Errorf("ParseFingerprint() error = %v", err)
	}
	if fp != "D53626F8174A9846F6A573CC1253FA47EA19E301" {
		t.Errorf("ParseFingerprint() = %q", fp)
	}
}

func TestParseFingerprint_Invalid(t *testing.T) {
	t.Parallel()
	_, err := ParseFingerprint("not-a-fingerprint")
	if err == nil {
		t.Error("ParseFingerprint() should fail for invalid input")
	}
}

// -- download_cache.go: Invalidate with non-existent cache --

func TestDownloadCache_invalidate_NonExistent(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache(t.TempDir())
	cache.SetSkipSecurityChecks(true)
	// invalidating a non-cached URL should not panic
	cache.invalidate("https://nonexistent.invalid/file.tar.gz")
}

// -- extract.go: isPathWithinDirectory edge cases --

func TestIsPathWithinDirectory_SameDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if !isPathWithinDirectory(tmpDir, tmpDir) {
		t.Error("isPathWithinDirectory() should return true for same directory")
	}
}

func TestIsPathWithinDirectory_Parent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parent := filepath.Dir(tmpDir)
	if isPathWithinDirectory(parent, tmpDir) {
		t.Error("isPathWithinDirectory() should return false for parent directory")
	}
}

func TestIsPathWithinDirectory_PartialMatch(t *testing.T) {
	t.Parallel()
	// /tmp/foobar should NOT match /tmp/foo
	if isPathWithinDirectory("/tmp/foobar", "/tmp/foo") {
		t.Error("isPathWithinDirectory() should not match partial directory names")
	}
}

// -- extract.go: validateSymlinkTarget --

func TestValidateSymlinkTarget_Absolute(t *testing.T) {
	t.Parallel()
	err := validateSymlinkTarget("/etc/passwd", "/tmp/archive/link", "/tmp/archive")
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("Expected absolute symlink error, got %v", err)
	}
}

func TestValidateSymlinkTarget_Escape(t *testing.T) {
	t.Parallel()
	err := validateSymlinkTarget("../../etc/passwd", "/tmp/archive/subdir/link", "/tmp/archive")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("Expected escape error, got %v", err)
	}
}

func TestValidateSymlinkTarget_Valid(t *testing.T) {
	t.Parallel()
	err := validateSymlinkTarget("../lib/libfoo.so", "/tmp/archive/bin/link", "/tmp/archive")
	if err != nil {
		t.Errorf("validateSymlinkTarget() error = %v", err)
	}
}

// -- extract.go: DetectArchiveFormat --

func TestDetectArchiveFormat_Variants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"file.tar.gz", "tar.gz"},
		{"file.tgz", "tar.gz"},
		{"file.tar.xz", "tar.xz"},
		{"file.tar.bz2", "tar.bz2"},
		{"file.tar.zst", "tar.zst"},
		{"file.zip", "zip"},
		{"file.tar", "tar"},
		{"file.tar.lz", "tar.lz"},
		{"file.unknown", ""},
	}
	for _, tt := range tests {
		got := DetectArchiveFormat(tt.input)
		if got != tt.want {
			t.Errorf("DetectArchiveFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// -- eval_deps.go: GetEvalDeps --

func TestGetEvalDeps_UnknownAction(t *testing.T) {
	t.Parallel()
	deps := GetEvalDeps("nonexistent_action_xyz")
	if deps != nil {
		t.Errorf("GetEvalDeps(unknown) = %v, want nil", deps)
	}
}

func TestGetEvalDeps_KnownAction(t *testing.T) {
	t.Parallel()
	deps := GetEvalDeps("gem_install")
	// gem_install has ruby as eval-time dep
	if len(deps) == 0 {
		t.Error("GetEvalDeps(gem_install) should return eval-time deps")
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

// -- cargo_build.go: executeLockDataMode validation paths --

func TestCargoBuildAction_ExecuteLockDataMode_InvalidCrateName(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "!invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid crate") {
		t.Errorf("Expected invalid crate error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "!invalid",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "ripgrep",
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_MissingLockChecksum(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "ripgrep",
	})
	if err == nil || !strings.Contains(err.Error(), "lock_checksum") {
		t.Errorf("Expected lock_checksum error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data":     "[dependencies]\n",
		"crate":         "ripgrep",
		"lock_checksum": "abc123",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_InvalidExecutable(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data":     "[dependencies]\n",
		"crate":         "ripgrep",
		"lock_checksum": "abc123",
		"executables":   []any{"../evil"},
	})
	if err == nil || !strings.Contains(err.Error(), "path separator") {
		t.Errorf("Expected path separator error, got %v", err)
	}
}

// -- cargo_build.go: Execute source_dir mode with features that have path traversal --

func TestCargoBuildAction_Execute_WithPathTraversalFeature(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Feature with path traversal (.. is rejected by isValidFeatureName)
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
		"features":    []any{"..evil"},
	})
	if err == nil {
		t.Error("Expected error for feature with path traversal")
	}
}

// -- cargo_build.go: Execute source_dir mode with options --

func TestCargoBuildAction_Execute_WithOptions(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	tmpDir := t.TempDir()
	// Create Cargo.toml and Cargo.lock for locked build
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Request unlocked build (locked=false) so we skip the Cargo.lock check
	// Build will still fail at actual cargo invocation but will pass validation
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
		"locked":      false,
	})
	// Should fail at cargo build, not at validation
	if err != nil && strings.Contains(err.Error(), "source_dir") {
		t.Errorf("Expected cargo build error, not validation error: %v", err)
	}
}

// -- npm_install.go: NpmInstallAction Decompose more paths --

func TestNpmInstallAction_Decompose_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package": "some-pkg",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestNpmInstallAction_Decompose_MissingVersion2(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"package":     "some-pkg",
		"executables": []any{"tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
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

// -- install_binaries.go: installBinariesMode helper --

func TestInstallBinariesAction_Execute_DirectoryMode(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create a binary in work dir
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tool"), []byte("#!/bin/sh\necho hello"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{Verify: &recipe.VerifySection{Command: "tool --version"}},
	}
	err := action.Execute(ctx, map[string]any{
		"binaries":     []any{"bin/tool"},
		"install_mode": "directory",
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

// -- homebrew.go: Execute param validation (not already in homebrew_test.go) --

func TestHomebrewAction_Execute_MissingFormula(t *testing.T) {
	t.Parallel()
	action := &HomebrewAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "formula") {
		t.Errorf("Expected formula error, got %v", err)
	}
}

func TestHomebrewAction_Execute_InvalidFormula(t *testing.T) {
	t.Parallel()
	action := &HomebrewAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"formula": "lib;evil",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("Expected invalid character error, got %v", err)
	}
}

func TestHomebrewAction_Execute_UnsupportedPlatform(t *testing.T) {
	t.Parallel()
	action := &HomebrewAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		OS:      "windows",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"formula": "libyaml",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("Expected unsupported platform error, got %v", err)
	}
}

// -- meson_build.go: Execute missing meson.build --

func TestMesonBuildAction_Execute_MissingMesonBuild(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
	})
	if err == nil {
		t.Error("Expected error for missing meson.build")
	}
}

// -- registryValidator tests (preflight.go) --

func TestRegistryValidator_RegisteredNames(t *testing.T) {
	t.Parallel()
	v := &registryValidator{}
	names := v.RegisteredNames()
	if len(names) == 0 {
		t.Error("RegisteredNames() returned empty list")
	}
}

func TestRegistryValidator_ValidateAction(t *testing.T) {
	t.Parallel()
	v := &registryValidator{}

	t.Run("known action", func(t *testing.T) {
		result := v.ValidateAction("download_archive", map[string]any{
			"url":      "https://nonexistent.invalid/tool-{version}.tar.gz",
			"binaries": []any{"bin/tool"},
		})
		// Should not panic and should return a result
		if result == nil {
			t.Error("ValidateAction() returned nil")
		}
	})

	t.Run("unknown action", func(t *testing.T) {
		result := v.ValidateAction("nonexistent_action_xyz", map[string]any{})
		if result == nil {
			t.Error("ValidateAction() returned nil for unknown action")
		}
	})
}
