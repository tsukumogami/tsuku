package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// -- composites.go: DownloadArchiveAction Execute additional paths --

func TestDownloadArchiveAction_Execute_LibraryExempt(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Type: "library"},
		},
	}
	// Library type should not require verify section even with directory mode
	err := action.Execute(ctx, map[string]any{
		"url":            "https://nonexistent.invalid/lib.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []any{"lib/libfoo.so"},
		"install_mode":   "directory",
	})
	// Should fail at download, not at install_mode validation
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
	// Verify the error is about download, not about install_mode
	errMsg := err.Error()
	if strings.Contains(errMsg, "verify") || strings.Contains(errMsg, "install_mode") {
		t.Errorf("Error should be about download, not verify: %s", errMsg)
	}
}

// -- composites.go: GitHubFileAction Execute with mappings --

func TestGitHubFileAction_Execute_WithBinariesArray(t *testing.T) {
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
	// binaries with invalid structure should fail
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}",
		"binaries":      []any{map[string]any{"dest": "bin/tool"}}, // Missing src
	})
	if err == nil {
		t.Error("Expected error for binaries without src")
	}
}

func TestGitHubFileAction_Execute_WithBinaryString(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: filepath.Join(t.TempDir(), ".install"),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Should pass validation but fail at download
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binary":        "tool",
		"os_mapping":    map[string]any{"linux": "Linux"},
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- composites.go: GitHubArchiveAction Preflight warnings --

func TestGitHubArchiveAction_Preflight_UnusedOSMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{arch}.tar.gz",
		"os_mapping":    map[string]any{"linux": "Linux"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused os_mapping")
	}
}

func TestGitHubArchiveAction_Preflight_UnusedArchMapping(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}.tar.gz",
		"arch_mapping":  map[string]any{"amd64": "x86_64"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused arch_mapping")
	}
}

func TestGitHubArchiveAction_Preflight_RedundantFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	result := action.Preflight(map[string]any{
		"repo":           "owner/repo",
		"asset_pattern":  "tool.tar.gz",
		"archive_format": "tar.gz",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for redundant archive_format")
	}
}

// -- download.go: Decompose with dest --

func TestDownloadAction_Decompose_WithChecksumAlgo(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"url":           "https://nonexistent.invalid/tool",
		"checksum_algo": "sha512",
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("Decompose() returned %d steps, want 1", len(steps))
	}
}

// -- download.go: Decompose with URL query params --

func TestDownloadAction_Decompose_URLWithQueryParams(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"url": "https://nonexistent.invalid/tool.bin?token=abc",
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	dest, _ := GetString(steps[0].Params, "dest")
	if dest != "tool.bin" {
		t.Errorf("dest = %q, want %q (query params should be stripped)", dest, "tool.bin")
	}
}

// -- install_binaries.go: Execute with install_mode validation --

func TestInstallBinariesAction_Execute_InvalidInstallMode(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"outputs":      []any{"bin/tool"},
		"install_mode": "invalid_mode",
	})
	if err == nil {
		t.Error("Expected error for invalid install_mode")
	}
}

func TestInstallBinariesAction_Execute_DirectoryWrappedNotImplemented(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: filepath.Join(tmpDir, ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe: &recipe.Recipe{
			Verify: &recipe.VerifySection{Command: "true"},
		},
	}
	err := action.Execute(ctx, map[string]any{
		"outputs":      []any{"bin/tool"},
		"install_mode": "directory_wrapped",
	})
	if err == nil {
		t.Error("Expected error for directory_wrapped (not implemented)")
	}
}

func TestInstallBinariesAction_Execute_DirectoryModeNoVerify(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"outputs":      []any{"bin/tool"},
		"install_mode": "directory",
	})
	if err == nil {
		t.Error("Expected error for directory mode without verify")
	}
}

// -- install_binaries.go: Execute with binary traversal --

func TestInstallBinariesAction_Execute_TraversalInOutputs(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: filepath.Join(tmpDir, ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"outputs": []any{map[string]any{"src": "../../etc/passwd", "dest": "bin/passwd"}},
	})
	if err == nil {
		t.Error("Expected error for path traversal in outputs")
	}
}

// -- set_rpath.go: Execute with unexpanded dep vars --

func TestSetRpathAction_Execute_UnexpandedDepVar(t *testing.T) {
	t.Parallel()
	action := &SetRpathAction{}
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "tool")
	if err := os.WriteFile(binPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
	}
	err := action.Execute(ctx, map[string]any{
		"binaries": []any{"tool"},
		"rpath":    "{dep:nonexistent}/lib",
	})
	if err == nil {
		t.Error("Expected error for unexpanded dep variable")
	}
}

// -- system_config.go: RequireCommandAction Execute with valid command --

func TestRequireCommandAction_Execute_ValidCommand(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}
	// "true" is available on all Unix systems
	err := action.Execute(ctx, map[string]any{
		"command": "true",
	})
	if err != nil {
		t.Errorf("Execute() error = %v for valid command 'true'", err)
	}
}

func TestRequireCommandAction_Execute_MissingCommand(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing command param")
	}
}

func TestRequireCommandAction_Execute_NonexistentCommand(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}
	err := action.Execute(ctx, map[string]any{
		"command": "nonexistent_command_xyz",
	})
	if err == nil {
		t.Error("Expected error for nonexistent command")
	}
}

func TestRequireCommandAction_Execute_WithVersionCheck(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}
	// Use 'bash' with --version since it's reliably available
	err := action.Execute(ctx, map[string]any{
		"command":       "bash",
		"min_version":   "1.0",
		"version_flag":  "--version",
		"version_regex": `(\d+\.\d+)`,
	})
	if err != nil {
		t.Errorf("Execute() error = %v for bash version check", err)
	}
}

func TestRequireCommandAction_Execute_VersionCheckMissingFlags(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}
	err := action.Execute(ctx, map[string]any{
		"command":     "true",
		"min_version": "1.0",
	})
	if err == nil {
		t.Error("Expected error for min_version without version_flag/version_regex")
	}
}

// -- system_config.go: RequireSystemAction Execute --

// -- require_system.go: Preflight with complete version check --

func TestRequireSystemAction_Preflight_CompleteVersionCheck(t *testing.T) {
	t.Parallel()
	action := &RequireSystemAction{}
	result := action.Preflight(map[string]any{
		"command":       "dpkg",
		"min_version":   "1.0",
		"version_flag":  "--version",
		"version_regex": `(\d+\.\d+)`,
	})
	// Should have no errors when all version fields are present
	if len(result.Errors) != 0 {
		t.Errorf("Preflight() errors = %v for complete version check", result.Errors)
	}
}

// -- composites.go: DownloadArchiveAction Execute with mappings --

func TestDownloadArchiveAction_Execute_WithMappings(t *testing.T) {
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
	// This tests the OS/arch mapping code paths. Download will fail.
	err := action.Execute(ctx, map[string]any{
		"url":            "https://nonexistent.invalid/{os}/{arch}/tool.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []any{"bin/tool"},
		"os_mapping":     map[string]any{"linux": "Linux"},
		"arch_mapping":   map[string]any{"amd64": "x86_64"},
	})
	// Should fail at download, not at mapping
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- fossil_archive.go: Execute with full params (fails at download) --

func TestFossilArchiveAction_Execute_FullParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "3.46.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":              "https://nonexistent.invalid/src",
		"project_name":      "test",
		"strip_dirs":        1,
		"tag_prefix":        "version-",
		"version_separator": "-",
	})
	// Should fail at download, not at param validation
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}
