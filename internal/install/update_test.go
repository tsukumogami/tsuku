package install

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
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

	// Set up state with only the new version installed. The old version has
	// already been reaped, so nothing still records its paths and its dropped
	// fish fragment really is orphaned.
	err := mgr.state.UpdateTool("tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
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

	stale := StaleCleanupActions(oldActions, newActions)

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

// recordWarns collects what a WarnFunc was handed, so a test can assert on the
// message a user would see rather than on a boolean.
func recordWarns(into *[]string) WarnFunc {
	return func(format string, args ...any) {
		*into = append(*into, fmt.Sprintf(format, args...))
	}
}

// TestWarnShellInitChanges covers the whole comparison in one table. The keying
// cases matter most: version-keyed filenames mean the same fragment has a
// different path in every version, so a comparison keyed on the raw path would
// never find a match and this warning would silently stop firing.
func TestWarnShellInitChanges(t *testing.T) {
	tests := []struct {
		name string
		tool string
		old  []CleanupAction
		new  []CleanupAction
		want []string
	}{
		{
			name: "matching hashes say nothing",
			tool: "tool",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
			},
		},
		{
			name: "only the shell whose hash changed is named",
			tool: "tool",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "changed"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
			},
			want: []string{"shell init changed for tool (bash)"},
		},
		{
			name: "every changed shell is named",
			tool: "tool",
			old: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "hash1"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "hash2"},
				{Action: "delete_file", Path: "share/shell.d/tool.fish", ContentHash: "hash3"},
			},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "changed1"},
				{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "changed2"},
				{Action: "delete_file", Path: "share/shell.d/tool.fish", ContentHash: "hash3"},
			},
			want: []string{
				"shell init changed for tool (bash)",
				"shell init changed for tool (zsh)",
			},
		},
		{
			name: "a shell the old version did not have is an addition, not a change",
			tool: "tool",
			old:  []CleanupAction{},
			new: []CleanupAction{
				{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
			},
		},
		{
			name: "actions without a recorded hash are skipped",
			tool: "tool",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/tool.bash"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/tool.bash"}},
		},
		{
			name: "same target and shell across version keys, content changed",
			tool: "nvm",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.5.bash", ContentHash: "old"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.6.bash", ContentHash: "new"}},
			want: []string{"shell init changed for nvm (bash)"},
		},
		{
			name: "same target and shell across version keys, content unchanged",
			tool: "nvm",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.5.bash", ContentHash: "same"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.6.bash", ContentHash: "same"}},
		},
		{
			name: "a different target is a new fragment, not a change",
			tool: "nvm",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.5.bash", ContentHash: "old"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/00-env-nvm@0.40.6.bash", ContentHash: "new"}},
		},
		{
			name: "a different shell is a new fragment, not a change",
			tool: "nvm",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.5.bash", ContentHash: "old"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/shell.d/nvm@0.40.6.zsh", ContentHash: "new"}},
		},
		{
			name: "a path outside shell.d is not a shell init fragment",
			tool: "tool",
			old:  []CleanupAction{{Action: "delete_file", Path: "share/completions/zsh/_tool", ContentHash: "old"}},
			new:  []CleanupAction{{Action: "delete_file", Path: "share/completions/zsh/_tool", ContentHash: "new"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			warnShellInitChanges(tt.tool, tt.old, tt.new, recordWarns(&got))

			// The order is the order of the new version's actions, which is a
			// slice, so it is deterministic and worth asserting on.
			if len(got) != len(tt.want) {
				t.Fatalf("warnings = %q, want %q", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("warning %d = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// A nil sink is the auto-apply-with-no-reporter case. It has to be inert rather
// than a panic in a background process nobody is watching.
func TestWarnShellInitChanges_NilSink(t *testing.T) {
	warnShellInitChanges("tool",
		[]CleanupAction{{Action: "delete_file", Path: "share/shell.d/tool@1.0.0.bash", ContentHash: "old"}},
		[]CleanupAction{{Action: "delete_file", Path: "share/shell.d/tool@2.0.0.bash", ContentHash: "new"}},
		nil)
}

// TestSnapshotCleanup covers what the snapshot has to survive: a tool that is
// not installed, a legacy state file with no ActiveVersion, and the normal case.
func TestSnapshotCleanup(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	if got := mgr.SnapshotCleanup("absent"); got.Tool != "" {
		t.Errorf("SnapshotCleanup on an uninstalled tool = %+v, want the zero snapshot", got)
	}

	twoVersionTool(t, mgr, cfg, "mytool", "1.0.0")

	snap := mgr.SnapshotCleanup("mytool")
	if snap.Tool != "mytool" || snap.Version != "1.0.0" {
		t.Fatalf("SnapshotCleanup() = %s@%s, want mytool@1.0.0", snap.Tool, snap.Version)
	}
	if len(snap.Actions) != 1 || snap.Actions[0].Path != "share/shell.d/mytool@1.0.0.bash" {
		t.Errorf("snapshot actions = %v, want the active version's fragment", snap.Actions)
	}

}

// activeVersionOf's fallback to the pre-multi-version field is defensive: a
// state file loaded from disk has already been through migrateToMultiVersion,
// which sets ActiveVersion. It is asserted directly rather than through a load
// for exactly that reason -- a test that went through the loader would be
// asserting the migration, not this.
func TestActiveVersionOf(t *testing.T) {
	tests := []struct {
		name string
		ts   ToolState
		want string
	}{
		{"active version wins", ToolState{ActiveVersion: "2.0.0", Version: "1.0.0"}, "2.0.0"},
		{"legacy field is the fallback", ToolState{Version: "1.0.0"}, "1.0.0"},
		{"neither is set", ToolState{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activeVersionOf(tt.ts); got != tt.want {
				t.Errorf("activeVersionOf(%+v) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}
}

// TestReconcileUpdate_DeletesOnlyWhatNothingRecords is the acceptance case from
// issue #2470, run through the real state machinery in both configurations.
//
// A tool drops a shell between versions. While the replaced version is still
// installed -- which it is immediately after an update, as the rollback target --
// its fragment is retained and stays on disk. Once that version is reaped, the
// same reconcile deletes it. Asserting only the first half would pass against a
// reconcile that never deletes anything at all.
func TestReconcileUpdate_DeletesOnlyWhatNothingRecords(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		t.Fatal(err)
	}

	// v1 integrates with bash and zsh; v2 drops zsh. The recorded hashes have to
	// be real digests of the files on disk, or the cache rebuild that follows
	// the cleanup drops the fragments as tampered-with.
	body := "export TOOL_HOME=1\n"
	writeFragment := func(path string) CleanupAction {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cfg.HomeDir, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return CleanupAction{Action: "delete_file", Path: path, ContentHash: hashOf(body)}
	}

	v1 := []CleanupAction{
		writeFragment("share/shell.d/mytool@1.0.0.bash"),
		writeFragment("share/shell.d/mytool@1.0.0.zsh"),
	}
	v2 := []CleanupAction{
		writeFragment("share/shell.d/mytool@2.0.0.bash"),
	}

	if err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{"1.0.0": {CleanupActions: v1}}
	}); err != nil {
		t.Fatal(err)
	}

	snap := mgr.SnapshotCleanup("mytool")

	// The update lands: v2 becomes active, v1 stays installed as the rollback
	// target.
	if err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.PreviousVersion = "1.0.0"
		ts.Versions["2.0.0"] = VersionState{CleanupActions: v2}
	}); err != nil {
		t.Fatal(err)
	}

	var warns []string
	mgr.ReconcileUpdate(snap, recordWarns(&warns))

	droppedShell := filepath.Join(cfg.HomeDir, "share", "shell.d", "mytool@1.0.0.zsh")
	if _, err := os.Stat(droppedShell); err != nil {
		t.Fatalf("the rollback target's zsh fragment was deleted while 1.0.0 is still installed: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("warnings = %q, want none: bash is the only shared shell and its target changed version, not content", warns)
	}

	// Reap 1.0.0 the way garbage collection eventually does, then reconcile the
	// same snapshot again. Nothing records the zsh fragment now.
	if err := mgr.state.UpdateTool("mytool", func(ts *ToolState) {
		delete(ts.Versions, "1.0.0")
		ts.PreviousVersion = ""
	}); err != nil {
		t.Fatal(err)
	}

	mgr.ReconcileUpdate(snap, nil)

	if _, err := os.Stat(droppedShell); !os.IsNotExist(err) {
		t.Errorf("the dropped shell's fragment survived once no installed version recorded it: %v", err)
	}
	// The shell the new version still integrates with keeps its file.
	if _, err := os.Stat(filepath.Join(cfg.HomeDir, "share", "shell.d", "mytool@2.0.0.bash")); err != nil {
		t.Errorf("the active version's own fragment was deleted: %v", err)
	}
}

// TestReconcileUpdate_Inert covers every shape that must do nothing. Each one
// reaching ExecuteStaleCleanup would compare a version against itself or against
// a version that is not there, and delete on the strength of it.
func TestReconcileUpdate_Inert(t *testing.T) {
	tests := []struct {
		name string
		snap func(mgr *Manager) CleanupSnapshot
	}{
		{
			name: "the zero snapshot",
			snap: func(*Manager) CleanupSnapshot { return CleanupSnapshot{} },
		},
		{
			name: "a snapshot with no recorded actions",
			snap: func(*Manager) CleanupSnapshot {
				return CleanupSnapshot{Tool: "mytool", Version: "1.0.0"}
			},
		},
		{
			name: "a tool that is no longer installed",
			snap: func(*Manager) CleanupSnapshot {
				return CleanupSnapshot{
					Tool:    "removed",
					Version: "1.0.0",
					Actions: []CleanupAction{{Action: "delete_file", Path: "share/shell.d/removed@1.0.0.bash"}},
				}
			},
		},
		{
			name: "the active version did not change",
			snap: func(mgr *Manager) CleanupSnapshot { return mgr.SnapshotCleanup("mytool") },
		},
		{
			// `tsuku update` on a tool already at latest installs over the same
			// version, and that install rewrites the version's cleanup record.
			// Reconciling then would compare a version against its own newer
			// record and delete the difference -- reinstall semantics, on a path
			// that did not ask for them. The version check is what stops it, so
			// this snapshot records a path nothing installed still claims.
			name: "the active version did not change but its record was rewritten",
			snap: func(mgr *Manager) CleanupSnapshot {
				snap := mgr.SnapshotCleanup("mytool")
				snap.Actions = append(append([]CleanupAction{}, snap.Actions...), CleanupAction{
					Action: "delete_file",
					Path:   "share/shell.d/" + orphanFragment,
				})
				return snap
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := testutil.NewTestConfig(t)
			defer cleanup()
			mgr := New(cfg)
			twoVersionTool(t, mgr, cfg, "mytool", "1.0.0")

			// A file no installed version records. Every guard under test sits
			// upstream of the retained-path check, so this is the one thing a
			// reconcile that should not have run is able to delete.
			shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
			if err := os.WriteFile(filepath.Join(shellDDir, orphanFragment), []byte("# orphan\n"), 0600); err != nil {
				t.Fatal(err)
			}

			before := shellDContents(t, shellDDir)

			var warns []string
			mgr.ReconcileUpdate(tt.snap(mgr), recordWarns(&warns))

			if len(warns) != 0 {
				t.Errorf("warnings = %q, want none", warns)
			}
			after := shellDContents(t, shellDDir)
			for _, name := range before {
				if !slices.Contains(after, name) {
					t.Errorf("%s was deleted by a reconcile that should have done nothing", name)
				}
			}
		})
	}
}

// orphanFragment names a shell.d file no version of any tool records. Nothing
// protects it, so it is what a reconcile that ran when it should not have would
// take with it.
const orphanFragment = "mytool@0.9.0.bash"

// shellDContents lists the files in a shell.d directory, sorted.
func shellDContents(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}
