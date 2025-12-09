package install

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

// TestRemoveVersion_Single tests removing a single version when multiple are installed.
func TestRemoveVersion_Single(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state with two versions
	v1Time := time.Now().Add(-1 * time.Hour)
	v2Time := time.Now()
	err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: v1Time},
			"2.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: v2Time},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Create tool directories
	for _, v := range []string{"1.0.0", "2.0.0"} {
		toolDir := cfg.ToolDir("mytool", v)
		binDir := filepath.Join(toolDir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("failed to create bin dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "mytool"), []byte("#!/bin/sh\necho "+v), 0755); err != nil {
			t.Fatalf("failed to create binary: %v", err)
		}
	}

	// Create symlink for active version
	symlinkPath := cfg.CurrentSymlink("mytool")
	targetPath := filepath.Join(cfg.ToolDir("mytool", "2.0.0"), "bin", "mytool")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Remove version 1.0.0 (not active)
	err = mgr.RemoveVersion("mytool", "1.0.0")
	if err != nil {
		t.Fatalf("RemoveVersion() error = %v", err)
	}

	// Verify state
	state, err := mgr.state.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	toolState := state.Installed["mytool"]

	// Should have only one version now
	if len(toolState.Versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(toolState.Versions))
	}

	// 1.0.0 should be gone
	if _, exists := toolState.Versions["1.0.0"]; exists {
		t.Error("version 1.0.0 should be removed")
	}

	// Active version should still be 2.0.0
	if toolState.ActiveVersion != "2.0.0" {
		t.Errorf("active_version = %s, want 2.0.0", toolState.ActiveVersion)
	}

	// Directory should be removed
	if _, err := os.Stat(cfg.ToolDir("mytool", "1.0.0")); !os.IsNotExist(err) {
		t.Error("version 1.0.0 directory should be removed")
	}

	// Version 2.0.0 directory should still exist
	if _, err := os.Stat(cfg.ToolDir("mytool", "2.0.0")); os.IsNotExist(err) {
		t.Error("version 2.0.0 directory should still exist")
	}
}

// TestRemoveVersion_ActiveSwitchesToMostRecent tests removing the active version switches to most recent.
func TestRemoveVersion_ActiveSwitchesToMostRecent(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state: v1 is older, v2 is active, v3 is newest
	v1Time := time.Now().Add(-2 * time.Hour)
	v2Time := time.Now().Add(-1 * time.Hour)
	v3Time := time.Now()
	err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: v1Time},
			"2.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: v2Time},
			"3.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: v3Time},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Create tool directories and symlinks
	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		toolDir := cfg.ToolDir("mytool", v)
		binDir := filepath.Join(toolDir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("failed to create bin dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "mytool"), []byte("#!/bin/sh\necho "+v), 0755); err != nil {
			t.Fatalf("failed to create binary: %v", err)
		}
	}

	// Remove active version (2.0.0)
	err = mgr.RemoveVersion("mytool", "2.0.0")
	if err != nil {
		t.Fatalf("RemoveVersion() error = %v", err)
	}

	// Verify state
	state, err := mgr.state.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	toolState := state.Installed["mytool"]

	// Should have 2 versions now
	if len(toolState.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(toolState.Versions))
	}

	// Active version should switch to 3.0.0 (most recent)
	if toolState.ActiveVersion != "3.0.0" {
		t.Errorf("active_version = %s, want 3.0.0 (most recent)", toolState.ActiveVersion)
	}

	// Symlink should point to 3.0.0
	symlinkPath := cfg.CurrentSymlink("mytool")
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if target != filepath.Join(cfg.ToolDir("mytool", "3.0.0"), "bin", "mytool") {
		t.Errorf("symlink should point to 3.0.0, got %s", target)
	}
}

// TestRemoveVersion_LastVersion tests removing the last version removes tool entirely.
func TestRemoveVersion_LastVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state with one version
	err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: time.Now()},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Create tool directory
	toolDir := cfg.ToolDir("mytool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "mytool"), []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	// Create symlink
	symlinkPath := cfg.CurrentSymlink("mytool")
	targetPath := filepath.Join(toolDir, "bin", "mytool")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Remove the only version
	err = mgr.RemoveVersion("mytool", "1.0.0")
	if err != nil {
		t.Fatalf("RemoveVersion() error = %v", err)
	}

	// Verify tool is gone from state
	state, err := mgr.state.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if _, exists := state.Installed["mytool"]; exists {
		t.Error("tool should be removed from state when last version is removed")
	}

	// Directory should be removed
	if _, err := os.Stat(toolDir); !os.IsNotExist(err) {
		t.Error("tool directory should be removed")
	}

	// Symlink should be removed
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
}

// TestRemoveVersion_NotInstalled tests error when version doesn't exist.
func TestRemoveVersion_NotInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state with one version
	err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/mytool"}},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Try to remove non-existent version
	err = mgr.RemoveVersion("mytool", "2.0.0")
	if err == nil {
		t.Error("RemoveVersion should error for non-existent version")
	}

	// Error should mention available versions
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

