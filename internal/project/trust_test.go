package project

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two uids the layout table needs and a test cannot create. They are
// injected through the metadata seam, which is the whole reason the seam
// exists: every "owned by another user" and "owned by root" row is otherwise
// unreachable from a test running as an unprivileged user.
const (
	otherUID uint32 = 9999
	thirdUID uint32 = 4242
)

// layoutFixture builds one row of the R2 table under t.TempDir().
type layoutFixture struct {
	t    *testing.T
	root string
	env  *fakeEnv
}

func newLayout(t *testing.T) *layoutFixture {
	t.Helper()
	root := t.TempDir()
	// Keep the fixture out from under any resolved home, so the rule applies.
	// A separate temporary directory is the home for the duration.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TSUKU_CEILING_PATHS", root)

	f := newFakeEnv(t, root)
	// The fixture root itself is the invoking user's and not writable by
	// others, so it never decides a row on its own.
	f.ownerOf[filepath.Clean(root)] = f.uid
	f.modeOf[filepath.Clean(root)] = 0o755
	return &layoutFixture{t: t, root: root, env: f}
}

// dir creates a directory and presents it with the given owner and mode.
func (l *layoutFixture) dir(rel string, owner uint32, mode fs.FileMode) string {
	l.t.Helper()
	p := filepath.Join(l.root, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		l.t.Fatalf("mkdir %s: %v", p, err)
	}
	l.env.ownerOf[filepath.Clean(p)] = owner
	l.env.modeOf[filepath.Clean(p)] = mode
	return p
}

// config writes a .tsuku.toml in dir and presents it with the given owner and
// mode.
func (l *layoutFixture) config(dir string, owner uint32, mode fs.FileMode) string {
	l.t.Helper()
	p := filepath.Join(dir, ConfigFileName)
	writeFile(l.t, p, "[tools]\nnode = \"20.0.0\"\n")
	l.env.ownerOf[filepath.Clean(p)] = owner
	l.env.modeOf[filepath.Clean(p)] = mode
	return p
}

func (l *layoutFixture) load(startDir string) (*ConfigResult, error) {
	return LoadProjectConfigIn(l.env.env(), startDir)
}

func mustBeFound(t *testing.T, l *layoutFixture, startDir string) {
	t.Helper()
	result, err := l.load(startDir)
	if err != nil {
		t.Fatalf("layout must be found, got error: %v", err)
	}
	if result == nil {
		t.Fatal("layout must be found, got no config")
	}
}

func mustBeRefused(t *testing.T, l *layoutFixture, startDir string) *RefusedError {
	t.Helper()
	result, err := l.load(startDir)
	if result != nil {
		t.Fatalf("layout must be refused, but a config at %s was applied", result.Path)
	}
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("layout must be refused with a *RefusedError, got: %v", err)
	}
	if refused.Reason == "" || refused.Remedy == "" {
		t.Fatalf("a refusal must carry both a reason and a remedy: %+v", refused)
	}
	return refused
}

