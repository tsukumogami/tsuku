package install

import (
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

// newOpsManager constructs a Manager backed by an isolated $TSUKU_HOME
// for state_ops_test.go cases. No event bus is wired — these methods do
// not publish events.
func newOpsManager(t *testing.T) *Manager {
	t.Helper()
	cfg, cleanup := testutil.NewTestConfig(t)
	t.Cleanup(cleanup)
	return New(cfg)
}

// readToolState fetches a tool's state via the new GetToolState accessor
// and fails the test if the tool is missing.
func readToolState(t *testing.T, mgr *Manager, name string) *ToolState {
	t.Helper()
	ts, err := mgr.GetToolState(name)
	if err != nil {
		t.Fatalf("GetToolState(%q) error: %v", name, err)
	}
	if ts == nil {
		t.Fatalf("GetToolState(%q) returned nil; expected tool present", name)
	}
	return ts
}

func TestManager_MarkExplicit_FreshTool(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.MarkExplicit("gh", "parent-tool"); err != nil {
		t.Fatalf("MarkExplicit error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !ts.IsExplicit {
		t.Error("IsExplicit should be true after MarkExplicit")
	}
	if !reflect.DeepEqual(ts.RequiredBy, []string{"parent-tool"}) {
		t.Errorf("RequiredBy = %v, want [parent-tool]", ts.RequiredBy)
	}
}

func TestManager_MarkExplicit_EmptyParent_NoRequiredByMutation(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.MarkExplicit("gh", ""); err != nil {
		t.Fatalf("MarkExplicit error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !ts.IsExplicit {
		t.Error("IsExplicit should be true even with empty parent")
	}
	if len(ts.RequiredBy) != 0 {
		t.Errorf("RequiredBy = %v, want empty slice", ts.RequiredBy)
	}
}

func TestManager_MarkExplicit_DedupesParent(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.MarkExplicit("gh", "a"); err != nil {
		t.Fatalf("first MarkExplicit error: %v", err)
	}
	if err := mgr.MarkExplicit("gh", "a"); err != nil {
		t.Fatalf("second MarkExplicit error: %v", err)
	}
	if err := mgr.MarkExplicit("gh", "b"); err != nil {
		t.Fatalf("third MarkExplicit error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !reflect.DeepEqual(ts.RequiredBy, []string{"a", "b"}) {
		t.Errorf("RequiredBy = %v, want [a b]", ts.RequiredBy)
	}
}

func TestManager_RecordDependency_FreshTool(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.RecordDependency("gh", "git"); err != nil {
		t.Fatalf("RecordDependency error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !reflect.DeepEqual(ts.InstallDependencies, []string{"git"}) {
		t.Errorf("InstallDependencies = %v, want [git]", ts.InstallDependencies)
	}
}

func TestManager_RecordDependency_DedupesExisting(t *testing.T) {
	mgr := newOpsManager(t)

	for _, dep := range []string{"git", "git", "make"} {
		if err := mgr.RecordDependency("gh", dep); err != nil {
			t.Fatalf("RecordDependency(%q) error: %v", dep, err)
		}
	}

	ts := readToolState(t, mgr, "gh")
	if !reflect.DeepEqual(ts.InstallDependencies, []string{"git", "make"}) {
		t.Errorf("InstallDependencies = %v, want [git make]", ts.InstallDependencies)
	}
}

func TestManager_RecordDependency_EmptyDep_NoOp(t *testing.T) {
	mgr := newOpsManager(t)

	// Seed an existing tool so we can verify the state is untouched.
	if err := mgr.MarkExplicit("gh", ""); err != nil {
		t.Fatalf("seed MarkExplicit error: %v", err)
	}

	if err := mgr.RecordDependency("gh", ""); err != nil {
		t.Fatalf("RecordDependency error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if len(ts.InstallDependencies) != 0 {
		t.Errorf("InstallDependencies = %v, want empty slice (no-op)", ts.InstallDependencies)
	}
}

func TestManager_SetInstallDependencies_Overwrites(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.RecordDependency("gh", "old-dep"); err != nil {
		t.Fatalf("seed RecordDependency error: %v", err)
	}
	if err := mgr.SetInstallDependencies("gh", []string{"new-a", "new-b"}); err != nil {
		t.Fatalf("SetInstallDependencies error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !reflect.DeepEqual(ts.InstallDependencies, []string{"new-a", "new-b"}) {
		t.Errorf("InstallDependencies = %v, want [new-a new-b]", ts.InstallDependencies)
	}
}

func TestManager_SetRuntimeDependencies_Overwrites(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.SetRuntimeDependencies("gh", []string{"libssl", "libcurl"}); err != nil {
		t.Fatalf("first SetRuntimeDependencies error: %v", err)
	}
	if err := mgr.SetRuntimeDependencies("gh", []string{"libssl"}); err != nil {
		t.Fatalf("second SetRuntimeDependencies error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if !reflect.DeepEqual(ts.RuntimeDependencies, []string{"libssl"}) {
		t.Errorf("RuntimeDependencies = %v, want [libssl]", ts.RuntimeDependencies)
	}
}

func TestManager_RecordCleanup_StoresOnActiveVersion(t *testing.T) {
	mgr := newOpsManager(t)

	// Seed the tool with an ActiveVersion and matching Versions entry.
	if err := mgr.state.UpdateTool("gh", func(ts *ToolState) {
		ts.ActiveVersion = "2.50.0"
		ts.Versions = map[string]VersionState{
			"2.50.0": {Requested: "2.50.0"},
		}
	}); err != nil {
		t.Fatalf("seed UpdateTool error: %v", err)
	}

	actions := []CleanupAction{
		{Action: "delete_file", Path: "shell/completions/gh.bash"},
		{Action: "delete_dir", Path: "share/gh"},
	}
	if err := mgr.RecordCleanup("gh", actions); err != nil {
		t.Fatalf("RecordCleanup error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	vs, ok := ts.Versions["2.50.0"]
	if !ok {
		t.Fatalf("Versions[2.50.0] missing after RecordCleanup")
	}
	if !reflect.DeepEqual(vs.CleanupActions, actions) {
		t.Errorf("CleanupActions = %v, want %v", vs.CleanupActions, actions)
	}
}

func TestManager_RecordCleanup_EmptyActions_NoOp(t *testing.T) {
	mgr := newOpsManager(t)

	// Seed with an ActiveVersion so an accidental write would be detectable.
	if err := mgr.state.UpdateTool("gh", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {
				Requested:      "1.0.0",
				CleanupActions: []CleanupAction{{Action: "delete_file", Path: "preexisting"}},
			},
		}
	}); err != nil {
		t.Fatalf("seed UpdateTool error: %v", err)
	}

	if err := mgr.RecordCleanup("gh", nil); err != nil {
		t.Fatalf("RecordCleanup(nil) error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	vs := ts.Versions["1.0.0"]
	if len(vs.CleanupActions) != 1 || vs.CleanupActions[0].Path != "preexisting" {
		t.Errorf("CleanupActions should be unchanged on no-op; got %v", vs.CleanupActions)
	}
}

func TestManager_RecordCleanup_NoActiveVersion_NoOp(t *testing.T) {
	mgr := newOpsManager(t)

	// Tool exists in state but has no ActiveVersion (e.g., entry created
	// by MarkExplicit before Install ran).
	if err := mgr.MarkExplicit("gh", ""); err != nil {
		t.Fatalf("seed MarkExplicit error: %v", err)
	}

	actions := []CleanupAction{{Action: "delete_file", Path: "x"}}
	if err := mgr.RecordCleanup("gh", actions); err != nil {
		t.Fatalf("RecordCleanup error: %v", err)
	}

	ts := readToolState(t, mgr, "gh")
	if len(ts.Versions) > 0 {
		t.Errorf("Versions should remain empty when ActiveVersion is unset; got %v", ts.Versions)
	}
}

func TestManager_GetToolState_NotFound_NilNoError(t *testing.T) {
	mgr := newOpsManager(t)

	ts, err := mgr.GetToolState("never-installed")
	if err != nil {
		t.Fatalf("GetToolState error: %v", err)
	}
	if ts != nil {
		t.Errorf("GetToolState for missing tool should return nil; got %+v", ts)
	}
}

func TestManager_LoadState_RoundTrip(t *testing.T) {
	mgr := newOpsManager(t)

	if err := mgr.MarkExplicit("alpha", "root"); err != nil {
		t.Fatalf("MarkExplicit error: %v", err)
	}
	if err := mgr.RecordDependency("alpha", "beta"); err != nil {
		t.Fatalf("RecordDependency error: %v", err)
	}

	state, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}
	alpha, ok := state.Installed["alpha"]
	if !ok {
		t.Fatalf("LoadState missing alpha; got %v", state.Installed)
	}
	if !alpha.IsExplicit {
		t.Error("alpha.IsExplicit should be true after MarkExplicit")
	}
	if !reflect.DeepEqual(alpha.RequiredBy, []string{"root"}) {
		t.Errorf("RequiredBy = %v, want [root]", alpha.RequiredBy)
	}
	if !reflect.DeepEqual(alpha.InstallDependencies, []string{"beta"}) {
		t.Errorf("InstallDependencies = %v, want [beta]", alpha.InstallDependencies)
	}
}
