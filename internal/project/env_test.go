package project

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeEnv is a DiscoveryEnv over the real filesystem with the pieces a test
// needs to control replaced.
//
// It exists because every layout that decides this package's rules involves a
// file the test's own user cannot create: one owned by somebody else, one owned
// by root, one under a directory only an administrator could have made. The
// fake supplies the metadata for those without the test needing privilege.
type fakeEnv struct {
	uid uint32

	// root bounds the ancestor walk. Without it a fixture under t.TempDir()
	// climbs into the real filesystem, where an injected owner owns nothing.
	root string

	// ownerOf and modeOf override the metadata for specific paths.
	ownerOf map[string]uint32
	modeOf  map[string]fs.FileMode

	// unreadable paths report a metadata error that is not "does not exist".
	unreadable map[string]bool

	// swapTo, when set for a path, is the file OpenNoFollow returns instead,
	// standing in for a swap between the metadata read and the open.
	swapTo map[string]string

	// contentReads counts reads of config bytes, so a test can assert none
	// happened before a decision.
	contentReads int

	// strict makes any access to a path outside root fail the test.
	strict bool
	t      *testing.T
}

func newFakeEnv(t *testing.T, root string) *fakeEnv {
	t.Helper()
	return &fakeEnv{
		uid:        uint32(os.Geteuid()),
		root:       filepath.Clean(root),
		ownerOf:    map[string]uint32{},
		modeOf:     map[string]fs.FileMode{},
		unreadable: map[string]bool{},
		swapTo:     map[string]string{},
		t:          t,
	}
}

func (f *fakeEnv) env() DiscoveryEnv {
	return DiscoveryEnv{
		UID:          f.uid,
		EvalSymlinks: filepath.EvalSymlinks,
		Lstat:        f.lstat,
		Readlink:     os.Readlink,
		Ancestors:    f.ancestors,
		OpenNoFollow: f.open,
	}
}

func (f *fakeEnv) lstat(path string) (FileMeta, error) {
	if f.unreadable[filepath.Clean(path)] {
		return FileMeta{}, errors.New("permission denied")
	}
	m, err := osLstat(path)
	if err != nil {
		return FileMeta{}, err
	}
	if uid, ok := f.ownerOf[filepath.Clean(path)]; ok {
		m.UID = uid
	}
	if mode, ok := f.modeOf[filepath.Clean(path)]; ok {
		// Keep the type bits the real filesystem reported; only the
		// permission bits are being presented differently.
		m.Mode = (m.Mode &^ fs.ModePerm) | (mode & fs.ModePerm)
		if mode&fs.ModeSticky != 0 {
			m.Mode |= fs.ModeSticky
		}
	}
	return m, nil
}

