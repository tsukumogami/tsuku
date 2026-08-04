package install

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// newLibraryWorkDir builds the work directory shape InstallLibrary copies from:
// a temp directory holding an .install subtree with the given relative paths
// and contents. An empty map still produces an empty .install directory.
func newLibraryWorkDir(t *testing.T, files map[string]string) string {
	t.Helper()

	workDir := t.TempDir()
	installDir := filepath.Join(workDir, ".install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create .install dir: %v", err)
	}

	for rel, content := range files {
		path := filepath.Join(installDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", rel, err)
		}
	}

	return workDir
}

// storedLibraryChecksumPaths returns the relative paths the library's recorded
// checksums cover, sorted. Those checksums are what `tsuku verify --integrity`
// compares against, so they are the state half of "the reinstall restored the
// original".
func storedLibraryChecksumPaths(t *testing.T, mgr *Manager, name, version string) []string {
	t.Helper()

	state, err := mgr.GetState().Load()
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	versions, ok := state.Libs[name]
	if !ok {
		t.Fatalf("state has no entry for library %s", name)
	}
	libState, ok := versions[version]
	if !ok {
		t.Fatalf("state has no entry for library %s@%s", name, version)
	}

	paths := make([]string, 0, len(libState.Checksums))
	for rel := range libState.Checksums {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

// sortedKeys returns a map's keys in sorted order, for comparison against
// storedLibraryChecksumPaths.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestManager_InstallLibrary_ReinstallReplacesDirectory covers what `tsuku verify`
// promises when it tells a user to run `tsuku install <library> --reinstall` to
// restore the original: the library directory afterwards holds the plan's files
// and nothing else.
//
// Copying over the live directory passes a test that only checks a modified file
// was restored, because copyFile truncates the destination. What it does not do
// is remove a file the plan never wrote, and the checksums recorded after the
// install then claim that file as part of the library.
func TestManager_InstallLibrary_ReinstallReplacesDirectory(t *testing.T) {
	tests := []struct {
		name string
		// first is the .install tree of the original install.
		first map[string]string
		// addedAfterInstall is written straight into the live library
		// directory once the first install has finished, standing in for
		// anything that lands there outside a plan.
		addedAfterInstall map[string]string
		// second is the .install tree of the reinstall.
		second map[string]string
		// wantGone are paths, relative to the library directory, that must
		// not exist once the reinstall has finished.
		wantGone []string
	}{
		{
			name:              "file added after installation",
			first:             map[string]string{"lib/libyaml.so": "original"},
			addedAfterInstall: map[string]string{"lib/evil.so": "payload"},
			second:            map[string]string{"lib/libyaml.so": "original"},
			wantGone:          []string{"lib/evil.so"},
		},
		{
			name:              "modified file restored and added file removed together",
			first:             map[string]string{"lib/libyaml.so": "original"},
			addedAfterInstall: map[string]string{"lib/libyaml.so": "tampered", "lib/evil.so": "payload"},
			second:            map[string]string{"lib/libyaml.so": "original"},
			wantGone:          []string{"lib/evil.so"},
		},
		{
			name:              "whole subdirectory added after installation",
			first:             map[string]string{"lib/libyaml.so": "original"},
			addedAfterInstall: map[string]string{"lib/hooks/preload.so": "payload"},
			second:            map[string]string{"lib/libyaml.so": "original"},
			wantGone:          []string{"lib/hooks/preload.so", "lib/hooks"},
		},
		{
			name:     "file the previous install wrote and the new plan omits",
			first:    map[string]string{"lib/libyaml.so": "v1", "lib/libyaml-compat.so": "v1"},
			second:   map[string]string{"lib/libyaml.so": "v2"},
			wantGone: []string{"lib/libyaml-compat.so"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := testutil.NewTestConfig(t)
			defer cleanup()

			mgr := New(cfg)
			opts := LibraryInstallOptions{}

			if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", newLibraryWorkDir(t, tt.first), opts); err != nil {
				t.Fatalf("first InstallLibrary() error = %v", err)
			}

			libDir := cfg.LibDir("libyaml", "0.2.5")
			for rel, content := range tt.addedAfterInstall {
				path := filepath.Join(libDir, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", rel, err)
				}
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write %s: %v", rel, err)
				}
			}

			if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", newLibraryWorkDir(t, tt.second), opts); err != nil {
				t.Fatalf("reinstall InstallLibrary() error = %v", err)
			}

			for _, rel := range tt.wantGone {
				if _, err := os.Lstat(filepath.Join(libDir, rel)); !os.IsNotExist(err) {
					t.Errorf("%s survived the reinstall; the library directory should hold only what the plan wrote", rel)
				}
			}

			for rel, want := range tt.second {
				got, err := os.ReadFile(filepath.Join(libDir, rel))
				if err != nil {
					t.Errorf("plan file %s missing after reinstall: %v", rel, err)
					continue
				}
				if string(got) != want {
					t.Errorf("plan file %s = %q, want %q", rel, got, want)
				}
			}

			// The recorded checksums are what `verify --integrity` compares
			// against. A survivor recorded here is a survivor the state calls
			// part of the library.
			gotPaths := storedLibraryChecksumPaths(t, mgr, "libyaml", "0.2.5")
			wantPaths := sortedKeys(tt.second)
			if !reflect.DeepEqual(gotPaths, wantPaths) {
				t.Errorf("recorded checksums cover %v, want exactly the plan's files %v", gotPaths, wantPaths)
			}

			assertNoWorkDirectoriesLeft(t, cfg, "libyaml", "0.2.5")
		})
	}
}

