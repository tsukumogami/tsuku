package realpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestResolve_ExistingPathUnderLink(t *testing.T) {
	link, resolved := testutil.SymlinkedTempDir(t)
	if err := os.MkdirAll(filepath.Join(resolved, "libs", "foo-1.0"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(filepath.Join(link, "libs", "foo-1.0"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(resolved, "libs", "foo-1.0"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

// The case filepath.EvalSymlinks can't answer: a directory an install hasn't
// created yet, under a home reached through a link.
func TestResolve_MissingTailUnderLink(t *testing.T) {
	link, resolved := testutil.SymlinkedTempDir(t)

	got, err := Resolve(filepath.Join(link, "tools", "tmux-3.5", "bin"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(resolved, "tools", "tmux-3.5", "bin"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestResolve_NoLinkIsUnchanged(t *testing.T) {
	root := testutil.ResolvedTempDir(t)
	if err := os.MkdirAll(filepath.Join(root, "libs"), 0755); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		root,
		filepath.Join(root, "libs"),
		filepath.Join(root, "tools", "missing", "bin"),
	} {
		got, err := Resolve(p)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", p, err)
		}
		if got != p {
			t.Errorf("Resolve(%q) = %q, want it unchanged", p, got)
		}
	}
}

// A link that leads outside the tree resolves to where it leads, which is what
// lets a containment check refuse it.
func TestResolve_FollowsLinkOutOfTree(t *testing.T) {
	root := testutil.ResolvedTempDir(t)
	outside := filepath.Join(root, "outside")
	libs := filepath.Join(root, "libs")
	for _, d := range []string{filepath.Join(outside, "lib"), libs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(libs, "evil-1.0")); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(filepath.Join(libs, "evil-1.0", "lib"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := filepath.Join(outside, "lib"); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
	if Within(got, libs) {
		t.Errorf("Within(%q, %q) = true, want false", got, libs)
	}
}

// A dangling link is refused rather than appended as written: whatever it is
// pointed at later is where the path leads.
func TestResolve_DanglingLinkIsAnError(t *testing.T) {
	root := testutil.ResolvedTempDir(t)
	libs := filepath.Join(root, "libs")
	if err := os.MkdirAll(libs, 0755); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(libs, "gone-1.0")
	if err := os.Symlink(filepath.Join(root, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{dangling, filepath.Join(dangling, "lib")} {
		got, err := Resolve(p)
		if err == nil {
			t.Errorf("Resolve(%q) = %q, want an error", p, got)
			continue
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Resolve(%q) error = %q, want it to name the dangling link", p, err)
		}
	}
}

func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	dir := sep + filepath.Join("home", "u", ".tsuku", "libs")
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"the dir itself", dir, true},
		{"the dir with a trailing separator", dir + sep, true},
		{"a child", filepath.Join(dir, "foo-1.0", "lib"), true},
		{"the parent", filepath.Dir(dir), false},
		{"a sibling sharing the prefix", dir + "-backup", false},
		{"a sibling with the prefix as a component start", dir + "x" + sep + "lib", false},
		{"unrelated", sep + filepath.Join("usr", "lib"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Within(tc.path, dir); got != tc.want {
				t.Errorf("Within(%q, %q) = %v, want %v", tc.path, dir, got, tc.want)
			}
		})
	}

	if !Within(sep+"anything", sep) {
		t.Errorf("Within(%q, %q) = false, want true", sep+"anything", sep)
	}
}
