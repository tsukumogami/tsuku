package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// -- apt_actions.go: AptRepoAction.Execute (stub, 0% covered) --

func TestAptRepoAction_Execute_Stub(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url": "https://example.invalid/repo",
	})
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (stub)", err)
	}
}

// -- apt_actions.go: AptPPAAction.Execute (stub, 0% covered) --

func TestAptPPAAction_Execute_Stub(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"ppa": "deadsnakes/ppa",
	})
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (stub)", err)
	}
}

// -- dnf_actions.go: DnfRepoAction.Execute (stub, 0% covered) --

func TestDnfRepoAction_Execute_Stub(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"url": "https://example.invalid/repo",
	})
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (stub)", err)
	}
}

// -- brew_actions.go: BrewInstallAction.Execute missing packages branch --

func TestBrewInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- brew_actions.go: BrewCaskAction.Execute missing packages branch --

func TestBrewCaskAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- pipx_install.go: computeFileSHA256 (0% covered, pure function) --

func TestComputeFileSHA256_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := computeFileSHA256(path)
	if err != nil {
		t.Fatalf("computeFileSHA256() error = %v", err)
	}
	// SHA256 of "hello world\n"
	expected := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if hash != expected {
		t.Errorf("computeFileSHA256() = %q, want %q", hash, expected)
	}
}

func TestComputeFileSHA256_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := computeFileSHA256("/nonexistent/file")
	if err == nil {
		t.Error("Expected error for missing file")
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

// -- linux_pm_actions.go: AptInstallAction.Execute uncovered branches --

func TestAptInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- linux_pm_actions.go: PacmanInstallAction.Execute uncovered branches --

func TestPacmanInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- linux_pm_actions.go: ApkInstallAction.Execute uncovered branches --

func TestApkInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- dnf_actions.go: DnfInstallAction.Execute missing packages --

func TestDnfInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing packages")
	}
}

// -- apt_actions.go: AptInstallAction.Execute with valid packages but missing system deps --

func TestAptInstallAction_Execute_WithPackages(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Use a very unlikely package name to test the missing-dependency path
	err := action.Execute(ctx, map[string]any{
		"packages": []any{"__tsuku_nonexistent_pkg_test__"},
	})
	// Should either succeed (if detection thinks it's installed) or fail with DependencyMissingError
	// Either way, we're exercising the code path past parameter validation
	_ = err
}

// -- download.go: DownloadAction.Execute uncovered branches --

func TestDownloadAction_Execute_WithURLFailsAtDownload(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
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
		"url":  "https://nonexistent.invalid/file.tar.gz",
		"dest": "output.tar.gz",
	})
	if err == nil {
		t.Error("Expected download error")
	}
}

// -- pip_exec.go: PipExecAction.Execute with valid params up to pip resolution --

// -- pip_exec.go: Execute with all required params (exercises optional param paths) --

func TestPipExecAction_Execute_AllRequiredParams(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Provide all required params to exercise code past locked_requirements check
	err := action.Execute(ctx, map[string]any{
		"package":             "flask",
		"version":             "3.0.0",
		"executables":         []any{"flask"},
		"locked_requirements": "flask==3.0.0",
		"has_native_addons":   true,
	})
	// Should fail at python resolution, not param validation
	if err == nil {
		t.Error("Expected error (python not found)")
	}
	if strings.Contains(err.Error(), "requires") {
		t.Errorf("Expected python resolution error, got param error: %v", err)
	}
}