// TestManager_InstallLibrary_FailedCopyLeavesPreviousInstallIntact covers the
// second failure mode: without staging, a copy that dies partway leaves the live
// library holding a mix of old and new files, and every tool whose RPATH points
// into it links against that mix.
//
// The failure is injected with a permission the process does not have, the way
// TestInstallWithOptions_RollbackOnSymlinkFailure injects its own. Directory
// entries are walked in name order, so a file copies successfully before the
// unreadable directory is reached -- that is what makes it a partial copy
// rather than a copy that never started.
func TestManager_InstallLibrary_FailedCopyLeavesPreviousInstallIntact(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: an unreadable directory is still readable, so the copy cannot be made to fail")
	}

	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	opts := LibraryInstallOptions{}

	first := map[string]string{"lib/libyaml.so": "original", "share/doc/README": "original docs"}
	if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", newLibraryWorkDir(t, first), opts); err != nil {
		t.Fatalf("first InstallLibrary() error = %v", err)
	}

	// "aaa.txt" sorts before "zzz", so copyDir copies it and then fails.
	failingWorkDir := newLibraryWorkDir(t, map[string]string{"aaa.txt": "replacement"})
	unreadable := filepath.Join(failingWorkDir, ".install", "zzz")
	if err := os.MkdirAll(unreadable, 0755); err != nil {
		t.Fatalf("failed to create source subdirectory: %v", err)
	}
	if err := os.Chmod(unreadable, 0000); err != nil {
		t.Fatalf("failed to make source subdirectory unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0755) })

	if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", failingWorkDir, opts); err == nil {
		t.Fatal("InstallLibrary() should fail when the copy cannot complete")
	}

	libDir := cfg.LibDir("libyaml", "0.2.5")
	for rel, want := range first {
		got, err := os.ReadFile(filepath.Join(libDir, rel))
		if err != nil {
			t.Errorf("previous install lost %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("previous install file %s = %q, want %q", rel, got, want)
		}
	}

	if _, err := os.Stat(filepath.Join(libDir, "aaa.txt")); !os.IsNotExist(err) {
		t.Error("a file from the failed install reached the live library directory")
	}

	assertNoWorkDirectoriesLeft(t, cfg, "libyaml", "0.2.5")
}

// TestManager_InstallLibrary_StaleWorkDirectoriesAreReplaced covers what a crash
// leaves behind. An install killed mid-swap leaves a staging directory, a
// moved-aside previous directory, or both; neither may block or contaminate the
// next install.
func TestManager_InstallLibrary_StaleWorkDirectoriesAreReplaced(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "staging directory", dir: ".libyaml-0.2.5.staging"},
		{name: "moved-aside previous directory", dir: ".libyaml-0.2.5.previous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := testutil.NewTestConfig(t)
			defer cleanup()

			mgr := New(cfg)

			stale := filepath.Join(cfg.LibsDir, tt.dir)
			if err := os.MkdirAll(filepath.Join(stale, "lib"), 0755); err != nil {
				t.Fatalf("failed to create stale dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(stale, "lib", "leftover.so"), []byte("stale"), 0644); err != nil {
				t.Fatalf("failed to write stale file: %v", err)
			}

			workDir := newLibraryWorkDir(t, map[string]string{"lib/libyaml.so": "fresh"})
			if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", workDir, LibraryInstallOptions{}); err != nil {
				t.Fatalf("InstallLibrary() error = %v", err)
			}

			libDir := cfg.LibDir("libyaml", "0.2.5")
			if _, err := os.Stat(filepath.Join(libDir, "lib", "leftover.so")); !os.IsNotExist(err) {
				t.Error("stale content reached the installed library")
			}
			if _, err := os.Stat(filepath.Join(libDir, "lib", "libyaml.so")); err != nil {
				t.Errorf("installed library missing its own file: %v", err)
			}
			assertNoWorkDirectoriesLeft(t, cfg, "libyaml", "0.2.5")
		})
	}
}

