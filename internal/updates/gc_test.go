package updates

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeVersionStore stands in for install.Manager: it answers what state records
// for each tool and captures the reap calls garbage collection makes.
type fakeVersionStore struct {
	versions map[string][]string
	listErr  error
	reapErr  error
	reaped   []string
}

func (f *fakeVersionStore) InstalledVersions(tool string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.versions[tool], nil
}

func (f *fakeVersionStore) ReapVersion(tool, version string) error {
	if f.reapErr != nil {
		return f.reapErr
	}
	f.reaped = append(f.reaped, tool+"@"+version)
	return nil
}

// storeFor builds a store that records exactly the versions given for one tool.
func storeFor(tool string, versions ...string) *fakeVersionStore {
	return &fakeVersionStore{versions: map[string][]string{tool: versions}}
}

func TestGarbageCollectVersions_RemovesOldVersions(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	mkdir(t, filepath.Join(dir, "node-20.0.0"))
	mkdir(t, filepath.Join(dir, "node-20.1.0"))

	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-10*24*time.Hour))

	store := storeFor("node", "18.0.0", "20.0.0", "20.1.0")
	if err := GarbageCollectVersions(store, dir, "node", "20.1.0", "20.0.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); !os.IsNotExist(err) {
		t.Error("expected node-18.0.0 to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-20.0.0")); err != nil {
		t.Error("expected node-20.0.0 to remain (rollback target)")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-20.1.0")); err != nil {
		t.Error("expected node-20.1.0 to remain (active)")
	}
}

func TestGarbageCollectVersions_ProtectsActive(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "rg-14.0.0"))
	setMtime(t, filepath.Join(dir, "rg-14.0.0"), time.Now().Add(-30*24*time.Hour))

	store := storeFor("rg", "14.0.0")
	if err := GarbageCollectVersions(store, dir, "rg", "14.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-14.0.0")); err != nil {
		t.Error("active version should never be removed")
	}
}

func TestGarbageCollectVersions_ProtectsPrevious(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "rg-13.0.0"))
	mkdir(t, filepath.Join(dir, "rg-14.0.0"))
	setMtime(t, filepath.Join(dir, "rg-13.0.0"), time.Now().Add(-30*24*time.Hour))

	store := storeFor("rg", "13.0.0", "14.0.0")
	if err := GarbageCollectVersions(store, dir, "rg", "14.0.0", "13.0.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-13.0.0")); err != nil {
		t.Error("previous version should never be removed")
	}
}

func TestGarbageCollectVersions_RetentionBoundary(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "jq-1.6"))
	setMtime(t, filepath.Join(dir, "jq-1.6"), time.Now().Add(-6*24*time.Hour))

	store := storeFor("jq", "1.6", "1.7")
	if err := GarbageCollectVersions(store, dir, "jq", "1.7", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "jq-1.6")); err != nil {
		t.Error("version within retention period should not be removed")
	}
}

func TestGarbageCollectVersions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := storeFor("node", "20.0.0")
	if err := GarbageCollectVersions(store, dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal("should not error on empty directory")
	}
}

func TestGarbageCollectVersions_IgnoresOtherTools(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "ripgrep-14.0.0"))
	setMtime(t, filepath.Join(dir, "ripgrep-14.0.0"), time.Now().Add(-30*24*time.Hour))

	store := &fakeVersionStore{versions: map[string][]string{
		"node":    {"20.0.0"},
		"ripgrep": {"14.0.0"},
	}}
	if err := GarbageCollectVersions(store, dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "ripgrep-14.0.0")); err != nil {
		t.Error("GC should not touch other tools' directories")
	}
}

// The registry ships 59 name pairs where one tool's name prefixes another's --
// git/git-lfs, docker/docker-compose, helm/helm-docs. Reclaiming the shorter
// name used to read "git-lfs-3.5.0" as version "lfs-3.5.0" of git and delete a
// working installation of a different tool.
func TestGarbageCollectVersions_LeavesToolWhoseNameSharesAPrefix(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "git-2.46.0"))
	mkdir(t, filepath.Join(dir, "git-2.47.0"))
	mkdir(t, filepath.Join(dir, "git-lfs-3.5.0"))
	// git-lfs was installed and then left alone for a month, so its directory
	// is well past retention. It is still a different tool.
	setMtime(t, filepath.Join(dir, "git-lfs-3.5.0"), time.Now().Add(-30*24*time.Hour))

	store := &fakeVersionStore{versions: map[string][]string{
		"git":     {"2.46.0", "2.47.0"},
		"git-lfs": {"3.5.0"},
	}}

	if err := GarbageCollectVersions(store, dir, "git", "2.47.0", "2.46.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "git-lfs-3.5.0")); err != nil {
		t.Error("reclaiming git deleted git-lfs-3.5.0, which belongs to another tool")
	}
	if len(store.reaped) != 0 {
		t.Errorf("reclaiming git reaped state that is not its own: %v", store.reaped)
	}
}

