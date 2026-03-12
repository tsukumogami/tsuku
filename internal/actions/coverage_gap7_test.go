package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// newTestExecCtx creates a minimal ExecutionContext for tests that call Execute.
func newTestExecCtx(t *testing.T) *ExecutionContext {
	t.Helper()
	tmpDir := t.TempDir()
	return &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: filepath.Join(tmpDir, ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
}

// -- set_rpath.go: detectBinaryFormat (Mach-O 32-bit variants not covered elsewhere) --

func TestDetectBinaryFormat_MachO32BigEndian_Alt(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "macho32be")
	if err := os.WriteFile(path, []byte{0xfe, 0xed, 0xfa, 0xce, 0x00, 0x00, 0x00, 0x00}, 0755); err != nil {
		t.Fatal(err)
	}
	format, err := detectBinaryFormat(path)
	if err != nil {
		t.Fatalf("detectBinaryFormat() error = %v", err)
	}
	if format != "macho" {
		t.Errorf("detectBinaryFormat() = %q, want %q", format, "macho")
	}
}

func TestDetectBinaryFormat_MachO32LittleEndian(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "macho32le")
	if err := os.WriteFile(path, []byte{0xce, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}, 0755); err != nil {
		t.Fatal(err)
	}
	format, err := detectBinaryFormat(path)
	if err != nil {
		t.Fatalf("detectBinaryFormat() error = %v", err)
	}
	if format != "macho" {
		t.Errorf("detectBinaryFormat() = %q, want %q", format, "macho")
	}
}

func TestDetectBinaryFormat_MachO64BigEndian_Alt(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "macho64be")
	if err := os.WriteFile(path, []byte{0xfe, 0xed, 0xfa, 0xcf, 0x00, 0x00, 0x00, 0x00}, 0755); err != nil {
		t.Fatal(err)
	}
	format, err := detectBinaryFormat(path)
	if err != nil {
		t.Fatalf("detectBinaryFormat() error = %v", err)
	}
	if format != "macho" {
		t.Errorf("detectBinaryFormat() = %q, want %q", format, "macho")
	}
}

func TestDetectBinaryFormat_FatBinaryLittleEndianVariant(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fatle")
	if err := os.WriteFile(path, []byte{0xbe, 0xba, 0xfe, 0xca, 0x00, 0x00, 0x00, 0x02}, 0755); err != nil {
		t.Fatal(err)
	}
	format, err := detectBinaryFormat(path)
	if err != nil {
		t.Fatalf("detectBinaryFormat() error = %v", err)
	}
	if format != "macho" {
		t.Errorf("detectBinaryFormat() = %q, want %q", format, "macho")
	}
}

func TestDetectBinaryFormat_EmptyFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty")
	if err := os.WriteFile(path, []byte{}, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := detectBinaryFormat(path)
	if err == nil {
		t.Error("detectBinaryFormat() expected error for empty file")
	}
}

// -- set_rpath.go: validatePathWithinDir --

func TestValidatePathWithinDir_Valid(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "sub", "file")
	if err := validatePathWithinDir(target, tmpDir); err != nil {
		t.Errorf("validatePathWithinDir() error = %v", err)
	}
}

func TestValidatePathWithinDir_Escape(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "..", "outside")
	if err := validatePathWithinDir(target, tmpDir); err == nil {
		t.Error("validatePathWithinDir() expected error for path escape")
	}
}

// -- set_rpath.go: validateRpath --

func TestValidateRpath_Empty(t *testing.T) {
	t.Parallel()
	if err := validateRpath("", ""); err != nil {
		t.Errorf("validateRpath() error = %v for empty rpath", err)
	}
}

func TestValidateRpath_ValidOrigin(t *testing.T) {
	t.Parallel()
	if err := validateRpath("$ORIGIN/../lib", ""); err != nil {
		t.Errorf("validateRpath() error = %v for $ORIGIN path", err)
	}
}

func TestValidateRpath_ValidExecutablePath(t *testing.T) {
	t.Parallel()
	if err := validateRpath("@executable_path/../lib", ""); err != nil {
		t.Errorf("validateRpath() error = %v for @executable_path", err)
	}
}

func TestValidateRpath_ValidLoaderPath(t *testing.T) {
	t.Parallel()
	if err := validateRpath("@loader_path/../lib", ""); err != nil {
		t.Errorf("validateRpath() error = %v for @loader_path", err)
	}
}

