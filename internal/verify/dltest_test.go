package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

func TestDlopenResult_JSONParsing_Success(t *testing.T) {
	input := `[{"path":"/lib/libc.so.6","ok":true}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/lib/libc.so.6" {
		t.Errorf("Path = %q, want %q", r.Path, "/lib/libc.so.6")
	}
	if !r.OK {
		t.Error("OK = false, want true")
	}
	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
}

func TestDlopenResult_JSONParsing_Failure(t *testing.T) {
	input := `[{"path":"/nonexistent.so","ok":false,"error":"cannot open shared object file"}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/nonexistent.so" {
		t.Errorf("Path = %q, want %q", r.Path, "/nonexistent.so")
	}
	if r.OK {
		t.Error("OK = true, want false")
	}
	if r.Error != "cannot open shared object file" {
		t.Errorf("Error = %q, want %q", r.Error, "cannot open shared object file")
	}
}

func TestDlopenResult_JSONParsing_Mixed(t *testing.T) {
	input := `[
		{"path":"/lib/libc.so.6","ok":true},
		{"path":"/lib/libpthread.so.0","ok":true},
		{"path":"/nonexistent.so","ok":false,"error":"not found"}
	]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First two should be OK
	if !results[0].OK || !results[1].OK {
		t.Error("first two results should be OK")
	}

	// Third should be failure
	if results[2].OK {
		t.Error("third result should be failure")
	}
	if results[2].Error != "not found" {
		t.Errorf("Error = %q, want %q", results[2].Error, "not found")
	}
}

func TestDlopenResult_JSONParsing_Empty(t *testing.T) {
	input := `[]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestEnsureDltest_NotInstalled(t *testing.T) {
	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Test the state check logic directly instead of calling EnsureDltest,
	// which would try to invoke tsuku to install (causing test recursion).
	// When nothing is installed, GetToolState should return nil (not an error).
	stateManager := install.NewStateManager(cfg)
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		t.Fatalf("GetToolState failed unexpectedly: %v", err)
	}
	if toolState != nil {
		t.Error("expected nil toolState for uninstalled tool")
	}

	// Verify the version check logic: when no state exists, installedVersion is empty
	var installedVersion string
	if toolState != nil {
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
	}
	if installedVersion != "" {
		t.Errorf("installedVersion = %q, want empty string", installedVersion)
	}

	// Verify this is NOT the pinned version (so installation would be triggered)
	if installedVersion == pinnedDltestVersion {
		t.Error("empty version should not match pinnedDltestVersion")
	}
}

func TestEnsureDltest_CorrectVersionInstalled(t *testing.T) {
	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Create the tool directory and binary
	version := pinnedDltestVersion
	binDir := cfg.ToolBinDir("tsuku-dltest", version)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a fake binary
	binaryPath := filepath.Join(binDir, "tsuku-dltest")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Set up state to show tool is installed
	stateManager := install.NewStateManager(cfg)
	if err := stateManager.UpdateTool("tsuku-dltest", func(ts *install.ToolState) {
		ts.ActiveVersion = version
		ts.IsHidden = true
	}); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// EnsureDltest should return path without trying to install
	path, err := EnsureDltest(cfg)
	if err != nil {
		t.Fatalf("EnsureDltest failed: %v", err)
	}

	if path != binaryPath {
		t.Errorf("path = %q, want %q", path, binaryPath)
	}
}

func TestEnsureDltest_WrongVersionInstalled(t *testing.T) {
	// Create a temp directory for test
	tmpDir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with temp directory
	cfg := &config.Config{
		HomeDir:  tmpDir,
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	// Set up state with wrong version
	stateManager := install.NewStateManager(cfg)
	wrongVersion := "v0.0.0-wrong"
	if err := stateManager.UpdateTool("tsuku-dltest", func(ts *install.ToolState) {
		ts.ActiveVersion = wrongVersion
		ts.IsHidden = true
	}); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Test the version check logic directly instead of calling EnsureDltest,
	// which would try to invoke tsuku to install (causing test recursion).
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		t.Fatalf("GetToolState failed: %v", err)
	}
	if toolState == nil {
		t.Fatal("expected non-nil toolState for installed tool")
	}

	// Verify the version detection logic works
	var installedVersion string
	if toolState.ActiveVersion != "" {
		installedVersion = toolState.ActiveVersion
	} else {
		installedVersion = toolState.Version
	}

	if installedVersion != wrongVersion {
		t.Errorf("installedVersion = %q, want %q", installedVersion, wrongVersion)
	}

	// Verify wrong version does NOT match pinned version (so installation would be triggered)
	if installedVersion == pinnedDltestVersion {
		t.Errorf("wrong version %q should not match pinnedDltestVersion %q",
			installedVersion, pinnedDltestVersion)
	}
}