// TestLayoutTable walks every row of the PRD's R2 table.
//
// The two rows that decide the rule are L3 and L8. They are ownership-identical
// -- in both the config is owned by somebody who is neither the invoking user
// nor root -- and only the writability of the namespace above them separates
// them. A rule that gets either one wrong gets the other wrong too.
func TestLayoutTable(t *testing.T) {
	t.Run("L1 user-owned checkout under an ordinary parent is found", func(t *testing.T) {
		l := newLayout(t)
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", l.env.uid, 0o755)
		l.config(app, l.env.uid, 0o644)
		mustBeFound(t, l, app)
	})

	t.Run("L2 root-owned checkout under an ordinary parent is found", func(t *testing.T) {
		l := newLayout(t)
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", rootUID, 0o755)
		l.config(app, rootUID, 0o644)
		mustBeFound(t, l, app)
	})

	t.Run("L3 foreign-owned checkout under an ordinary parent is found", func(t *testing.T) {
		l := newLayout(t)
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", otherUID, 0o755)
		l.config(app, otherUID, 0o644)
		mustBeFound(t, l, app)
	})

	t.Run("L4 foreign-owned checkout with root invoking is found", func(t *testing.T) {
		l := newLayout(t)
		l.env.uid = rootUID
		l.env.ownerOf[filepath.Clean(l.root)] = rootUID
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", otherUID, 0o755)
		l.config(app, otherUID, 0o644)
		mustBeFound(t, l, app)
	})

	t.Run("L5 config at the checkout root with the work several levels below", func(t *testing.T) {
		l := newLayout(t)
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", otherUID, 0o755)
		l.config(app, otherUID, 0o644)
		deep := l.dir("srv/app/a/b/c", otherUID, 0o755)
		mustBeFound(t, l, deep)
	})

	t.Run("L6 planted config in a shared directory is refused", func(t *testing.T) {
		l := newLayout(t)
		shared := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		l.config(shared, otherUID, 0o644)
		work := l.dir("tmp/shared-work", l.env.uid, 0o755)
		mustBeRefused(t, l, work)
	})

	t.Run("L7 the same with root invoking is refused", func(t *testing.T) {
		l := newLayout(t)
		l.env.uid = rootUID
		l.env.ownerOf[filepath.Clean(l.root)] = rootUID
		shared := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		l.config(shared, otherUID, 0o644)
		work := l.dir("tmp/shared-work", rootUID, 0o755)
		mustBeRefused(t, l, work)
	})

	t.Run("L8 squatted directory in a shared space is refused", func(t *testing.T) {
		l := newLayout(t)
		l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		build := l.dir("tmp/build", otherUID, 0o755)
		l.config(build, otherUID, 0o644)
		mustBeRefused(t, l, build)
	})

	t.Run("L11 umask 002 permissions are ordinary", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("work", l.env.uid, 0o775)
		l.config(app, l.env.uid, 0o664)
		mustBeFound(t, l, app)
	})

	t.Run("L12 foreign-owned file in a root-owned directory is found", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("image/app", rootUID, 0o755)
		l.config(app, thirdUID, 0o644)
		mustBeFound(t, l, app)
	})

	t.Run("L13 a world-writable directory without the sticky bit is refused", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("open", l.env.uid, 0o777)
		l.config(app, l.env.uid, 0o644)
		mustBeRefused(t, l, app)
	})
}

// TestRootOwnedWritableAncestorDoesNotSatisfyTheChain is the assertion the L8
// row alone does not make.
//
// L8's parent is both sticky and world-writable, so an implementation that
// wrongly exempted sticky ancestors would fail it. This one fails a different
// wrong implementation: one that compares each writable ancestor against the
// whole trusted set rather than against the config's own owner. Root is trusted
// as an owner, /tmp is root-owned, and admitting it re-admits every config
// planted there.
func TestRootOwnedWritableAncestorDoesNotSatisfyTheChain(t *testing.T) {
	l := newLayout(t)
	// World-writable and root-owned, without the sticky bit on the ancestor so
	// no sticky exemption could be what refuses it.
	l.dir("shared", rootUID, 0o777)
	app := l.dir("shared/app", otherUID, 0o755)
	l.config(app, otherUID, 0o644)

	refused := mustBeRefused(t, l, app)
	if !strings.Contains(refused.Reason, "anyone can write") {
		t.Fatalf("refused for the wrong reason: %q", refused.Reason)
	}
}

// TestClausesDoNotShortCircuitEachOther pins that all three must pass.
//
// Read as "accept when the owner is the invoking user", the ownership clause
// looks like it settles the question on its own. It does not: it settles the
// ownership question, and the directory and mode clauses still run. A config
// the user owns in a directory anyone can write to and delete from is refused,
// because the hard link an attacker puts at that path is owned by the user too.
func TestClausesDoNotShortCircuitEachOther(t *testing.T) {
	t.Run("user-owned config in a world-writable non-sticky directory is refused", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("open", l.env.uid, 0o777)
		l.config(app, l.env.uid, 0o644)
		mustBeRefused(t, l, app)
	})

	t.Run("user-owned world-writable config is refused", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("work", l.env.uid, 0o755)
		l.config(app, l.env.uid, 0o666)
		refused := mustBeRefused(t, l, app)
		if !strings.Contains(refused.Remedy, "metadata") {
			t.Fatalf("the mode refusal must name the mount option as well as chmod: %q", refused.Remedy)
		}
	})

	t.Run("user-owned config in a sticky world-writable directory is found", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		l.config(app, l.env.uid, 0o644)
		mustBeFound(t, l, app)
	})
}

