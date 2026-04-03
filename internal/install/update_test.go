package install

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestStaleCleanupActions(t *testing.T) {
	tests := []struct {
		name string
		old  []CleanupAction
		new  []CleanupAction
		want []CleanupAction
	}{
		{
			name: "empty old returns nil",
			old:  nil,
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
			},
			want: nil,
		},
		{
			name: "empty new returns all old",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
			new: nil,
			want: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
		},
		{
			name: "identical actions returns nil",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
			want: nil,
		},
		{
			name: "stale paths detected",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
				{Action: "delete_file", Path: "share/completions/bash/tool"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
			want: []CleanupAction{
				{Action: "delete_file", Path: "share/completions/bash/tool"},
			},
		},
		{
			name: "new version adds paths not in old",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
			},
			want: nil,
		},
		{
			name: "action type matters in comparison",
			old: []CleanupAction{
				{Action: "delete_dir", Path: "share/shell.d/tool.bash"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
			},
			want: []CleanupAction{
				{Action: "delete_dir", Path: "share/shell.d/tool.bash"},
			},
		},
		{
			name: "both old and new empty",
			old:  nil,
			new:  nil,
			want: nil,
		},
		{
			name: "old is empty slice not nil",
			old:  []CleanupAction{},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash"},
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StaleCleanupActions(tc.old, tc.new)

			if tc.want == nil {
				if got != nil {
					t.Errorf("StaleCleanupActions() = %v, want nil", got)
				}
				return
			}

			if len(got) != len(tc.want) {
				t.Fatalf("StaleCleanupActions() returned %d actions, want %d", len(got), len(tc.want))
			}
			for i, want := range tc.want {
				if got[i].Action != want.Action || got[i].Path != want.Path {
					t.Errorf("StaleCleanupActions()[%d] = {%s, %s}, want {%s, %s}",
						i, got[i].Action, got[i].Path, want.Action, want.Path)
				}
			}
		})
	}
}

// TestExecuteStaleCleanup_DeletesStaleFiles tests that stale files are
// deleted and shell caches rebuilt during update.
func TestExecuteStaleCleanup_DeletesStaleFiles(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create shell.d directory with stale and current files
	shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatalf("failed to create shell.d dir: %v", err)
	}

	// Stale file (old version created it, new version doesn't)
	staleFile := filepath.Join(shellDDir, "tool.fish")
	if err := os.WriteFile(staleFile, []byte("# old fish init\n"), 0644); err != nil {
		t.Fatalf("failed to create stale file: %v", err)
	}

	// Current file (both versions create it -- not stale)
	currentFile := filepath.Join(shellDDir, "tool.bash")
	if err := os.WriteFile(currentFile, []byte("# bash init\n"), 0644); err != nil {
		t.Fatalf("failed to create current file: %v", err)
	}

	staleActions := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.fish"},
	}

	mgr.ExecuteStaleCleanup(staleActions)

	// Stale file should be deleted
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Error("stale file should be deleted")
	}

	// Current file should still exist
	if _, err := os.Stat(currentFile); os.IsNotExist(err) {
		t.Error("current file should not be deleted")
	}
}

// TestExecuteStaleCleanup_NoOpWhenEmpty tests that no work is done with empty actions.
func TestExecuteStaleCleanup_NoOpWhenEmpty(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Should not panic or error with nil/empty actions
	mgr.ExecuteStaleCleanup(nil)
	mgr.ExecuteStaleCleanup([]CleanupAction{})
}

// TestExecuteStaleCleanup_FailureDoesNotPanic tests that cleanup of
// non-existent files logs a warning but doesn't panic or error out.
func TestExecuteStaleCleanup_FailureDoesNotPanic(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Point to files that don't exist
	staleActions := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/nonexistent.bash"},
	}

	// Should not panic
	mgr.ExecuteStaleCleanup(staleActions)
}

// TestUpdateStaleCleanup_EndToEnd tests the full update stale cleanup flow:
// load old state, compute stale, execute cleanup.
func TestUpdateStaleCleanup_EndToEnd(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Simulate old version with shell.d files for bash, zsh, and fish
	shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatalf("failed to create shell.d dir: %v", err)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		f := filepath.Join(shellDDir, "tool."+shell)
		if err := os.WriteFile(f, []byte("# "+shell+" init\n"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Old version state: produced bash, zsh, fish
	oldActions := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
		{Action: "delete_file", Path: "share/shell.d/tool.fish"},
	}

	// New version state: only produces bash and zsh (dropped fish support)
	newActions := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh"},
	}

	// Set up state with old and new versions
	err := mgr.state.UpdateTool("tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {
				Binaries:       []string{"bin/tool"},
				InstalledAt:    time.Now().Add(-1 * time.Hour),
				CleanupActions: oldActions,
			},
			"2.0.0": {
				Binaries:       []string{"bin/tool"},
				InstalledAt:    time.Now(),
				CleanupActions: newActions,
			},
		}
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Load state and compute stale
	toolState, err := mgr.state.GetToolState("tool")
	if err != nil {
		t.Fatalf("failed to get tool state: %v", err)
	}

	oldVS := toolState.Versions["1.0.0"]
	newVS := toolState.Versions["2.0.0"]

	stale := StaleCleanupActions(oldVS.CleanupActions, newVS.CleanupActions)

	// Should find exactly one stale action: fish
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale action, got %d", len(stale))
	}
	if stale[0].Path != "share/shell.d/tool.fish" {
		t.Errorf("stale path = %s, want share/shell.d/tool.fish", stale[0].Path)
	}

	// Execute stale cleanup
	mgr.ExecuteStaleCleanup(stale)

	// Fish file should be gone
	fishFile := filepath.Join(shellDDir, "tool.fish")
	if _, err := os.Stat(fishFile); !os.IsNotExist(err) {
		t.Error("stale fish file should be deleted")
	}

	// Bash and zsh should remain
	for _, shell := range []string{"bash", "zsh"} {
		f := filepath.Join(shellDDir, "tool."+shell)
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("%s file should still exist", shell)
		}
	}
}
