package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// -- composites.go: DownloadArchiveAction.Execute with format detection --

func TestDownloadArchiveAction_Execute_AutoDetectFormat(t *testing.T) {
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
	// URL has tar.gz extension so format should be auto-detected
	err := action.Execute(ctx, map[string]any{
		"url":      "https://nonexistent.invalid/tool-1.0.0.tar.gz",
		"binaries": []any{"bin/tool"},
	})
	// Should fail at download, not at format detection
	if err == nil || strings.Contains(err.Error(), "archive format") {
		t.Error("Expected download error, not format detection error")
	}
}

func TestDownloadArchiveAction_Execute_CannotDetectFormat(t *testing.T) {
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
		"url":      "https://nonexistent.invalid/tool",
		"binaries": []any{"bin/tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "archive format") {
		t.Errorf("Expected format detection error, got %v", err)
	}
}

// -- composites.go: DownloadArchiveAction Preflight warning paths --

func TestDownloadArchiveAction_Preflight_UnusedOSMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{
		"url":        "https://nonexistent.invalid/tool-{version}-{arch}.tar.gz",
		"binaries":   []any{"bin/tool"},
		"os_mapping": map[string]any{"linux": "Linux"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused os_mapping")
	}
}

func TestDownloadArchiveAction_Preflight_UnusedArchMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{
		"url":          "https://nonexistent.invalid/tool-{version}-{os}.tar.gz",
		"binaries":     []any{"bin/tool"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused arch_mapping")
	}
}

// -- composites.go: DownloadArchiveAction Preflight missing binaries --

func TestDownloadArchiveAction_Preflight_MissingAll(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	result := action.Preflight(map[string]any{})
	if len(result.Errors) == 0 {
		t.Error("Expected at least 1 error for missing params")
	}
}

// -- composites.go: GitHubFileAction Preflight additional paths --

func TestGitHubFileAction_Preflight_ValidWithBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binaries":      []any{map[string]any{"src": "tool", "dest": "bin/tool"}},
	})
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v", result.Errors)
	}
}

// -- cargo_build.go: Execute with missing Cargo.toml --

func TestCargoBuildAction_Execute_MissingCargoToml(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
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
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "Cargo.toml") {
		t.Errorf("Expected Cargo.toml error, got %v", err)
	}
}

// -- configure_make.go: Execute missing configure script --

func TestConfigureMakeAction_Execute_MissingConfigureScript(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
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
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
	})
	if err == nil {
		t.Error("Expected error for missing configure script")
	}
}

// -- gem_install.go: Execute param validation --

func TestGemInstallAction_Execute_MissingGem(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing gem param")
	}
}

func TestGemInstallAction_Execute_InvalidGemName(t *testing.T) {
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
		"gem": "invalid gem name!",
	})
	if err == nil {
		t.Error("Expected error for invalid gem name")
	}
}

func TestGemInstallAction_Execute_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "invalid version!@#",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem": "rails",
	})
	if err == nil {
		t.Error("Expected error for invalid version")
	}
}

func TestGemInstallAction_Execute_MissingExecutables(t *testing.T) {
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
		"gem": "rails",
	})
	if err == nil {
		t.Error("Expected error for missing executables")
	}
}

func TestGemInstallAction_Execute_InvalidExecutableName(t *testing.T) {
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
		"executables": []any{"../evil"},
	})
	if err == nil {
		t.Error("Expected error for invalid executable name with path separator")
	}
}

// -- pip_exec.go: Execute param validation --

func TestPipExecAction_Execute_MissingPackage(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing package param")
	}
}

func TestPipExecAction_Execute_MissingVersion(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
	})
	if err == nil {
		t.Error("Expected error for missing version param")
	}
}

func TestPipExecAction_Execute_MissingExecutables(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
		"version": "3.0.0",
	})
	if err == nil {
		t.Error("Expected error for missing executables param")
	}
}

func TestPipExecAction_Execute_MissingLockedRequirements(t *testing.T) {
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
	err := action.Execute(ctx, map[string]any{
		"package":     "flask",
		"version":     "3.0.0",
		"executables": []any{"flask"},
	})
	if err == nil {
		t.Error("Expected error for missing locked_requirements")
	}
}

// -- npm_exec.go: Execute with source_dir that has no package.json --

func TestNpmExecAction_Execute_NoPackageJSON(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
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
		"source_dir": tmpDir,
		"command":    "build",
	})
	if err == nil {
		t.Error("Expected error for missing package.json")
	}
}

func TestNpmExecAction_Execute_MissingCommand(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	tmpDir := t.TempDir()
	// Create package.json
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"source_dir": tmpDir,
	})
	if err == nil {
		t.Error("Expected error for missing command")
	}
}

// -- cargo_build.go: Execute with lock_data mode --

func TestCargoBuildAction_Execute_LockDataMode_MissingCrate(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: filepath.Join(t.TempDir(), ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
	})
	if err == nil {
		t.Error("Expected error for missing crate in lock_data mode")
	}
}

// -- app_bundle.go: Preflight --

func TestAppBundleAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &AppBundleAction{}

	t.Run("valid", func(t *testing.T) {
		result := action.Preflight(map[string]any{
			"url":      "https://nonexistent.invalid/app.dmg",
			"app_name": "MyApp.app",
			"binary":   "MyApp",
			"checksum": "abc123",
		})
		if len(result.Errors) != 0 {
			t.Errorf("Preflight() errors = %v", result.Errors)
		}
	})

	t.Run("missing required", func(t *testing.T) {
		result := action.Preflight(map[string]any{
			"url": "https://nonexistent.invalid/app.dmg",
		})
		if len(result.Errors) == 0 {
			t.Error("Expected errors for missing required params")
		}
	})
}

// -- fossil_archive.go: Decompose with custom tag format --

func TestFossilArchiveAction_Decompose_DefaultParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "3.46.0",
		VersionTag: "version-3.46.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"repo":         "https://nonexistent.invalid/src",
		"project_name": "test",
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("Decompose() returned %d steps, want 2", len(steps))
	}
}