// TestRemoveVersion_InvalidVersion tests error for path traversal attempts.
func TestRemoveVersion_InvalidVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	tests := []struct {
		name    string
		version string
	}{
		{"path traversal", "../etc/passwd"},
		{"forward slash", "1.0/2.0"},
		{"backslash", "1.0\\2.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := mgr.RemoveVersion("sometool", tc.version)
			if err == nil {
				t.Error("RemoveVersion should error for invalid version")
			}
		})
	}
}

// TestRemoveAllVersions tests removing all versions of a tool.
func TestRemoveAllVersions(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state with two versions
	err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: time.Now().Add(-1 * time.Hour)},
			"2.0.0": {Binaries: []string{"bin/mytool"}, InstalledAt: time.Now()},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Create tool directories
	for _, v := range []string{"1.0.0", "2.0.0"} {
		toolDir := cfg.ToolDir("mytool", v)
		binDir := filepath.Join(toolDir, "bin")
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatalf("failed to create bin dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "mytool"), []byte("#!/bin/sh"), 0755); err != nil {
			t.Fatalf("failed to create binary: %v", err)
		}
	}

	// Create symlink
	symlinkPath := cfg.CurrentSymlink("mytool")
	targetPath := filepath.Join(cfg.ToolDir("mytool", "2.0.0"), "bin", "mytool")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Remove all versions
	err = mgr.RemoveAllVersions("mytool")
	if err != nil {
		t.Fatalf("RemoveAllVersions() error = %v", err)
	}

	// Verify tool is gone from state
	state, err := mgr.state.Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if _, exists := state.Installed["mytool"]; exists {
		t.Error("tool should be removed from state")
	}

	// Both directories should be removed
	for _, v := range []string{"1.0.0", "2.0.0"} {
		if _, err := os.Stat(cfg.ToolDir("mytool", v)); !os.IsNotExist(err) {
			t.Errorf("version %s directory should be removed", v)
		}
	}

	// Symlink should be removed
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
}

// TestRemoveAllVersions_ToolNotInstalled tests error when tool doesn't exist.
func TestRemoveAllVersions_ToolNotInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	err := mgr.RemoveAllVersions("nonexistent")
	if err == nil {
		t.Error("RemoveAllVersions should error for non-existent tool")
	}
}

// TestGetMostRecentVersion tests the getMostRecentVersion helper.
func TestGetMostRecentVersion(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		versions map[string]VersionState
		want     string
	}{
		{
			name: "single version",
			versions: map[string]VersionState{
				"1.0.0": {InstalledAt: now},
			},
			want: "1.0.0",
		},
		{
			name: "multiple versions",
			versions: map[string]VersionState{
				"1.0.0": {InstalledAt: now.Add(-2 * time.Hour)},
				"2.0.0": {InstalledAt: now.Add(-1 * time.Hour)},
				"3.0.0": {InstalledAt: now},
			},
			want: "3.0.0",
		},
		{
			name: "older version is most recent",
			versions: map[string]VersionState{
				"1.0.0": {InstalledAt: now},
				"2.0.0": {InstalledAt: now.Add(-1 * time.Hour)},
			},
			want: "1.0.0",
		},
		{
			name:     "empty map",
			versions: map[string]VersionState{},
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getMostRecentVersion(tc.versions)
			if got != tc.want {
				t.Errorf("getMostRecentVersion() = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestRemoveVersion_MultipleBinaries tests removing a tool with multiple binaries.
func TestRemoveVersion_MultipleBinaries(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Set up state with multiple binaries
	err := mgr.state.UpdateTool("multitool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/tool1", "bin/tool2"}, InstalledAt: time.Now()},
		}
		ts.Binaries = []string{"bin/tool1", "bin/tool2"}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Create tool directory
	toolDir := cfg.ToolDir("multitool", "1.0.0")
	binDir := filepath.Join(toolDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	for _, bin := range []string{"tool1", "tool2"} {
		if err := os.WriteFile(filepath.Join(binDir, bin), []byte("#!/bin/sh"), 0755); err != nil {
			t.Fatalf("failed to create binary: %v", err)
		}
		// Create symlink
		symlinkPath := cfg.CurrentSymlink(bin)
		if err := os.Symlink(filepath.Join(binDir, bin), symlinkPath); err != nil {
			t.Fatalf("failed to create symlink for %s: %v", bin, err)
		}
	}

	// Remove the tool
	err = mgr.RemoveVersion("multitool", "1.0.0")
	if err != nil {
		t.Fatalf("RemoveVersion() error = %v", err)
	}

	// Both symlinks should be removed
	for _, bin := range []string{"tool1", "tool2"} {
		symlinkPath := cfg.CurrentSymlink(bin)
		if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
			t.Errorf("symlink for %s should be removed", bin)
		}
	}
}