// assertNoWorkDirectoriesLeft checks that neither half of the swap for
// name@version is still on disk. Both are dot-prefixed, so both would linger
// unnoticed: the LibsDir scans skip dot-prefixed entries by design.
func assertNoWorkDirectoriesLeft(t *testing.T, cfg *config.Config, name, version string) {
	t.Helper()

	libDir := cfg.LibDir(name, version)
	mgr := New(cfg)
	for _, dir := range []string{mgr.libStagingDir(name, version), previousDirFor(libDir)} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s should not survive the install", filepath.Base(dir))
		}
	}
}

// TestLibsDirScans_IgnoreStagingDirectories pins the two places that enumerate
// $TSUKU_HOME/libs by reading the directory rather than by reading state. A
// staging directory is visible to both while an install is in flight, and stays
// visible if a crash leaves one behind.
func TestLibsDirScans_IgnoreStagingDirectories(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	realLib := cfg.LibDir("libyaml", "0.2.5")
	if err := os.MkdirAll(filepath.Join(realLib, "lib"), 0755); err != nil {
		t.Fatalf("failed to create library dir: %v", err)
	}
	for _, name := range []string{".libffi-3.4.4.staging", ".libffi-3.4.4.previous"} {
		if err := os.MkdirAll(filepath.Join(cfg.LibsDir, name, "lib"), 0755); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	t.Run("ListLibraries", func(t *testing.T) {
		libs, err := mgr.ListLibraries()
		if err != nil {
			t.Fatalf("ListLibraries() error = %v", err)
		}
		if len(libs) != 1 {
			t.Fatalf("ListLibraries() returned %d entries, want 1: %+v", len(libs), libs)
		}
		if libs[0].Name != "libyaml" || libs[0].Version != "0.2.5" {
			t.Errorf("ListLibraries() returned %s@%s, want libyaml@0.2.5", libs[0].Name, libs[0].Version)
		}
	})

	t.Run("collectLibraryPaths", func(t *testing.T) {
		paths := mgr.collectLibraryPaths(libraryTestCtx())
		want := []string{filepath.Join(realLib, "lib")}
		if !reflect.DeepEqual(paths, want) {
			t.Errorf("collectLibraryPaths() = %v, want %v", paths, want)
		}
	})
}

// TestSwapIntoPlace_RestoresPreviousWhenTheSwapFails is the reason the swap
// moves the existing tree aside instead of deleting it. Half a swap must not
// leave the destination missing, because every tool whose RPATH points into a
// library directory resolves through that path.
//
// The failure is injected by pointing the swap at a staging directory that does
// not exist: the move-aside succeeds, the move-in cannot, and the restore is
// what decides whether the destination survives.
func TestSwapIntoPlace_RestoresPreviousWhenTheSwapFails(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "libyaml-0.2.5")
	previous := previousDirFor(dst)
	missingStaging := filepath.Join(root, ".libyaml-0.2.5.staging")

	if err := os.MkdirAll(filepath.Join(dst, "lib"), 0755); err != nil {
		t.Fatalf("failed to create destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "lib", "libyaml.so"), []byte("original"), 0644); err != nil {
		t.Fatalf("failed to write destination file: %v", err)
	}

	if err := swapIntoPlace(missingStaging, dst); err == nil {
		t.Fatal("swapIntoPlace() should fail when the staged tree is not there")
	}

	got, err := os.ReadFile(filepath.Join(dst, "lib", "libyaml.so"))
	if err != nil {
		t.Fatalf("destination did not survive the failed swap: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("destination file = %q, want %q", got, "original")
	}
	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Error("the moved-aside tree should be back at the destination, not left at previous")
	}
}

// TestLibWorkDirs_SharesParentWithLibraryDir pins the two properties the swap
// depends on: both work directories sit in the same parent as the destination,
// so every rename stays within one filesystem, and both are dot-prefixed, so
// the LibsDir scans skip them.
func TestLibWorkDirs_SharesParentWithLibraryDir(t *testing.T) {
	cfg := &config.Config{LibsDir: "/home/user/.tsuku/libs"}
	mgr := New(cfg)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "staging", got: mgr.libStagingDir("libyaml", "0.2.5"), want: ".libyaml-0.2.5.staging"},
		{name: "previous", got: previousDirFor(cfg.LibDir("libyaml", "0.2.5")), want: ".libyaml-0.2.5.previous"},
	}

	wantParent := filepath.Dir(cfg.LibDir("libyaml", "0.2.5"))
	for _, tt := range tests {
		if got := filepath.Dir(tt.got); got != wantParent {
			t.Errorf("%s parent = %q, want %q", tt.name, got, wantParent)
		}
		if got := filepath.Base(tt.got); got != tt.want {
			t.Errorf("%s base = %q, want %q", tt.name, got, tt.want)
		}
	}
}