// The mirror image: the colliding tool reclaiming its own old version must
// still work, so the fix is not "never delete anything near a collision".
func TestGarbageCollectVersions_ReclaimsOwnVersionDespiteCollidingName(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "git-2.47.0"))
	mkdir(t, filepath.Join(dir, "git-lfs-3.4.0"))
	mkdir(t, filepath.Join(dir, "git-lfs-3.5.0"))
	setMtime(t, filepath.Join(dir, "git-lfs-3.4.0"), time.Now().Add(-30*24*time.Hour))

	store := &fakeVersionStore{versions: map[string][]string{
		"git":     {"2.47.0"},
		"git-lfs": {"3.4.0", "3.5.0"},
	}}

	if err := GarbageCollectVersions(store, dir, "git-lfs", "3.5.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "git-lfs-3.4.0")); !os.IsNotExist(err) {
		t.Error("git-lfs should still reclaim its own aged-out version")
	}
	if _, err := os.Stat(filepath.Join(dir, "git-2.47.0")); err != nil {
		t.Error("reclaiming git-lfs must not touch git")
	}
	if len(store.reaped) != 1 || store.reaped[0] != "git-lfs@3.4.0" {
		t.Errorf("expected git-lfs@3.4.0 to be reaped, got %v", store.reaped)
	}
}

// A directory nothing in state claims is left alone. Reclaiming it would mean
// deleting on the strength of a filesystem name, which is the mistake this
// function used to make.
func TestGarbageCollectVersions_KeepsDirectoryStateDoesNotClaim(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-17.0.0"))
	setMtime(t, filepath.Join(dir, "node-17.0.0"), time.Now().Add(-30*24*time.Hour))

	store := storeFor("node", "20.1.0")
	if err := GarbageCollectVersions(store, dir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "node-17.0.0")); err != nil {
		t.Error("a directory state has no record of should be left where it is")
	}
	if len(store.reaped) != 0 {
		t.Errorf("nothing should have been reaped, got %v", store.reaped)
	}
}

func TestGarbageCollectVersions_ReapsStateForRemovedVersions(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	mkdir(t, filepath.Join(dir, "node-20.1.0"))
	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-10*24*time.Hour))

	store := storeFor("node", "18.0.0", "20.1.0")
	if err := GarbageCollectVersions(store, dir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if len(store.reaped) != 1 || store.reaped[0] != "node@18.0.0" {
		t.Errorf("expected the GC'd version's state to be reaped, got %v", store.reaped)
	}
	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); !os.IsNotExist(err) {
		t.Error("expected node-18.0.0 to be removed")
	}
}

func TestGarbageCollectVersions_KeepsDirectoryWhenReapFails(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-10*24*time.Hour))

	store := storeFor("node", "18.0.0", "20.1.0")
	store.reapErr = errReapFailed
	if err := GarbageCollectVersions(store, dir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Leaving the directory in place keeps state and disk agreeing: a version
	// state.json still records must still exist.
	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); err != nil {
		t.Error("expected node-18.0.0 to survive a failed reap")
	}
}

// A version that could steer the path join out of the tools directory is
// dropped before anything is removed. state.json is written by tsuku, but it is
// an editable file on disk and the version now reaches os.RemoveAll from it.
func TestGarbageCollectVersions_RefusesVersionThatEscapesToolsDir(t *testing.T) {
	root := t.TempDir()
	toolsDir := filepath.Join(root, "tools")
	mkdir(t, toolsDir)
	victim := filepath.Join(root, "victim")
	mkdir(t, victim)
	setMtime(t, victim, time.Now().Add(-30*24*time.Hour))

	// Joined onto the tools directory, "node-1.0.0/../../victim" resolves to
	// the sibling directory outside it.
	store := storeFor("node", "1.0.0/../../victim", "20.1.0")
	if err := GarbageCollectVersions(store, toolsDir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(victim); err != nil {
		t.Error("a version containing a path separator must not reach os.RemoveAll")
	}
	if len(store.reaped) != 0 {
		t.Errorf("nothing should have been reaped, got %v", store.reaped)
	}
}

// A symlink where a version directory should be is not one, and the thing it
// points at has its own mtime and its own owner. Leave it alone.
func TestGarbageCollectVersions_DoesNotFollowASymlinkedVersionDir(t *testing.T) {
	root := t.TempDir()
	toolsDir := filepath.Join(root, "tools")
	mkdir(t, toolsDir)
	target := filepath.Join(root, "elsewhere")
	mkdir(t, target)
	setMtime(t, target, time.Now().Add(-30*24*time.Hour))

	link := filepath.Join(toolsDir, "node-18.0.0")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	store := storeFor("node", "18.0.0", "20.1.0")
	if err := GarbageCollectVersions(store, toolsDir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(link); err != nil {
		t.Error("a symlink is not a version directory and should not be reclaimed")
	}
	if len(store.reaped) != 0 {
		t.Errorf("nothing should have been reaped, got %v", store.reaped)
	}
}

func TestGarbageCollectVersions_RequiresAStore(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-30*24*time.Hour))

	// Without state there is no way to tell one tool's directory from
	// another's, so this must refuse rather than fall back to name matching.
	if err := GarbageCollectVersions(nil, dir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err == nil {
		t.Error("expected an error when no version store is supplied")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); err != nil {
		t.Error("nothing should be removed when there is no store to authorize it")
	}
}

func TestGarbageCollectVersions_ReturnsStateReadFailure(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-30*24*time.Hour))

	store := &fakeVersionStore{listErr: errStateUnreadable}
	if err := GarbageCollectVersions(store, dir, "node", "20.1.0", "", 7*24*time.Hour, time.Now()); err == nil {
		t.Error("expected the state read failure to surface")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); err != nil {
		t.Error("nothing should be removed when state cannot be read")
	}
}

var (
	errReapFailed      = errors.New("reap failed")
	errStateUnreadable = errors.New("state unreadable")
)

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func setMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
