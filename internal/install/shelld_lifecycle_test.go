package install

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// twoVersionTool lays out a tool with two installed versions, each owning its
// own bash fragment under share/shell.d, and marks active as the active one.
// Every version's directory exists so Activate's existence check passes.
func twoVersionTool(t *testing.T, mgr *Manager, cfg *config.Config, name, active string) {
	t.Helper()

	shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		t.Fatal(err)
	}

	versions := map[string]VersionState{}
	for i, v := range []string{"1.0.0", "2.0.0"} {
		body := "export TOOL_HOME=" + v + "\n"
		fragment := name + "@" + v + ".bash"
		if err := os.WriteFile(filepath.Join(shellDDir, fragment), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(cfg.ToolDir(name, v), "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		versions[v] = VersionState{
			Binaries:    []string{filepath.Join("bin", name)},
			InstalledAt: time.Now().Add(time.Duration(i) * time.Hour),
			CleanupActions: []CleanupAction{{
				Action:      "delete_file",
				Path:        "share/shell.d/" + fragment,
				ContentHash: hashOf(body),
			}},
		}
	}

	if err := mgr.state.UpdateTool(name, func(ts *ToolState) {
		ts.ActiveVersion = active
		ts.Versions = versions
	}); err != nil {
		t.Fatal(err)
	}
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func readCache(t *testing.T, cfg *config.Config) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cfg.HomeDir, "share", "shell.d", ".init-cache.bash"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("reading cache: %v", err)
	}
	return string(data)
}

// Activate had no shell.d code at all. The hook lives on Manager rather than in
// cmd/tsuku so that `tsuku rollback` and auto-apply's failure rollback -- both
// of which call Activate directly -- are covered by the same line.
func TestActivate_RebuildsShellCacheForTheNewActiveVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	twoVersionTool(t, mgr, cfg, "mytool", "2.0.0")

	if err := mgr.Activate(removeTestCtx(), "mytool", "1.0.0"); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	cache := readCache(t, cfg)
	if !strings.Contains(cache, "export TOOL_HOME=1.0.0") {
		t.Errorf("cache should hold the newly activated version's exports, got %q", cache)
	}
	if strings.Contains(cache, "export TOOL_HOME=2.0.0") {
		t.Errorf("cache still holds the deactivated version's exports: %q", cache)
	}

	// Rolling forward again must swap it back.
	if err := mgr.Activate(removeTestCtx(), "mytool", "2.0.0"); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if cache := readCache(t, cfg); !strings.Contains(cache, "export TOOL_HOME=2.0.0") {
		t.Errorf("cache should hold 2.0.0's exports after switching back, got %q", cache)
	}
}

// RemoveVersion rebuilt caches before its promotion block, so even a correct
// affected-shell set rebuilt against the pre-promotion world.
func TestRemoveVersion_RebuildsAfterThePromotion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	twoVersionTool(t, mgr, cfg, "mytool", "2.0.0")

	// Removing the active version promotes 1.0.0.
	if err := mgr.RemoveVersion(removeTestCtx(), "mytool", "2.0.0"); err != nil {
		t.Fatalf("RemoveVersion() error = %v", err)
	}

	ts, err := mgr.state.GetToolState("mytool")
	if err != nil || ts == nil {
		t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	if ts.ActiveVersion != "1.0.0" {
		t.Fatalf("active version = %q, want 1.0.0", ts.ActiveVersion)
	}

	cache := readCache(t, cfg)
	if !strings.Contains(cache, "export TOOL_HOME=1.0.0") {
		t.Errorf("cache should hold the promoted version's exports, got %q", cache)
	}
	if strings.Contains(cache, "export TOOL_HOME=2.0.0") {
		t.Errorf("cache still holds the removed version's exports: %q", cache)
	}
}