func TestValidateRpath_AbsoluteWithinLibsDir(t *testing.T) {
	t.Parallel()
	libsDir := "/home/user/.tsuku/libs"
	if err := validateRpath("/home/user/.tsuku/libs/openssl/lib", libsDir); err != nil {
		t.Errorf("validateRpath() error = %v for path within libsDir", err)
	}
}

func TestValidateRpath_AbsoluteOutsideLibsDir(t *testing.T) {
	t.Parallel()
	libsDir := "/home/user/.tsuku/libs"
	if err := validateRpath("/usr/lib", libsDir); err == nil {
		t.Error("validateRpath() expected error for absolute path outside libsDir")
	}
}

func TestValidateRpath_AbsoluteNoLibsDir(t *testing.T) {
	t.Parallel()
	if err := validateRpath("/usr/lib", ""); err == nil {
		t.Error("validateRpath() expected error for absolute path with no libsDir")
	}
}

func TestValidateRpath_InvalidRelative(t *testing.T) {
	t.Parallel()
	if err := validateRpath("../lib", ""); err == nil {
		t.Error("validateRpath() expected error for relative path without valid prefix")
	}
}

func TestValidateRpath_ColonSeparated(t *testing.T) {
	t.Parallel()
	if err := validateRpath("$ORIGIN/../lib:@loader_path/../lib", ""); err != nil {
		t.Errorf("validateRpath() error = %v for colon-separated valid rpaths", err)
	}
}

// -- set_rpath.go: validateBinaryName --

func TestValidateBinaryName_Valid(t *testing.T) {
	t.Parallel()
	validNames := []string{"tool", "my-tool", "tool_v2", "lib.so.1", "a.out"}
	for _, name := range validNames {
		if err := validateBinaryName(name); err != nil {
			t.Errorf("validateBinaryName(%q) error = %v", name, err)
		}
	}
}

func TestValidateBinaryName_Invalid(t *testing.T) {
	t.Parallel()
	invalidNames := []string{"tool;rm -rf", "bin/tool", "tool name", "$PATH"}
	for _, name := range invalidNames {
		if err := validateBinaryName(name); err == nil {
			t.Errorf("validateBinaryName(%q) expected error", name)
		}
	}
}

// -- set_rpath.go: createLibraryWrapper --

func TestCreateLibraryWrapper_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "mytool")
	if err := os.WriteFile(binaryPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatal(err)
	}

	if err := createLibraryWrapper(binaryPath, "$ORIGIN/../lib"); err != nil {
		t.Fatalf("createLibraryWrapper() error = %v", err)
	}

	// Check wrapper script was created
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}
	if len(content) == 0 {
		t.Error("wrapper script is empty")
	}

	// Check original was renamed
	if _, err := os.Stat(binaryPath + ".orig"); err != nil {
		t.Error("original binary was not renamed to .orig")
	}
}

func TestCreateLibraryWrapper_Symlink(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	realPath := filepath.Join(tmpDir, "real")
	linkPath := filepath.Join(tmpDir, "link")
	if err := os.WriteFile(realPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}
	if err := createLibraryWrapper(linkPath, "$ORIGIN/../lib"); err == nil {
		t.Error("createLibraryWrapper() expected error for symlink")
	}
}

// -- pip_exec.go: fixPythonShebang --

func TestFixPythonShebang_AbsolutePythonPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.py")
	content := "#!/usr/bin/python3\nimport sys\nprint('hello')\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	if err := fixPythonShebang(scriptPath); err != nil {
		t.Fatalf("fixPythonShebang() error = %v", err)
	}

	result, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result[:2]) != "#!" {
		t.Error("result should start with shebang")
	}
}

func TestFixPythonShebang_NotAScript(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "binary")
	if err := os.WriteFile(scriptPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err == nil {
		t.Error("fixPythonShebang() expected error for non-script file")
	}
}

func TestFixPythonShebang_NotPython(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.sh")
	content := "#!/bin/bash\necho hello\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err == nil {
		t.Error("fixPythonShebang() expected error for non-Python script")
	}
}

func TestFixPythonShebang_AlreadyRelative(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.py")
	content := "#!/usr/bin/env ./python3\nimport sys\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err != nil {
		t.Errorf("fixPythonShebang() error = %v for already relative path", err)
	}
}

// -- pip_exec.go: countRequirementsPackages --

func TestCountRequirementsPackages_Continuation(t *testing.T) {
	t.Parallel()
	// Test continuation lines (backslash) are skipped
	input := "requests==2.31.0 \\\n  --hash=sha256:abc\n"
	got := countRequirementsPackages(input)
	if got != 1 {
		t.Errorf("countRequirementsPackages() = %d, want 1", got)
	}
}

