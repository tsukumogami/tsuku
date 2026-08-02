package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// postInstallHome sets up a $TSUKU_HOME with one installed version of a tool
// that has already written its shell.d fragment but not yet recorded it.
func postInstallHome(t *testing.T) (*config.Config, *install.Manager, string) {
	t.Helper()

	home := t.TempDir()
	cfg := &config.Config{
		HomeDir:    home,
		ToolsDir:   filepath.Join(home, "tools"),
		CurrentDir: filepath.Join(home, "tools", "current"),
		ShareDir:   filepath.Join(home, "share"),
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatal(err)
	}
	shellDDir := filepath.Join(home, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shellDDir, "mytool@1.0.0.bash"), []byte("# v1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	mgr := install.New(cfg)
	if err := mgr.GetState().UpdateTool("mytool", func(ts *install.ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]install.VersionState{"1.0.0": {}}
	}); err != nil {
		t.Fatal(err)
	}
	return cfg, mgr, shellDDir
}

// `tsuku install --plan` never called RecordCleanup, so the fragments it wrote
// were orphaned: invisible to remove, to doctor's hash check, and to the
// active-version projection. Both install paths now go through one helper.
func TestFinishPostInstall_RecordsCleanupAndRebuilds(t *testing.T) {
	cfg, mgr, shellDDir := postInstallHome(t)

	sum := sha256.Sum256([]byte("# v1\n"))
	cleanup := []actions.CleanupAction{{
		Action:      "delete_file",
		Path:        "share/shell.d/mytool@1.0.0.bash",
		ContentHash: hex.EncodeToString(sum[:]),
	}}

	var warnings []string
	finishPostInstall(cfg, mgr, "mytool", "1.0.0", cleanup, func(format string, args ...interface{}) {
		warnings = append(warnings, format)
	})
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	ts, err := mgr.GetToolState("mytool")
	if err != nil || ts == nil {
		t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	recorded := ts.Versions["1.0.0"].CleanupActions
	if len(recorded) != 1 || recorded[0].Path != "share/shell.d/mytool@1.0.0.bash" {
		t.Fatalf("cleanup actions = %v, want the fragment recorded", recorded)
	}

	cache, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}
	if !strings.Contains(string(cache), "# v1") {
		t.Errorf("cache should hold the fragment, got %q", cache)
	}
	// The cache must name the tool, not the versioned filename.
	if strings.Contains(string(cache), "mytool@1.0.0") {
		t.Errorf("cache should display the tool name without the version key, got %q", cache)
	}
}

// Recording has to happen before the rebuild: the selection the rebuild uses is
// read back out of state, so a rebuild that runs first cannot see the paths
// this install just wrote and would verify no hashes.
func TestFinishPostInstall_RebuildUsesTheJustRecordedSelection(t *testing.T) {
	cfg, mgr, shellDDir := postInstallHome(t)

	// A tampered fragment: the recorded hash describes different bytes.
	cleanup := []actions.CleanupAction{{
		Action:      "delete_file",
		Path:        "share/shell.d/mytool@1.0.0.bash",
		ContentHash: "0000000000000000000000000000000000000000000000000000000000000000",
	}}

	finishPostInstall(cfg, mgr, "mytool", "1.0.0", cleanup, func(string, ...interface{}) {})

	// A hash mismatch keeps the fragment out of the cache, which only happens
	// if the rebuild saw the record.
	if _, err := os.Stat(filepath.Join(shellDDir, ".init-cache.bash")); !os.IsNotExist(err) {
		t.Error("expected the hash-mismatched fragment to be excluded, leaving no cache")
	}
}

func TestFinishPostInstall_NoCleanupIsANoOp(t *testing.T) {
	cfg, mgr, shellDDir := postInstallHome(t)

	finishPostInstall(cfg, mgr, "mytool", "1.0.0", nil, func(format string, args ...interface{}) {
		t.Errorf("unexpected warning: "+format, args...)
	})

	if _, err := os.Stat(filepath.Join(shellDDir, ".init-cache.bash")); !os.IsNotExist(err) {
		t.Error("an install that wrote nothing should not touch the cache")
	}
}

func TestActiveCleanupPaths(t *testing.T) {
	tests := []struct {
		name  string
		state *install.ToolState
		want  []string
	}{
		{"nil", nil, nil},
		{
			name: "only the active version's paths",
			state: &install.ToolState{
				ActiveVersion: "2.0.0",
				Versions: map[string]install.VersionState{
					"1.0.0": {CleanupActions: []install.CleanupAction{{Path: "share/shell.d/t@1.0.0.bash"}}},
					"2.0.0": {CleanupActions: []install.CleanupAction{{Path: "share/shell.d/t@2.0.0.bash"}}},
				},
			},
			want: []string{"share/shell.d/t@2.0.0.bash"},
		},
		{
			name: "legacy state falls back to the deprecated Version field",
			state: &install.ToolState{
				Version: "1.0.0",
				Versions: map[string]install.VersionState{
					"1.0.0": {CleanupActions: []install.CleanupAction{{Path: "share/shell.d/t.bash"}}},
				},
			},
			want: []string{"share/shell.d/t.bash"},
		},
		{
			name:  "an active version with no version state",
			state: &install.ToolState{ActiveVersion: "3.0.0"},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := activeCleanupPaths(tt.state)
			if len(got) != len(tt.want) {
				t.Fatalf("activeCleanupPaths() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("activeCleanupPaths()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
