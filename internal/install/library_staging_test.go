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
		})
	}
}

// TestManager_InstallLibrary_FailedCopyLeavesPreviousInstallIntact covers the
// second failure mode: without staging, a copy that dies partway leaves the live
// library holding a mix of old and new files, and every tool whose RPATH points
// into it links against that mix.
//
// The failure is injected the way TestManager_InstallWithOptions_SymlinkFailureRollback
// injects its own: something the process cannot read. Directory entries are
// walked in name order, so a file copies successfully before the unreadable
// directory is reached -- that is what makes it a partial copy rather than a
// copy that never started.
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

	stagingDir := filepath.Join(cfg.LibsDir, ".libyaml-0.2.5.staging")
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Error("staging directory should not survive a failed install")
	}
}

// TestManager_InstallLibrary_StaleStagingDirectoryIsReplaced covers what a crash
// leaves behind: an install killed between creating the staging directory and
// renaming it must not block or contaminate the next one.
func TestManager_InstallLibrary_StaleStagingDirectoryIsReplaced(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	stagingDir := filepath.Join(cfg.LibsDir, ".libyaml-0.2.5.staging")
	if err := os.MkdirAll(filepath.Join(stagingDir, "lib"), 0755); err != nil {
		t.Fatalf("failed to create stale staging dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "lib", "leftover.so"), []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to write stale staging file: %v", err)
	}

	workDir := newLibraryWorkDir(t, map[string]string{"lib/libyaml.so": "fresh"})
	if err := mgr.InstallLibrary(libraryTestCtx(), "libyaml", "0.2.5", workDir, LibraryInstallOptions{}); err != nil {
		t.Fatalf("InstallLibrary() error = %v", err)
	}

	libDir := cfg.LibDir("libyaml", "0.2.5")
	if _, err := os.Stat(filepath.Join(libDir, "lib", "leftover.so")); !os.IsNotExist(err) {
		t.Error("stale staging content reached the installed library")
	}
	if _, err := os.Stat(filepath.Join(libDir, "lib", "libyaml.so")); err != nil {
		t.Errorf("installed library missing its own file: %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Error("staging directory should not survive a successful install")
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
	staging := filepath.Join(cfg.LibsDir, ".libffi-3.4.4.staging")
	if err := os.MkdirAll(filepath.Join(staging, "lib"), 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
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

// TestLibStagingDir_SharesParentWithLibraryDir pins the property the atomic
// rename depends on: staging and destination sit in the same directory, so they
// are always on the same filesystem.
func TestLibStagingDir_SharesParentWithLibraryDir(t *testing.T) {
	cfg := &config.Config{LibsDir: "/home/user/.tsuku/libs"}
	mgr := New(cfg)

	staging := mgr.libStagingDir("libyaml", "0.2.5")
	if got, want := filepath.Dir(staging), filepath.Dir(cfg.LibDir("libyaml", "0.2.5")); got != want {
		t.Errorf("libStagingDir parent = %q, want %q", got, want)
	}
	if got, want := filepath.Base(staging), ".libyaml-0.2.5.staging"; got != want {
		t.Errorf("libStagingDir base = %q, want %q", got, want)
	}
}