// -- fossil_archive.go: versionToTag --

func TestFossilArchiveAction_VersionToTag_Direct(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}

	tests := []struct {
		version          string
		tagPrefix        string
		versionSeparator string
		want             string
	}{
		{"3.46.0", "version-", ".", "version-3.46.0"},
		{"9.0.0", "core-", "-", "core-9-0-0"},
		{"1.2.3", "", ".", "1.2.3"},
		{"1.2.3", "v", ".", "v1.2.3"},
		{"1.2.3", "release-", "_", "release-1_2_3"},
	}
	for _, tt := range tests {
		got := action.versionToTag(tt.version, tt.tagPrefix, tt.versionSeparator)
		if got != tt.want {
			t.Errorf("versionToTag(%q, %q, %q) = %q, want %q",
				tt.version, tt.tagPrefix, tt.versionSeparator, got, tt.want)
		}
	}
}

// -- fossil_archive.go: Execute param validation --

func TestFossilArchiveAction_Execute_MissingProjectName(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo": "https://sqlite.org/src",
	})
	if err == nil {
		t.Error("Expected error for missing project_name")
	}
}

// -- install_binaries.go: parseOutputs edge cases --

func TestInstallBinariesAction_ParseOutputs_InvalidType(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	_, err := action.parseOutputs([]any{42})
	if err == nil {
		t.Error("parseOutputs() expected error for int type")
	}
}

func TestInstallBinariesAction_ParseOutputs_MissingSrc(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	_, err := action.parseOutputs([]any{
		map[string]any{"dest": "bin/tool"},
	})
	if err == nil {
		t.Error("parseOutputs() expected error for missing src")
	}
}

func TestInstallBinariesAction_ParseOutputs_MissingDest(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	_, err := action.parseOutputs([]any{
		map[string]any{"src": "tool"},
	})
	if err == nil {
		t.Error("parseOutputs() expected error for missing dest")
	}
}

// -- install_binaries.go: DetermineExecutables additional cases --

func TestDetermineExecutables_InferFromBinPrefix(t *testing.T) {
	t.Parallel()
	outputs := []recipe.BinaryMapping{
		{Src: "tool", Dest: "bin/tool"},
		{Src: "lib.so", Dest: "lib/lib.so"},
	}
	result := DetermineExecutables(outputs, nil)
	if len(result) != 1 || result[0] != "bin/tool" {
		t.Errorf("DetermineExecutables() = %v, want [bin/tool]", result)
	}
}

func TestDetermineExecutables_ExplicitOverride(t *testing.T) {
	t.Parallel()
	outputs := []recipe.BinaryMapping{
		{Src: "tool", Dest: "bin/tool"},
	}
	explicit := []string{"custom/tool"}
	result := DetermineExecutables(outputs, explicit)
	if len(result) != 1 || result[0] != "custom/tool" {
		t.Errorf("DetermineExecutables() = %v, want [custom/tool]", result)
	}
}

// -- install_binaries.go: extractOutputNames --

func TestExtractOutputNames_Direct(t *testing.T) {
	t.Parallel()
	outputs := []recipe.BinaryMapping{
		{Src: "dist/tool", Dest: "bin/tool"},
		{Src: "build/lib.so", Dest: "lib/lib.so"},
	}
	names := extractOutputNames(outputs)
	if len(names) != 2 || names[0] != "tool" || names[1] != "lib.so" {
		t.Errorf("extractOutputNames() = %v, want [tool lib.so]", names)
	}
}

// -- install_binaries.go: validateBinaryPath --

func TestInstallBinariesAction_ValidateBinaryPath_DotDot(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	if err := action.validateBinaryPath("../../etc/passwd"); err == nil {
		t.Error("validateBinaryPath() expected error for '..'")
	}
}

func TestInstallBinariesAction_ValidateBinaryPath_Absolute(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	if err := action.validateBinaryPath("/usr/bin/tool"); err == nil {
		t.Error("validateBinaryPath() expected error for absolute path")
	}
}

func TestInstallBinariesAction_ValidateBinaryPath_Valid(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	if err := action.validateBinaryPath("bin/tool"); err != nil {
		t.Errorf("validateBinaryPath() error = %v for valid path", err)
	}
}

// -- composites.go: GitHubArchiveAction.Execute param validation --

func TestGitHubArchiveAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubArchiveAction_Execute_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":     "owner/repo",
		"binaries": []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubArchiveAction_Execute_MissingBinaries(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
	})
	if err == nil {
		t.Error("Expected error for missing binaries")
	}
}

