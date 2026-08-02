package shellenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// versionedHome lays out two versions of one tool plus an unrecorded fragment
// from a writer that never records a cleanup action.
func versionedHome(t *testing.T) (tsukuHome, shellDDir string) {
	t.Helper()
	tsukuHome = t.TempDir()
	shellDDir = filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(shellDDir, "nvm@0.40.5.bash"), "# v5\n")
	writeTestFile(t, filepath.Join(shellDDir, "nvm@0.40.6.bash"), "# v6\n")
	writeTestFile(t, filepath.Join(shellDDir, "unrecorded.bash"), "# unrecorded\n")
	return tsukuHome, shellDDir
}

// activeSixth records both versions but marks only 0.40.6 active.
func activeSixth() ShellDSelection {
	return ShellDSelection{
		Active: map[string]string{
			"share/shell.d/nvm@0.40.6.bash": testHash("# v6\n"),
		},
		Known: map[string]string{
			"share/shell.d/nvm@0.40.5.bash": testHash("# v5\n"),
			"share/shell.d/nvm@0.40.6.bash": testHash("# v6\n"),
		},
	}
}

func TestRebuildShellCache_ExcludesRecordedInactiveVersions(t *testing.T) {
	tsukuHome, shellDDir := versionedHome(t)

	if err := RebuildShellCache(tsukuHome, "bash", activeSixth()); err != nil {
		t.Fatalf("RebuildShellCache() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}

	// The inactive version is out; the active one and the unrecorded one are in.
	// Unrecorded files keeping their pre-version-key treatment is the whole
	// reason the rule is "Known and not Active" rather than "not Active".
	want := wrapExpected("nvm", "# v6\n") + wrapExpected("unrecorded", "# unrecorded\n")
	if string(content) != want {
		t.Errorf("cache = %q, want %q", content, want)
	}
}

func TestRebuildShellCache_NoSelectionExcludesNothing(t *testing.T) {
	tsukuHome, shellDDir := versionedHome(t)

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}

	// Passing nothing must mean excluding nothing, so every caller that has not
	// been taught about the selection keeps its current semantics.
	want := wrapExpected("nvm", "# v5\n") + wrapExpected("nvm", "# v6\n") +
		wrapExpected("unrecorded", "# unrecorded\n")
	if string(content) != want {
		t.Errorf("cache = %q, want %q", content, want)
	}
}

func TestRebuildShellCache_DisplayNameDropsTheVersionKey(t *testing.T) {
	tsukuHome, shellDDir := versionedHome(t)

	if err := RebuildShellCache(tsukuHome, "bash", activeSixth()); err != nil {
		t.Fatalf("RebuildShellCache() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}
	if got := string(content); !strings.Contains(got, "# tsuku: nvm\n") || strings.Contains(got, "nvm@0.40.6") {
		t.Errorf("cache should name the tool nvm, not nvm@0.40.6; got %q", got)
	}
}

func TestCheckShellD_AgreesWithRebuildOnExclusions(t *testing.T) {
	tsukuHome, _ := versionedHome(t)
	selection := activeSixth()

	if err := RebuildShellCache(tsukuHome, "bash", selection); err != nil {
		t.Fatalf("RebuildShellCache() error = %v", err)
	}

	// The two must agree, or doctor reports a stale cache on every
	// multi-version install and --fix rebuilds it into the same state.
	result := CheckShellD(tsukuHome, selection)
	if result.CacheStale["bash"] {
		t.Error("CheckShellD reported the cache stale immediately after a rebuild with the same selection")
	}

	scripts := result.ActiveScripts["bash"]
	if len(scripts) != 2 || scripts[0] != "nvm" || scripts[1] != "unrecorded" {
		t.Errorf("ActiveScripts = %v, want [nvm unrecorded]", scripts)
	}
}

func TestShellDSelection_Excludes(t *testing.T) {
	sel := activeSixth()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"active", "share/shell.d/nvm@0.40.6.bash", false},
		{"known but inactive", "share/shell.d/nvm@0.40.5.bash", true},
		{"unrecorded", "share/shell.d/unrecorded.bash", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sel.excludes(tt.path); got != tt.want {
				t.Errorf("excludes(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	// The zero selection is what every caller that passes nothing gets.
	if (ShellDSelection{}).excludes("share/shell.d/anything.bash") {
		t.Error("the zero selection must exclude nothing")
	}
}
