package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- homebrew_relocate.go: Dependencies, extractBottlePrefixes --

func TestHomebrewRelocateAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := HomebrewRelocateAction{}
	deps := action.Dependencies()
	if len(deps.LinuxInstallTime) != 1 || deps.LinuxInstallTime[0] != "patchelf" {
		t.Errorf("Dependencies().LinuxInstallTime = %v, want [patchelf]", deps.LinuxInstallTime)
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	content := []byte(`some text /tmp/action-validator-abc12345/.install/libyaml/0.2.5/lib/libyaml.so more text
another line /tmp/action-validator-abc12345/.install/libyaml/0.2.5/include/yaml.h end`)

	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)

	if len(prefixMap) != 2 {
		t.Errorf("extractBottlePrefixes() found %d entries, want 2", len(prefixMap))
	}

	expectedPrefix := "/tmp/action-validator-abc12345/.install/libyaml/0.2.5"
	for fullPath, prefix := range prefixMap {
		if prefix != expectedPrefix {
			t.Errorf("prefix for %q = %q, want %q", fullPath, prefix, expectedPrefix)
		}
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoMatch(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes([]byte("no bottle paths here"), prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-match content, want 0", len(prefixMap))
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoInstallSegment(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	// Has the marker but no /.install/ segment
	content := []byte("/tmp/action-validator-abc12345/other/path")
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-install content, want 0", len(prefixMap))
	}
}

// -- findPatchelf discovery tests --

func TestFindPatchelf_ExecPaths(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Create a temporary bin dir with a fake patchelf
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(binDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		ExecPaths: []string{binDir},
	}

	got, err := action.findPatchelf(ctx)
	if err != nil {
		t.Fatalf("findPatchelf() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelf() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelf_NotFound_ReturnsError(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()
	action := &HomebrewRelocateAction{}

	// Override PATH so system patchelf isn't found
	t.Setenv("PATH", t.TempDir())

	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		ToolsDir:   filepath.Join(tmpDir, "tools"),
		CurrentDir: filepath.Join(tmpDir, "tools", "current"),
	}

	_, err := action.findPatchelf(ctx)
	if err == nil {
		t.Fatal("findPatchelf() should return error when patchelf not found anywhere")
	}
	if !strings.Contains(err.Error(), "patchelf not found") {
		t.Errorf("error message = %q, want it to contain 'patchelf not found'", err.Error())
	}
}

// -- findPatchelfInToolsDir tests (test glob/current fallback directly) --

func TestFindPatchelfInToolsDir_Glob(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Simulate $TSUKU_HOME/tools/patchelf-0.18.0/bin/patchelf
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	patchelfBinDir := filepath.Join(toolsDir, "patchelf-0.18.0", "bin")
	if err := os.MkdirAll(patchelfBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(patchelfBinDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := action.findPatchelfInToolsDir(toolsDir, filepath.Join(toolsDir, "current"))
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelfInToolsDir_CurrentDir(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Simulate $TSUKU_HOME/tools/current/patchelf (no versioned dir)
	tmpDir := t.TempDir()
	currentDir := filepath.Join(tmpDir, "tools", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(currentDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := action.findPatchelfInToolsDir(filepath.Join(tmpDir, "tools"), currentDir)
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelfInToolsDir_PicksLatestVersion(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Create two patchelf versions
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	for _, ver := range []string{"0.17.0", "0.18.0"} {
		binDir := filepath.Join(toolsDir, "patchelf-"+ver, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "patchelf"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := action.findPatchelfInToolsDir(toolsDir, filepath.Join(toolsDir, "current"))
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	// Should pick 0.18.0 (last in lexicographic order)
	want := filepath.Join(toolsDir, "patchelf-0.18.0", "bin", "patchelf")
	if got != want {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q (latest version)", got, want)
	}
}

func TestFindPatchelfInToolsDir_NotFound(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	tmpDir := t.TempDir()
	_, err := action.findPatchelfInToolsDir(filepath.Join(tmpDir, "tools"), filepath.Join(tmpDir, "tools", "current"))
	if err == nil {
		t.Fatal("findPatchelfInToolsDir() should return error when patchelf not found")
	}
}