func TestGitHubArchiveAction_Execute_InvalidInstallMode(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool.tar.gz",
		"binaries":      []any{"bin/tool"},
		"install_mode":  "invalid",
	})
	if err == nil {
		t.Error("Expected error for invalid install_mode")
	}
}

func TestGitHubArchiveAction_Execute_UndetectableFormat(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool",
		"binaries":      []any{"bin/tool"},
	})
	if err == nil {
		t.Error("Expected error for undetectable archive format")
	}
}

// -- composites.go: GitHubFileAction.Execute param validation --

func TestGitHubFileAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"binary":        "tool",
	})
	if err == nil {
		t.Error("Expected error for missing repo")
	}
}

func TestGitHubFileAction_Execute_MissingAssetPattern(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":   "owner/repo",
		"binary": "tool",
	})
	if err == nil {
		t.Error("Expected error for missing asset_pattern")
	}
}

func TestGitHubFileAction_Execute_MissingBinary(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
	})
	if err == nil {
		t.Error("Expected error for missing binary/binaries")
	}
}

// -- download.go: Execute param validation --

func TestDownloadAction_Execute_URLWithMappings(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	ctx := newTestExecCtx(t)
	// This tests the early code paths of Execute with mappings (will fail at HTTP)
	err := action.Execute(ctx, map[string]any{
		"url":          "https://example.com/{os}/{arch}/tool",
		"os_mapping":   map[string]any{"linux": "Linux"},
		"arch_mapping": map[string]any{"amd64": "x86_64"},
	})
	// Should fail at download, not at param validation
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- npm_exec.go: Dependencies, RequiresNetwork --

func TestNpmExecAction_Dependencies_Direct(t *testing.T) {
	t.Parallel()
	action := NpmExecAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "nodejs" {
		t.Errorf("Dependencies().InstallTime = %v, want [nodejs]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "nodejs" {
		t.Errorf("Dependencies().Runtime = %v, want [nodejs]", deps.Runtime)
	}
}

// -- pip_install.go: Dependencies, RequiresNetwork --

func TestPipInstallAction_Dependencies_Direct(t *testing.T) {
	t.Parallel()
	action := PipInstallAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "python" {
		t.Errorf("Dependencies().InstallTime = %v, want [python]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "python" {
		t.Errorf("Dependencies().Runtime = %v, want [python]", deps.Runtime)
	}
}

func TestPipInstallAction_RequiresNetwork_Direct(t *testing.T) {
	t.Parallel()
	action := PipInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- nix_realize.go: Dependencies, RequiresNetwork --

func TestNixRealizeAction_Dependencies_Direct(t *testing.T) {
	t.Parallel()
	action := NixRealizeAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "nix-portable" {
		t.Errorf("Dependencies().InstallTime = %v, want [nix-portable]", deps.InstallTime)
	}
}

func TestNixRealizeAction_RequiresNetwork_Direct(t *testing.T) {
	t.Parallel()
	action := NixRealizeAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- pip_exec.go: Dependencies --

func TestPipExecAction_Dependencies_Direct(t *testing.T) {
	t.Parallel()
	action := PipExecAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "python-standalone" {
		t.Errorf("Dependencies().InstallTime = %v, want [python-standalone]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "python-standalone" {
		t.Errorf("Dependencies().Runtime = %v, want [python-standalone]", deps.Runtime)
	}
}

// -- npm_exec.go: Execute missing source_dir --

func TestNpmExecAction_Execute_MissingSourceDir(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing source_dir and package_lock")
	}
}

// -- run_command.go: Preflight warnings --

func TestRunCommandAction_Preflight_HardcodedPaths(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	result := action.Preflight(map[string]any{
		"command": "ls ~/.tsuku/tools/something",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for hardcoded tsuku paths")
	}
}

func TestRunCommandAction_Preflight_HomeEnvPaths(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	result := action.Preflight(map[string]any{
		"command": "ls $HOME/.tsuku/bin/tool",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for $HOME/.tsuku paths")
	}
}

// -- chmod.go: Preflight warnings --

func TestChmodAction_Preflight_EmptyFiles(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	result := action.Preflight(map[string]any{
		"files": []any{},
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for empty files array")
	}
}

func TestChmodAction_Preflight_WorldWritable(t *testing.T) {
	t.Parallel()
	action := &ChmodAction{}
	result := action.Preflight(map[string]any{
		"files": []any{"bin/tool"},
		"mode":  "0777",
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for world-writable mode")
	}
}

// -- preflight.go: PreflightResult methods --

func TestPreflightResult_ToError_Nil(t *testing.T) {
	t.Parallel()
	var r *PreflightResult
	if err := r.ToError(); err != nil {
		t.Errorf("ToError() on nil = %v, want nil", err)
	}
}

func TestPreflightResult_ToError_NoErrors(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	if err := r.ToError(); err != nil {
		t.Errorf("ToError() with no errors = %v, want nil", err)
	}
}

func TestPreflightResult_ToError_SingleError(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	r.AddError("test error")
	err := r.ToError()
	if err == nil {
		t.Fatal("ToError() = nil, want error")
	}
	if err.Error() != "test error" {
		t.Errorf("ToError() = %q, want %q", err.Error(), "test error")
	}
}

func TestPreflightResult_ToError_MultipleErrors(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	r.AddError("error1")
	r.AddError("error2")
	r.AddError("error3")
	err := r.ToError()
	if err == nil {
		t.Fatal("ToError() = nil, want error")
	}
}

func TestPreflightResult_HasWarnings(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	if r.HasWarnings() {
		t.Error("HasWarnings() = true on empty result")
	}
	r.AddWarning("warn")
	if !r.HasWarnings() {
		t.Error("HasWarnings() = false after AddWarning")
	}
}

func TestPreflightResult_AddWarningf(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	r.AddWarningf("test %d", 42)
	if len(r.Warnings) != 1 || r.Warnings[0] != "test 42" {
		t.Errorf("AddWarningf() = %v, want [test 42]", r.Warnings)
	}
}

func TestPreflightResult_AddErrorf(t *testing.T) {
	t.Parallel()
	r := &PreflightResult{}
	r.AddErrorf("error %s", "msg")
	if len(r.Errors) != 1 || r.Errors[0] != "error msg" {
		t.Errorf("AddErrorf() = %v, want [error msg]", r.Errors)
	}
}

// -- preflight.go: ValidateAction and RegisteredNames --

func TestRegisteredNames_Sorted(t *testing.T) {
	t.Parallel()
	names := RegisteredNames()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("RegisteredNames() not sorted: %q < %q at index %d", names[i], names[i-1], i)
			break
		}
	}
}

func TestValidateAction_ExistingWithoutPreflight(t *testing.T) {
	t.Parallel()
	// cargo_build doesn't implement Preflight
	result := ValidateAction("cargo_build", map[string]any{})
	if result.HasErrors() {
		t.Errorf("ValidateAction(cargo_build) has errors = %v", result.Errors)
	}
}

// -- install_binaries.go: Preflight with binary param --

func TestInstallBinariesAction_Preflight_BinaryParam(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	result := action.Preflight(map[string]any{
		"binary": "tool",
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for 'binary' param (singular)")
	}
}

func TestInstallBinariesAction_Preflight_BothOutputsAndBinaries(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	result := action.Preflight(map[string]any{
		"outputs":  []any{"bin/tool"},
		"binaries": []any{"bin/tool"},
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for both outputs and binaries")
	}
}

func TestInstallBinariesAction_Preflight_EmptyOutputs(t *testing.T) {
	t.Parallel()
	action := &InstallBinariesAction{}
	result := action.Preflight(map[string]any{
		"outputs": []any{},
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for empty outputs array")
	}
}

// -- require_system.go: Preflight additional cases --

func TestRequireSystemAction_Preflight_DeprecatedInstallGuide(t *testing.T) {
	t.Parallel()
	action := &RequireSystemAction{}
	result := action.Preflight(map[string]any{
		"command":       "dpkg",
		"install_guide": "apt install libssl-dev",
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for deprecated install_guide")
	}
}

func TestRequireSystemAction_Preflight_IncompleteVersionDetection(t *testing.T) {
	t.Parallel()
	action := &RequireSystemAction{}
	result := action.Preflight(map[string]any{
		"command":     "dpkg",
		"min_version": "1.0",
	})
	if len(result.Errors) == 0 {
		t.Error("Expected error for min_version without version_flag/version_regex")
	}
}

// -- download.go: containsPlaceholder --

func TestContainsPlaceholder_Direct(t *testing.T) {
	t.Parallel()
	if !containsPlaceholder("https://example.com/{version}/tool", "version") {
		t.Error("expected true for URL with {version}")
	}
	if containsPlaceholder("https://example.com/tool", "version") {
		t.Error("expected false for URL without {version}")
	}
}
