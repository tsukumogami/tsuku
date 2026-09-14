package install

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

// TestLibraryPathsAndShellFilterSplit pins the boundary between the two
// callers of the library scan.
//
// LibraryPaths answers "where do installed libraries live", and both the
// wrapper generator here and the dlopen verification helper in internal/verify
// need the same answer -- when the helper computed its own, it used the libs
// directory itself, where no .so file lives, and the system copy won
// (tsukumogami/tsuku#1090).
//
// The shell-safety filter is not part of that answer. It belongs to
// collectLibraryPaths, whose output is interpolated into a generated /bin/sh
// wrapper: a path carrying a quote or a `$` would break or subvert the script.
// The helper passes the same paths to execve as an environment value, where no
// shell sees them, so filtering there would silently drop a real library
// directory from a verification run and weaken the verdict.
//
// A directory named with a `$` is legal on every filesystem tsuku supports, so
// the two answers genuinely differ and this test fails if either half is moved
// to the other side.
func TestLibraryPathsAndShellFilterSplit(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	libsDir := cfg.LibsDir
	safe := filepath.Join(libsDir, "libyaml-0.2.5", "lib")
	unsafe := filepath.Join(libsDir, "weird$name-1.0", "lib")
	staging := filepath.Join(libsDir, ".libffi-3.4.4.staging", "lib")
	for _, dir := range []string{safe, unsafe, staging} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}

	// A file, and a directory with no lib/ subdirectory: neither is a library.
	if err := os.WriteFile(filepath.Join(libsDir, "stray.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libsDir, "half-installed"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("LibraryPaths keeps every real library, shell-safe or not", func(t *testing.T) {
		got := LibraryPaths(libsDir)
		want := []string{safe, unsafe}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("LibraryPaths() = %v, want %v.\n"+
				"It must return every installed library's lib/ directory: the "+
				"verification helper passes these to execve, where a dropped "+
				"directory means a library verified against the system copy.", got, want)
		}
	})

	t.Run("collectLibraryPaths drops what the wrapper script cannot carry", func(t *testing.T) {
		got := New(cfg).collectLibraryPaths(libraryTestCtx())
		want := []string{safe}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("collectLibraryPaths() = %v, want %v.\n"+
				"These paths are interpolated into a generated /bin/sh wrapper, "+
				"so one carrying a $ or a quote must not reach it.", got, want)
		}
	})
}
