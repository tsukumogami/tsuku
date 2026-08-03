package updates

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// installFixture writes a tool's version directories and the state entry that
// claims them, the way an install would leave things.
func installFixture(t *testing.T, mgr *install.Manager, cfg *config.Config, name, active string, versions ...string) {
	t.Helper()

	recorded := map[string]install.VersionState{}
	for _, v := range versions {
		if err := os.MkdirAll(filepath.Join(cfg.ToolDir(name, v), "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		recorded[v] = install.VersionState{
			Binaries:    []string{filepath.Join("bin", name)},
			InstalledAt: time.Now(),
		}
	}

	if err := mgr.GetState().UpdateTool(name, func(ts *install.ToolState) {
		ts.ActiveVersion = active
		ts.Versions = recorded
	}); err != nil {
		t.Fatal(err)
	}
}

// The end-to-end shape of the collision, driven through the real
// install.Manager rather than a stand-in: git updates, its reclamation runs
// unattended, and git-lfs must still be installed afterwards -- on disk and as
// something Manager.List reports. Both halves matter. List skips a version
// whose directory is missing, so a deleted directory takes the tool out of
// tsuku list entirely while its bin/ symlinks stay behind pointing at nothing.
func TestGarbageCollectVersions_RealManagerSparesPrefixSharingTool(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := install.New(cfg)

	installFixture(t, mgr, cfg, "git", "2.47.0", "2.46.0", "2.47.0")
	installFixture(t, mgr, cfg, "git-lfs", "3.5.0", "3.5.0")

	lfsDir := cfg.ToolDir("git-lfs", "3.5.0")
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(lfsDir, old, old); err != nil {
		t.Fatal(err)
	}

	if err := GarbageCollectVersions(mgr, cfg.ToolsDir, "git", "2.47.0", "2.46.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(lfsDir); err != nil {
		t.Errorf("reclaiming git deleted %s, an installation of a different tool", lfsDir)
	}

	installed, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tool := range installed {
		if tool.Name == "git-lfs" && tool.Version == "3.5.0" {
			found = true
		}
	}
	if !found {
		t.Error("git-lfs 3.5.0 should still be installed after git's reclamation")
	}
}

// The same fixture with the roles reversed: git-lfs still reclaims its own
// aged-out version, and git is untouched.
func TestGarbageCollectVersions_RealManagerReclaimsOwnOldVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()
	mgr := install.New(cfg)

	installFixture(t, mgr, cfg, "git", "2.47.0", "2.47.0")
	installFixture(t, mgr, cfg, "git-lfs", "3.5.0", "3.4.0", "3.5.0")

	oldLFS := cfg.ToolDir("git-lfs", "3.4.0")
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldLFS, old, old); err != nil {
		t.Fatal(err)
	}

	if err := GarbageCollectVersions(mgr, cfg.ToolsDir, "git-lfs", "3.5.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(oldLFS); !os.IsNotExist(err) {
		t.Error("git-lfs 3.4.0 is past retention and should have been reclaimed")
	}
	if _, err := os.Stat(cfg.ToolDir("git", "2.47.0")); err != nil {
		t.Error("reclaiming git-lfs must not touch git")
	}

	ts, err := mgr.GetState().GetToolState("git-lfs")
	if err != nil || ts == nil {
		t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	if _, still := ts.Versions["3.4.0"]; still {
		t.Error("the reclaimed version should be gone from state, not left dangling")
	}
}