// The affectedShells assignment sat after the "still referenced by another
// version" continue, so a retained path never triggered a rebuild. Version
// keying makes retention rare but not impossible: a legacy record from before
// the rename is shared by two versions.
func TestExecuteCleanupActions_MarksSkippedShellsAffected(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	shared := "share/shell.d/shared.bash"
	toolState := &ToolState{
		ActiveVersion: "2.0.0",
		Versions: map[string]VersionState{
			"1.0.0": {CleanupActions: []CleanupAction{{Action: "delete_file", Path: shared}}},
			"2.0.0": {CleanupActions: []CleanupAction{{Action: "delete_file", Path: shared}}},
		},
	}

	shellDDir := filepath.Join(cfg.HomeDir, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shellDDir, "shared.bash"), []byte("# shared\n"), 0600); err != nil {
		t.Fatal(err)
	}

	affected := mgr.executeCleanupActions(removeTestCtx(), "mytool", "2.0.0",
		toolState.Versions["2.0.0"].CleanupActions, toolState)

	if !affected["bash"] {
		t.Error("a shell whose path was retained for another version must still be rebuilt")
	}
	if _, err := os.Stat(filepath.Join(shellDDir, "shared.bash")); err != nil {
		t.Error("a path another version still references must not be deleted")
	}
}

// ExecuteStaleCleanup had no cross-version guard at all. Version-keyed
// filenames mean the old and new versions share no shell.d path, so without the
// guard every one of the previous version's fragments reads as stale -- and
// that version is still installed and is the rollback target.
func TestExecuteStaleCleanup_KeepsPathsAnInstalledVersionRecords(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	twoVersionTool(t, mgr, cfg, "mytool", "2.0.0")

	ts, err := mgr.state.GetToolState("mytool")
	if err != nil {
		t.Fatal(err)
	}
	stale := StaleCleanupActions(ts.Versions["1.0.0"].CleanupActions, ts.Versions["2.0.0"].CleanupActions)
	if len(stale) != 1 {
		t.Fatalf("expected the old version's fragment to read as stale, got %v", stale)
	}

	mgr.ExecuteStaleCleanup(stale)

	// The rollback target's fragment must survive.
	rollbackFragment := filepath.Join(cfg.HomeDir, "share", "shell.d", "mytool@1.0.0.bash")
	if _, err := os.Stat(rollbackFragment); err != nil {
		t.Fatalf("the rollback target's fragment was deleted: %v", err)
	}

	// And rolling back must put its content in the shell the user sources.
	if err := mgr.Activate(removeTestCtx(), "mytool", "1.0.0"); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if cache := readCache(t, cfg); !strings.Contains(cache, "export TOOL_HOME=1.0.0") {
		t.Errorf("after rollback the cache should hold 1.0.0's exports, got %q", cache)
	}
}

func TestReapVersion_RunsCleanupAndDropsVersionState(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	twoVersionTool(t, mgr, cfg, "mytool", "2.0.0")

	if err := mgr.ReapVersion("mytool", "1.0.0"); err != nil {
		t.Fatalf("ReapVersion() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.HomeDir, "share", "shell.d", "mytool@1.0.0.bash")); !os.IsNotExist(err) {
		t.Error("the reaped version's fragment should be deleted, not leaked")
	}

	ts, err := mgr.state.GetToolState("mytool")
	if err != nil || ts == nil {
		t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	if _, still := ts.Versions["1.0.0"]; still {
		t.Error("the reaped version should be dropped from state")
	}
	if _, ok := ts.Versions["2.0.0"]; !ok {
		t.Error("the active version must survive a reap of another version")
	}

}

// The refusals are the safety property garbage collection leans on, so they get
// their own test rather than living at the bottom of the happy path. The caller
// is about to delete the version's directory; a version that state cannot vouch
// for is one this tool has no business claiming, and saying so is what stops a
// directory belonging to something else from being removed.
func TestReapVersion_RefusesWhatStateCannotVouchFor(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := New(cfg)

	twoVersionTool(t, mgr, cfg, "mytool", "2.0.0")

	if err := mgr.ReapVersion("mytool", "2.0.0"); err == nil {
		t.Error("ReapVersion must refuse the active version")
	}
	if err := mgr.ReapVersion("mytool", "9.9.9"); err == nil {
		t.Error("ReapVersion must refuse a version that state has no record of")
	}
	if err := mgr.ReapVersion("nosuchtool", "1.0.0"); err == nil {
		t.Error("ReapVersion must refuse a tool that state has no record of")
	}

	// Refusing must not be a side-effect-free lie: the versions it declined to
	// reap are still recorded.
	ts, err := mgr.state.GetToolState("mytool")
	if err != nil || ts == nil {
		t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	if _, ok := ts.Versions["2.0.0"]; !ok {
		t.Error("a refused reap must leave the version recorded")
	}
}