// TestHomeEarlyReturnPreconditions covers the rule's off switch.
//
// HOME is an environment variable, so anyone who can write a shell profile or a
// directory-local environment file can set it. If HOME=/ switched the rule off
// for the whole filesystem, the rule would have a one-line bypass.
func TestHomeEarlyReturnPreconditions(t *testing.T) {
	refusedUnder := func(t *testing.T, setHome func(t *testing.T, root string)) {
		t.Helper()
		l := newLayout(t)
		setHome(t, l.root)
		shared := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		l.config(shared, otherUID, 0o644)
		work := l.dir("tmp/shared-work", l.env.uid, 0o755)
		mustBeRefused(t, l, work)
	}

	t.Run("HOME unset", func(t *testing.T) {
		refusedUnder(t, func(t *testing.T, _ string) { t.Setenv("HOME", "") })
	})

	t.Run("HOME is the filesystem root", func(t *testing.T) {
		refusedUnder(t, func(t *testing.T, _ string) { t.Setenv("HOME", "/") })
	})

	t.Run("HOME owned by another user", func(t *testing.T) {
		l := newLayout(t)
		home := l.dir("home", otherUID, 0o755)
		t.Setenv("HOME", home)
		shared := l.dir("home/tmp", rootUID, 0o777|fs.ModeSticky)
		l.config(shared, otherUID, 0o644)
		work := l.dir("home/tmp/shared-work", l.env.uid, 0o755)
		mustBeRefused(t, l, work)
	})

	t.Run("a qualifying home switches the rule off", func(t *testing.T) {
		l := newLayout(t)
		home := l.dir("home", l.env.uid, 0o755)
		t.Setenv("HOME", home)
		// A layout the rule would refuse anywhere else.
		open := l.dir("home/work", l.env.uid, 0o777)
		l.config(open, otherUID, 0o666)
		mustBeFound(t, l, open)
	})
}

// TestSymlinkLayouts covers L9 and L10 and the bypass the requirement's row
// leaves open.
func TestSymlinkLayouts(t *testing.T) {
	link := func(t *testing.T, target, name string) {
		t.Helper()
		if err := os.Symlink(target, name); err != nil {
			t.Skipf("cannot create a symlink here: %v", err)
		}
	}

	t.Run("L9 foreign-owned link is refused", func(t *testing.T) {
		l := newLayout(t)
		shared := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		target := filepath.Join(shared, "shared.toml")
		writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
		l.env.ownerOf[target] = l.env.uid
		linkPath := filepath.Join(shared, ConfigFileName)
		link(t, "shared.toml", linkPath)
		l.env.ownerOf[linkPath] = otherUID
		mustBeRefused(t, l, shared)
	})

	t.Run("L9 foreign-owned target is refused", func(t *testing.T) {
		l := newLayout(t)
		app := l.dir("work", l.env.uid, 0o755)
		store := l.dir("tmp", rootUID, 0o777|fs.ModeSticky)
		target := filepath.Join(store, "shared.toml")
		writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
		l.env.ownerOf[target] = otherUID
		linkPath := filepath.Join(app, ConfigFileName)
		link(t, target, linkPath)
		l.env.ownerOf[linkPath] = l.env.uid
		mustBeRefused(t, l, app)
	})

	t.Run("L9 a link from an acceptable location into a plantable one is refused", func(t *testing.T) {
		l := newLayout(t)
		// The checkout itself passes at the link's own location.
		app := l.dir("srv/app", l.env.uid, 0o755)
		l.dir("srv", rootUID, 0o755)
		// The target sits in a directory anyone can write to and delete from.
		open := l.dir("open", l.env.uid, 0o777)
		target := filepath.Join(open, "planted.toml")
		writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
		l.env.ownerOf[target] = l.env.uid
		linkPath := filepath.Join(app, ConfigFileName)
		link(t, target, linkPath)
		l.env.ownerOf[linkPath] = l.env.uid

		refused := mustBeRefused(t, l, app)
		if !strings.Contains(refused.Reason, "anyone can write") {
			t.Fatalf("the target's own directory was not judged: %q", refused.Reason)
		}
	})

	t.Run("L10 an ordinary link inside an accepted checkout is found", func(t *testing.T) {
		l := newLayout(t)
		l.dir("srv", rootUID, 0o755)
		app := l.dir("srv/app", otherUID, 0o755)
		target := filepath.Join(app, "shared.toml")
		writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
		l.env.ownerOf[target] = otherUID
		linkPath := filepath.Join(app, ConfigFileName)
		link(t, "shared.toml", linkPath)
		l.env.ownerOf[linkPath] = otherUID
		mustBeFound(t, l, app)
	})
}

