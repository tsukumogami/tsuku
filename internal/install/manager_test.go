package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestGenerateWrapperScript_NoPathAdditions(t *testing.T) {
	content := generateWrapperScript("/home/user/.tsuku/tools/mytool-1.0.0/bin/mytool", nil)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should NOT have PATH= line when no additions
	if strings.Contains(content, "PATH=") {
		t.Errorf("wrapper should not have PATH= line when no path additions, got: %s", content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/mytool-1.0.0/bin/mytool" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_SinglePathAddition(t *testing.T) {
	pathAdditions := []string{"/home/user/.tsuku/tools/nodejs-20.10.0/bin"}
	content := generateWrapperScript("/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo", pathAdditions)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should have PATH= line with the addition
	expectedPath := `PATH="/home/user/.tsuku/tools/nodejs-20.10.0/bin:$PATH"`
	if !strings.Contains(content, expectedPath) {
		t.Errorf("wrapper should have PATH addition, want %s, got: %s", expectedPath, content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_MultiplePathAdditions(t *testing.T) {
	pathAdditions := []string{
		"/home/user/.tsuku/tools/nodejs-20.10.0/bin",
		"/home/user/.tsuku/tools/python-3.11.0/bin",
	}
	content := generateWrapperScript("/home/user/.tsuku/tools/sometool-1.0.0/bin/sometool", pathAdditions)

	// Should have shebang
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang, got: %s", content)
	}

	// Should have PATH= line with both additions (colon separated)
	// Order may vary due to map iteration, so check both are present
	if !strings.Contains(content, "PATH=\"") {
		t.Errorf("wrapper should have PATH= line, got: %s", content)
	}
	if !strings.Contains(content, "/home/user/.tsuku/tools/nodejs-20.10.0/bin") {
		t.Errorf("wrapper should contain nodejs path, got: %s", content)
	}
	if !strings.Contains(content, "/home/user/.tsuku/tools/python-3.11.0/bin") {
		t.Errorf("wrapper should contain python path, got: %s", content)
	}
	if !strings.Contains(content, ":$PATH\"") {
		t.Errorf("wrapper should end PATH with :$PATH, got: %s", content)
	}

	// Should have exec with target
	if !strings.Contains(content, `exec "/home/user/.tsuku/tools/sometool-1.0.0/bin/sometool" "$@"`) {
		t.Errorf("wrapper should have exec line, got: %s", content)
	}
}

func TestGenerateWrapperScript_CorrectLineOrder(t *testing.T) {
	pathAdditions := []string{"/home/user/.tsuku/tools/nodejs-20.10.0/bin"}
	content := generateWrapperScript("/home/user/.tsuku/tools/turbo-1.10.0/bin/turbo", pathAdditions)

	lines := strings.Split(content, "\n")

	// Line 0: shebang
	if lines[0] != "#!/bin/sh" {
		t.Errorf("line 0 should be shebang, got: %s", lines[0])
	}

	// Line 1: PATH=
	if !strings.HasPrefix(lines[1], "PATH=") {
		t.Errorf("line 1 should be PATH=, got: %s", lines[1])
	}

	// Line 2: exec
	if !strings.HasPrefix(lines[2], "exec ") {
		t.Errorf("line 2 should be exec, got: %s", lines[2])
	}
}

func TestGenerateWrapperScript_AbsolutePaths(t *testing.T) {
	pathAdditions := []string{"/absolute/path/to/dep/bin"}
	content := generateWrapperScript("/absolute/path/to/tool/bin/tool", pathAdditions)

	// Verify absolute paths are used (no $HOME or relative paths)
	if strings.Contains(content, "$HOME") {
		t.Errorf("wrapper should use absolute paths, not $HOME, got: %s", content)
	}
	if strings.Contains(content, "$TSUKU_HOME") {
		t.Errorf("wrapper should use absolute paths, not $TSUKU_HOME, got: %s", content)
	}

	// The paths should be absolute
	if !strings.Contains(content, "/absolute/path/to/dep/bin") {
		t.Errorf("wrapper should contain absolute dep path, got: %s", content)
	}
	if !strings.Contains(content, "/absolute/path/to/tool/bin/tool") {
		t.Errorf("wrapper should contain absolute tool path, got: %s", content)
	}
}

func TestGenerateWrapperScript_NoEmptyPathEntries(t *testing.T) {
	// Empty path additions should not create empty PATH entries like "::$PATH"
	content := generateWrapperScript("/path/to/tool", []string{})

	if strings.Contains(content, "::") {
		t.Errorf("wrapper should not have empty path entries (::), got: %s", content)
	}
	if strings.Contains(content, "PATH=\":") {
		t.Errorf("wrapper should not start PATH with colon, got: %s", content)
	}
}

func TestValidateShellSafePath_ValidPaths(t *testing.T) {
	validPaths := []string{
		"/home/user/.tsuku/tools/nodejs-20.10.0/bin",
		"/usr/local/bin/tool",
		"/path/with-dashes/and_underscores/v1.0.0",
		"/path/with spaces/is fine",
		"/tmp/tool@version",
	}

	for _, path := range validPaths {
		if err := validateShellSafePath(path); err != nil {
			t.Errorf("expected path %q to be valid, got error: %v", path, err)
		}
	}
}

func TestValidateShellSafePath_DangerousChars(t *testing.T) {
	tests := []struct {
		name string
		path string
		char rune
	}{
		{"newline", "/path/with\nnewline", '\n'},
		{"carriage return", "/path/with\rreturn", '\r'},
		{"double quote", "/path/with\"quote", '"'},
		{"single quote", "/path/with'quote", '\''},
		{"backtick", "/path/with`command`", '`'},
		{"dollar sign", "/path/with$var", '$'},
		{"backslash", "/path/with\\escape", '\\'},
		{"semicolon", "/path/with;command", ';'},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateShellSafePath(tc.path)
			if err == nil {
				t.Errorf("expected error for path with %s, got nil", tc.name)
			}
		})
	}
}

func TestCreateBinaryWrapper_Success(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create the tool directory structure
	toolDir := cfg.ToolDir("mytool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(binDir, "mytool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	// Ensure current dir exists
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create wrapper with runtime deps
	runtimeDeps := map[string]string{
		"nodejs": "20.10.0",
	}

	err := mgr.createBinaryWrapper("mytool", "1.0.0", "bin/mytool", runtimeDeps)
	if err != nil {
		t.Fatalf("createBinaryWrapper() error = %v", err)
	}

	// Verify wrapper was created
	wrapperPath := cfg.CurrentSymlink("mytool")
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	// Check wrapper content
	contentStr := string(content)
	if !strings.HasPrefix(contentStr, "#!/bin/sh\n") {
		t.Errorf("wrapper should start with shebang")
	}
	if !strings.Contains(contentStr, "PATH=") {
		t.Errorf("wrapper should contain PATH=")
	}
	if !strings.Contains(contentStr, "nodejs-20.10.0") {
		t.Errorf("wrapper should contain nodejs path")
	}
	if !strings.Contains(contentStr, "exec") {
		t.Errorf("wrapper should contain exec")
	}

	// Check wrapper is executable
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("failed to stat wrapper: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("wrapper should be executable")
	}
}

func TestCreateBinaryWrapper_InvalidBinaryPath(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	tests := []struct {
		name       string
		binaryPath string
	}{
		{"empty", ""},
		{"dot", "."},
		{"dotdot", ".."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := mgr.createBinaryWrapper("tool", "1.0.0", tc.binaryPath, nil)
			if err == nil {
				t.Errorf("expected error for invalid binary path %q", tc.binaryPath)
			}
		})
	}
}

func TestCreateBinaryWrapper_DeterministicOrder(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create the tool directory structure
	toolDir := cfg.ToolDir("mytool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create wrapper multiple times with same deps in different order
	// The output should be deterministic (sorted by dep name)
	runtimeDeps := map[string]string{
		"zebra":  "1.0.0",
		"alpha":  "2.0.0",
		"middle": "3.0.0",
	}

	var contents []string
	for i := 0; i < 5; i++ {
		err := mgr.createBinaryWrapper("mytool", "1.0.0", "bin/mytool", runtimeDeps)
		if err != nil {
			t.Fatalf("createBinaryWrapper() error = %v", err)
		}

		wrapperPath := cfg.CurrentSymlink("mytool")
		content, err := os.ReadFile(wrapperPath)
		if err != nil {
			t.Fatalf("failed to read wrapper: %v", err)
		}
		contents = append(contents, string(content))
	}

	// All contents should be identical (deterministic)
	for i := 1; i < len(contents); i++ {
		if contents[i] != contents[0] {
			t.Errorf("wrapper content not deterministic: run %d differs from run 0", i)
		}
	}

	// Verify alphabetical order: alpha, middle, zebra
	if !strings.Contains(contents[0], "alpha-2.0.0") {
		t.Errorf("wrapper should contain alpha dep")
	}
	alphaIdx := strings.Index(contents[0], "alpha")
	middleIdx := strings.Index(contents[0], "middle")
	zebraIdx := strings.Index(contents[0], "zebra")

	if alphaIdx > middleIdx || middleIdx > zebraIdx {
		t.Errorf("deps should be in alphabetical order: alpha=%d, middle=%d, zebra=%d",
			alphaIdx, middleIdx, zebraIdx)
	}
}

func TestCreateWrappersForBinaries_MultipleBinaries(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory with multiple binaries
	toolDir := cfg.ToolDir("multitool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create fake binaries
	binaries := []string{"bin/tool1", "bin/tool2", "bin/tool3"}
	for _, bin := range binaries {
		binPath := filepath.Join(toolDir, bin)
		if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
			t.Fatalf("failed to create binary %s: %v", bin, err)
		}
	}

	runtimeDeps := map[string]string{"nodejs": "20.0.0"}

	err := mgr.createWrappersForBinaries("multitool", "1.0.0", binaries, runtimeDeps)
	if err != nil {
		t.Fatalf("createWrappersForBinaries() error = %v", err)
	}

	// Verify all wrappers were created
	for _, bin := range binaries {
		binaryName := filepath.Base(bin)
		wrapperPath := cfg.CurrentSymlink(binaryName)
		if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
			t.Errorf("wrapper for %s not created", binaryName)
		}
	}
}

func TestCreateWrappersForBinaries_FallbackToToolName(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory
	toolDir := cfg.ToolDir("simpletool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create binary with tool name
	binaryPath := filepath.Join(binDir, "simpletool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	runtimeDeps := map[string]string{"python": "3.11.0"}

	// Call with empty binaries slice - should fallback to bin/simpletool
	err := mgr.createWrappersForBinaries("simpletool", "1.0.0", nil, runtimeDeps)
	if err != nil {
		t.Fatalf("createWrappersForBinaries() error = %v", err)
	}

	// Verify wrapper was created with tool name
	wrapperPath := cfg.CurrentSymlink("simpletool")
	if _, err := os.Stat(wrapperPath); os.IsNotExist(err) {
		t.Errorf("wrapper for simpletool not created")
	}
}

func TestCreateBinaryWrapper_AtomicWrite(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory
	toolDir := cfg.ToolDir("atomictool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create an existing wrapper (to test replacement)
	wrapperPath := cfg.CurrentSymlink("atomictool")
	if err := os.WriteFile(wrapperPath, []byte("old content"), 0755); err != nil {
		t.Fatalf("failed to create old wrapper: %v", err)
	}

	runtimeDeps := map[string]string{"dep": "1.0.0"}

	// Create new wrapper (should atomically replace)
	err := mgr.createBinaryWrapper("atomictool", "1.0.0", "bin/atomictool", runtimeDeps)
	if err != nil {
		t.Fatalf("createBinaryWrapper() error = %v", err)
	}

	// Verify new content
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}
	if string(content) == "old content" {
		t.Errorf("wrapper was not replaced")
	}
	if !strings.HasPrefix(string(content), "#!/bin/sh") {
		t.Errorf("wrapper should have new content with shebang")
	}

	// Verify no temp file left behind
	tmpPath := wrapperPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after successful write")
	}
}

func TestCreateBinaryWrapper_NoDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory
	toolDir := cfg.ToolDir("nodeptool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create wrapper with nil runtime deps
	err := mgr.createBinaryWrapper("nodeptool", "1.0.0", "bin/nodeptool", nil)
	if err != nil {
		t.Fatalf("createBinaryWrapper() error = %v", err)
	}

	// Verify wrapper was created without PATH modification
	wrapperPath := cfg.CurrentSymlink("nodeptool")
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "PATH=") {
		t.Errorf("wrapper should not have PATH= when no deps")
	}
	if !strings.Contains(contentStr, "exec") {
		t.Errorf("wrapper should contain exec")
	}
}

func TestCreateBinaryWrapper_EmptyDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory
	toolDir := cfg.ToolDir("emptydeptool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create wrapper with empty runtime deps map
	err := mgr.createBinaryWrapper("emptydeptool", "1.0.0", "bin/emptydeptool", map[string]string{})
	if err != nil {
		t.Fatalf("createBinaryWrapper() error = %v", err)
	}

	// Verify wrapper was created without PATH modification
	wrapperPath := cfg.CurrentSymlink("emptydeptool")
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "PATH=") {
		t.Errorf("wrapper should not have PATH= when empty deps")
	}
}

func TestCreateWrappersForBinaries_ErrorPropagation(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Remove current dir to force write error
	if err := os.RemoveAll(cfg.CurrentDir); err != nil {
		t.Fatalf("failed to remove current dir: %v", err)
	}

	runtimeDeps := map[string]string{"nodejs": "20.0.0"}

	// This should fail because current dir doesn't exist
	err := mgr.createWrappersForBinaries("failtool", "1.0.0", []string{"bin/tool"}, runtimeDeps)
	if err == nil {
		t.Error("createWrappersForBinaries() should fail when current dir doesn't exist")
	}
}

func TestCreateBinaryWrapper_WriteError(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tool directory
	toolDir := cfg.ToolDir("writefail", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Remove current dir to force write error
	if err := os.RemoveAll(cfg.CurrentDir); err != nil {
		t.Fatalf("failed to remove current dir: %v", err)
	}

	runtimeDeps := map[string]string{"dep": "1.0.0"}

	err := mgr.createBinaryWrapper("writefail", "1.0.0", "bin/writefail", runtimeDeps)
	if err == nil {
		t.Error("createBinaryWrapper() should fail when current dir doesn't exist")
	}
	if !strings.Contains(err.Error(), "failed to write wrapper script") {
		t.Errorf("error should mention write failure, got: %v", err)
	}
}

func TestInstallWithOptions_WithRuntimeDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory simulating executor output
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure in work dir
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(installBinDir, "mytool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	opts := InstallOptions{
		CreateSymlinks:      true,
		IsHidden:            false,
		Binaries:            []string{"bin/mytool"},
		RuntimeDependencies: map[string]string{"nodejs": "20.10.0"},
	}

	err := mgr.InstallWithOptions("mytool", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify wrapper was created (not symlink)
	wrapperPath := cfg.CurrentSymlink("mytool")
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	// Should be a wrapper script, not a symlink
	if !strings.HasPrefix(string(content), "#!/bin/sh") {
		t.Errorf("expected wrapper script, got: %s", content)
	}
	if !strings.Contains(string(content), "PATH=") {
		t.Errorf("wrapper should contain PATH modification")
	}
	if !strings.Contains(string(content), "nodejs-20.10.0") {
		t.Errorf("wrapper should contain nodejs dependency path")
	}

	// Verify state was updated
	state, err := mgr.GetState().Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	toolState, ok := state.Installed["mytool"]
	if !ok {
		t.Error("tool should be in state")
	}
	if toolState.Version != "1.0.0" {
		t.Errorf("version = %s, want 1.0.0", toolState.Version)
	}
}

func TestInstallWithOptions_NoRuntimeDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory simulating executor output
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure in work dir
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(installBinDir, "simpletool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	opts := InstallOptions{
		CreateSymlinks: true,
		IsHidden:       false,
		Binaries:       []string{"bin/simpletool"},
		// No RuntimeDependencies - should use symlinks
	}

	err := mgr.InstallWithOptions("simpletool", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify symlink was created (not wrapper)
	symlinkPath := cfg.CurrentSymlink("simpletool")
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("failed to stat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}
}

func TestInstallWithOptions_MultipleBinariesWithDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create multiple binaries
	for _, name := range []string{"tool1", "tool2"} {
		binaryPath := filepath.Join(installBinDir, name)
		if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho "+name), 0755); err != nil {
			t.Fatalf("failed to create binary %s: %v", name, err)
		}
	}

	opts := InstallOptions{
		CreateSymlinks:      true,
		IsHidden:            false,
		Binaries:            []string{"bin/tool1", "bin/tool2"},
		RuntimeDependencies: map[string]string{"python": "3.11.0"},
	}

	err := mgr.InstallWithOptions("multitool", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify both wrappers were created
	for _, name := range []string{"tool1", "tool2"} {
		wrapperPath := cfg.CurrentSymlink(name)
		content, err := os.ReadFile(wrapperPath)
		if err != nil {
			t.Fatalf("failed to read wrapper %s: %v", name, err)
		}
		if !strings.HasPrefix(string(content), "#!/bin/sh") {
			t.Errorf("expected wrapper script for %s", name)
		}
	}
}

func TestInstallWithOptions_HiddenTool(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create binary
	binaryPath := filepath.Join(installBinDir, "hiddentool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	opts := InstallOptions{
		CreateSymlinks: false, // Hidden tool - no symlinks
		IsHidden:       true,
		Binaries:       []string{"bin/hiddentool"},
	}

	err := mgr.InstallWithOptions("hiddentool", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify no symlink/wrapper was created in current/
	symlinkPath := cfg.CurrentSymlink("hiddentool")
	if _, err := os.Stat(symlinkPath); !os.IsNotExist(err) {
		t.Error("hidden tool should not have symlink in current/")
	}

	// Verify state marks it as hidden
	state, err := mgr.GetState().Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	toolState := state.Installed["hiddentool"]
	if !toolState.IsHidden {
		t.Error("tool should be marked as hidden")
	}
	if !toolState.IsExecutionDependency {
		t.Error("tool should be marked as execution dependency")
	}
}

func TestInstallWithOptions_NoBinariesFallback(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create binary with same name as tool (fallback case)
	binaryPath := filepath.Join(installBinDir, "fallbacktool")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	opts := InstallOptions{
		CreateSymlinks: true,
		IsHidden:       false,
		// No Binaries specified - should fallback to bin/toolname
	}

	err := mgr.InstallWithOptions("fallbacktool", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify symlink was created with tool name
	symlinkPath := cfg.CurrentSymlink("fallbacktool")
	if _, err := os.Lstat(symlinkPath); err != nil {
		t.Errorf("symlink should exist at %s: %v", symlinkPath, err)
	}
}

func TestInstallWithOptions_NoBinariesWithRuntimeDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create a work directory
	workDir, workCleanup := testutil.TempDir(t)
	defer workCleanup()

	// Create .install/bin structure
	installBinDir := filepath.Join(workDir, ".install", "bin")
	if err := os.MkdirAll(installBinDir, 0755); err != nil {
		t.Fatalf("failed to create install bin dir: %v", err)
	}

	// Create binary with same name as tool
	binaryPath := filepath.Join(installBinDir, "depfallback")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	opts := InstallOptions{
		CreateSymlinks:      true,
		IsHidden:            false,
		RuntimeDependencies: map[string]string{"nodejs": "20.0.0"},
		// No Binaries specified - should fallback to bin/toolname and create wrapper
	}

	err := mgr.InstallWithOptions("depfallback", "1.0.0", workDir, opts)
	if err != nil {
		t.Fatalf("InstallWithOptions() error = %v", err)
	}

	// Verify wrapper was created with tool name
	wrapperPath := cfg.CurrentSymlink("depfallback")
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	// Should be a wrapper, not symlink
	if !strings.HasPrefix(string(content), "#!/bin/sh") {
		t.Errorf("expected wrapper script, got symlink or other")
	}
}