// ancestors stops at the fixture root rather than climbing to /.
func (f *fakeEnv) ancestors(dir string) []string {
	dir = filepath.Clean(dir)
	var out []string
	for {
		out = append(out, dir)
		if dir == f.root {
			return out
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
	}
}

// countingFile presents chosen metadata over a real handle and counts reads of
// the config's bytes, so a test can assert none happened before a decision.
type countingFile struct {
	File
	meta FileMeta
	env  *fakeEnv
}

func (c *countingFile) Meta() (FileMeta, error) { return c.meta, nil }

func (c *countingFile) Read(p []byte) (int, error) {
	n, err := c.File.Read(p)
	if n > 0 {
		c.env.contentReads++
	}
	return n, err
}

func (f *fakeEnv) open(path string) (File, error) {
	target := filepath.Clean(path)
	if f.strict && !strings.HasPrefix(target, f.root) {
		f.t.Fatalf("discovery reached %s, outside the fixture", path)
	}

	open := target
	if swap, ok := f.swapTo[target]; ok {
		// The metadata the decision was made on belongs to the original; the
		// bytes now come from a different object, which is what the device and
		// inode comparison has to catch. Report the swapped object's own
		// metadata, as a real fstat on it would.
		open = swap
	}

	handle, err := osOpenNoFollow(open)
	if err != nil {
		return nil, err
	}
	meta, err := handle.Meta()
	if err != nil {
		handle.Close()
		return nil, err
	}
	if mode, ok := f.modeOf[open]; ok {
		meta.Mode = (meta.Mode &^ fs.ModePerm) | (mode & fs.ModePerm)
	}
	if uid, ok := f.ownerOf[open]; ok {
		meta.UID = uid
	}
	return &countingFile{File: handle, meta: meta, env: f}, nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestAncestorsStopsAtTheFixtureRoot is the assertion the other tests rest on.
//
// A fake whose ancestor walk climbs past the fixture reaches directories the
// test does not own and cannot describe, and every layout built under it is
// then decided by the real filesystem instead of by the fixture.
func TestAncestorsStopsAtTheFixtureRoot(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	f := newFakeEnv(t, root)

	got := f.ancestors(deep)
	if got[len(got)-1] != filepath.Clean(root) {
		t.Fatalf("walk ended at %s, want the fixture root %s", got[len(got)-1], root)
	}
	for _, dir := range got {
		if !strings.HasPrefix(dir, filepath.Clean(root)) {
			t.Fatalf("walk reached %s, outside the fixture", dir)
		}
	}
}

// TestLoadRoutesEveryAccessThroughTheSeams asserts discovery reads no config
// bytes before it has decided to read the file, which is what keeps the
// shell-prompt path from parsing anything it has not accepted.
func TestLoadRoutesEveryAccessThroughTheSeams(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(work, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")

	f := newFakeEnv(t, root)
	f.strict = true
	t.Setenv("TSUKU_CEILING_PATHS", root)

	result, err := LoadProjectConfigIn(f.env(), work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil {
		t.Fatal("expected the config to be found")
	}
	if f.contentReads == 0 {
		t.Fatal("the config was returned without any read going through the seam")
	}
}

// TestLoadRefusesAFileSwappedBetweenTheCheckAndTheRead pins the property the
// open-then-check sequence exists for: the bytes parsed belong to the object
// the decision was made on.
func TestLoadRefusesAFileSwappedBetweenTheCheckAndTheRead(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	configPath := filepath.Join(work, ConfigFileName)
	writeFile(t, configPath, "[tools]\nnode = \"20.0.0\"\n")

	other := filepath.Join(root, "attacker.toml")
	writeFile(t, other, "[tools]\nevil = \"1.0.0\"\n")

	f := newFakeEnv(t, root)
	f.swapTo[configPath] = other
	t.Setenv("TSUKU_CEILING_PATHS", root)

	_, err := LoadProjectConfigIn(f.env(), work)
	if err == nil {
		t.Fatal("a file swapped between the check and the read must not be parsed")
	}
	if !strings.Contains(err.Error(), "changed between the check and the read") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoadDoesNotBlockOnANamedPipe covers the denial of service a config path
// that is a FIFO produces today: every shell prompt waits for a writer.
func TestLoadDoesNotBlockOnANamedPipe(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(work, ConfigFileName)
	if err := mkfifo(fifo); err != nil {
		t.Skipf("cannot create a FIFO here: %v", err)
	}

	f := newFakeEnv(t, root)
	t.Setenv("TSUKU_CEILING_PATHS", root)

	done := make(chan error, 1)
	go func() {
		_, err := LoadProjectConfigIn(f.env(), work)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a named pipe at the config path must not be parsed")
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-timeoutAfterSeconds(5):
		t.Fatal("discovery blocked on the named pipe")
	}

	if f.contentReads != 0 {
		t.Fatalf("read %d times from a file that is not a config", f.contentReads)
	}
}

// TestCeilingsAreComparedResolved covers the half of #2555 that made a
// symlinked $HOME or ceiling stop nothing: the walk is resolved and the
// ceilings were not, so the two could never match.
func TestCeilingsAreComparedResolved(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(realDir, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	// A config above the ceiling. If the ceiling does not match, the walk
	// reaches it.
	writeFile(t, filepath.Join(root, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")

	f := newFakeEnv(t, root)
	t.Setenv("TSUKU_CEILING_PATHS", link)

	result, err := LoadProjectConfigIn(f.env(), filepath.Join(link, "repo"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result != nil {
		t.Fatalf("the ceiling named through a symlink did not stop the walk; found %s", result.Path)
	}
}

// TestUnresolvableCeilingIsKeptAsWritten pins that a ceiling naming a directory
// that does not exist is ignored rather than producing an error.
func TestUnresolvableCeilingIsKeptAsWritten(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(work, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")

	f := newFakeEnv(t, root)
	t.Setenv("TSUKU_CEILING_PATHS", filepath.Join(root, "nope")+":"+root)

	result, err := LoadProjectConfigIn(f.env(), work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil {
		t.Fatal("expected the config to be found")
	}
}

// TestUnexaminableDirectoryEndsTheWalk covers R5's fail-closed direction.
//
// Today an unreadable directory is indistinguishable from one holding no config
// and the walk continues past it. Ending there is the change; it is silent
// because nothing was found, so there is no file to name.
func TestUnexaminableDirectoryEndsTheWalk(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "mid")
	work := filepath.Join(mid, "repo")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	// A config above the unexaminable directory. It must not be reached.
	writeFile(t, filepath.Join(root, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")

	f := newFakeEnv(t, root)
	f.unreadable[filepath.Join(mid, ConfigFileName)] = true

	result, err := LoadProjectConfigIn(f.env(), work)
	if err != nil {
		t.Fatalf("an unexaminable directory must end the walk quietly, got: %v", err)
	}
	if result != nil {
		t.Fatalf("the walk continued past an unexaminable directory and found %s", result.Path)
	}
}

// TestMissingEntryContinuesTheWalk is the other half of the case above: an
// entry that simply is not there is ordinary, not a reason to stop.
func TestMissingEntryContinuesTheWalk(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "mid")
	work := filepath.Join(mid, "repo")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")

	f := newFakeEnv(t, root)

	result, err := LoadProjectConfigIn(f.env(), work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil {
		t.Fatal("the walk stopped at a directory that simply had no config")
	}
	if result.Dir != filepath.Clean(root) {
		t.Fatalf("found %s, want the config at the fixture root", result.Dir)
	}
}

// TestSymlinkToARegularFileStillLoads keeps the existing behavior: a config
// that is a symlink is read, through the target.
func TestSymlinkToARegularFileStillLoads(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	target := filepath.Join(work, "shared.toml")
	writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
	if err := os.Symlink("shared.toml", filepath.Join(work, ConfigFileName)); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	f := newFakeEnv(t, root)
	t.Setenv("TSUKU_CEILING_PATHS", root)

	result, err := LoadProjectConfigIn(f.env(), work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil || result.Config.Tools["node"].Version != "20.0.0" {
		t.Fatalf("a symlinked config was not read through its target: %+v", result)
	}
}

// TestSymlinkChainIsRefused pins that a link to a link is not walked. Each
// additional hop is another object nothing checked.
func TestSymlinkChainIsRefused(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	target := filepath.Join(work, "shared.toml")
	writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
	if err := os.Symlink("shared.toml", filepath.Join(work, "hop.toml")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if err := os.Symlink("hop.toml", filepath.Join(work, ConfigFileName)); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	f := newFakeEnv(t, root)
	t.Setenv("TSUKU_CEILING_PATHS", root)

	_, err := LoadProjectConfigIn(f.env(), work)
	if err == nil {
		t.Fatal("a symlink chain longer than one link must not be walked")
	}
	if !strings.Contains(err.Error(), "symlink to another symlink") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoadProjectConfigDelegates pins that the one-argument form still works
// against the real filesystem, so production behavior is not an untested path.
func TestLoadProjectConfigDelegates(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(work, ConfigFileName), "[tools]\nnode = \"20.0.0\"\n")
	t.Setenv("TSUKU_CEILING_PATHS", root)

	result, err := LoadProjectConfig(work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil {
		t.Fatal("expected the config to be found")
	}
}

func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}

func mkfifo(path string) error {
	return syscall.Mkfifo(path, 0o644)
}

// TestSymlinkBranchIsTakenFromMetadataNotFromTheOpenError pins the property
// that keeps the symlink path portable.
//
// Opening a symlink without following it fails, and reading that failure as the
// signal to run the symlink sequence would work on Linux. It would also make
// the branch depend on which errno each platform picks for a refused
// O_NOFOLLOW open -- on the one path no macOS test exercises, since the Go
// tests run on Linux only. The walk decides from the mode bit instead.
//
// The fake's OpenNoFollow fails the test if it is reached for the link at all,
// which is what distinguishes "decided before opening" from "decided from the
// open's error".
func TestSymlinkBranchIsTakenFromMetadataNotFromTheOpenError(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "repo")
	target := filepath.Join(work, "shared.toml")
	writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
	link := filepath.Join(work, ConfigFileName)
	if err := os.Symlink("shared.toml", link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	f := newFakeEnv(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TSUKU_CEILING_PATHS", root)

	// The link is presented at 0755 rather than the 0777 Linux really reports.
	// Without this the guard below is unreachable: an implementation that
	// dropped the metadata branch would judge the link with the mode clause and
	// be refused on its 0777 before it ever reached the open, so the test would
	// fail for the wrong reason and the guard would stay unproven. At 0755 the
	// clauses pass and the only thing left to catch a dropped branch is the
	// guard.
	f.modeOf[filepath.Clean(link)] = 0o755
	f.ownerOf[filepath.Clean(link)] = f.uid

	env := f.env()
	inner := env.OpenNoFollow
	env.OpenNoFollow = func(path string) (File, error) {
		if filepath.Clean(path) == filepath.Clean(link) {
			t.Errorf("the walk opened the link itself; the symlink branch is "+
				"being entered from the open's error rather than from the "+
				"metadata, which makes it depend on a platform's errno (%s)", path)
		}
		return inner(path)
	}

	result, err := LoadProjectConfigIn(env, work)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if result == nil || result.Config.Tools["node"].Version != "20.0.0" {
		t.Fatalf("the symlinked config was not read through its target: %+v", result)
	}
}