// TestLinkModeIsNotJudged is the platform assertion.
//
// Linux reports mode 0777 for every symlink and macOS derives it from the
// umask. A mode clause applied at the link would therefore refuse every
// symlinked config on Linux and accept the same repository on macOS, which is
// the divergence the supported-platforms driver exists to prevent. This test
// presents both values through the seam and requires the same outcome.
func TestLinkModeIsNotJudged(t *testing.T) {
	for name, linkMode := range map[string]fs.FileMode{
		"linux-style 0777": 0o777,
		"macos-style 0755": 0o755,
	} {
		t.Run(name, func(t *testing.T) {
			l := newLayout(t)
			l.dir("srv", rootUID, 0o755)
			app := l.dir("srv/app", l.env.uid, 0o755)
			target := filepath.Join(app, "shared.toml")
			writeFile(t, target, "[tools]\nnode = \"20.0.0\"\n")
			l.env.ownerOf[target] = l.env.uid
			l.env.modeOf[target] = 0o644
			linkPath := filepath.Join(app, ConfigFileName)
			if err := os.Symlink("shared.toml", linkPath); err != nil {
				t.Skipf("cannot create a symlink here: %v", err)
			}
			l.env.ownerOf[linkPath] = l.env.uid
			l.env.modeOf[linkPath] = linkMode

			mustBeFound(t, l, app)
		})
	}
}

// TestRefusalStopsTheWalk pins R3: a refused config is an answer, not a reason
// to keep looking. Walking past it would apply a config further from the
// working directory than the nearest one, without saying so.
func TestRefusalStopsTheWalk(t *testing.T) {
	l := newLayout(t)
	// An acceptable config above.
	above := l.dir("above", l.env.uid, 0o755)
	l.config(above, l.env.uid, 0o644)
	// A refused one below it.
	open := l.dir("above/open", l.env.uid, 0o777)
	l.config(open, l.env.uid, 0o644)

	refused := mustBeRefused(t, l, open)
	if refused.Dir != filepath.Clean(open) {
		t.Fatalf("the refusal names %s, want the nearer config's directory %s", refused.Dir, open)
	}
}

// TestRefusedFileIsNeverParsed pins that a refusal reports the refusal and
// nothing about the contents, because the contents were never read.
func TestRefusedFileIsNeverParsed(t *testing.T) {
	l := newLayout(t)
	open := l.dir("open", l.env.uid, 0o777)
	p := filepath.Join(open, ConfigFileName)
	writeFile(t, p, "this is not = = toml\n")
	l.env.ownerOf[p] = l.env.uid
	l.env.modeOf[p] = 0o644

	mustBeRefused(t, l, open)

	_, err := l.load(open)
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		t.Fatal("a refused file must not also be reported as unparseable")
	}
	if l.env.contentReads != 0 {
		t.Fatalf("a refused file was read %d times", l.env.contentReads)
	}
}
