package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

func TestGetEvalDeps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action   string
		wantDeps []string
	}{
		{"npm_install", []string{"nodejs"}},
		{"go_install", []string{"go"}},
		{"download", nil},       // Core primitive, no eval deps
		{"extract", nil},        // Core primitive, no eval deps
		{"unknown_action", nil}, // Unknown action
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			deps := GetEvalDeps(tc.action)
			if len(deps) != len(tc.wantDeps) {
				t.Errorf("GetEvalDeps(%q) = %v, want %v", tc.action, deps, tc.wantDeps)
				return
			}
			for i, dep := range deps {
				if dep != tc.wantDeps[i] {
					t.Errorf("GetEvalDeps(%q)[%d] = %q, want %q", tc.action, i, dep, tc.wantDeps[i])
				}
			}
		})
	}
}

// testHome returns a config rooted at an empty tsuku home.
func testHome(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	return &config.Config{
		HomeDir:  home,
		ToolsDir: filepath.Join(home, "tools"),
	}
}

// installTool leaves a tool behind the way tsuku does when it installs an
// eval-time dependency: a version directory under $TSUKU_HOME/tools and an
// entry in state.json. Both halves matter -- the point of the check is that
// the directory on its own does not make a tool installed.
func installTool(t *testing.T, cfg *config.Config, name, version string) {
	t.Helper()
	if err := os.MkdirAll(cfg.ToolDir(name, version), 0o755); err != nil {
		t.Fatalf("create %s-%s directory: %v", name, version, err)
	}
	if err := install.New(cfg).EnsureDependencyEntry(name, version, "", nil); err != nil {
		t.Fatalf("record %s-%s in state: %v", name, version, err)
	}
}

func assertMissing(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("missing = %v, want %v", got, want)
		}
	}
}

// A tool whose name is a prefix of another tool's is not that other tool. The
// registry ships dozens of these pairs, so "a directory starting with <dep>-"
// was never a safe way to answer whether <dep> is installed.
func TestCheckEvalDepsInHome_AsksStateNotDirectoryNames(t *testing.T) {
	t.Parallel()

	type tool struct{ name, version string }

	tests := []struct {
		name        string
		installed   []tool
		deps        []string
		wantMissing []string
	}{
		{
			name:        "go-task does not satisfy go",
			installed:   []tool{{"go-task", "3.44.0"}},
			deps:        []string{"go"},
			wantMissing: []string{"go"},
		},
		{
			name:        "git-lfs does not satisfy git",
			installed:   []tool{{"git-lfs", "3.5.0"}},
			deps:        []string{"git"},
			wantMissing: []string{"git"},
		},
		{
			name:      "go satisfies go",
			installed: []tool{{"go", "1.23.4"}},
			deps:      []string{"go"},
		},
		{
			name:      "go satisfies go with go-task alongside it",
			installed: []tool{{"go", "1.23.4"}, {"go-task", "3.44.0"}},
			deps:      []string{"go"},
		},
		{
			name:        "nothing installed",
			deps:        []string{"nodejs", "go"},
			wantMissing: []string{"nodejs", "go"},
		},
		{
			name:        "rust-analyzer does not satisfy rust, nodejs satisfies nodejs",
			installed:   []tool{{"nodejs", "20.0.0"}, {"rust-analyzer", "2025.1.1"}},
			deps:        []string{"nodejs", "rust"},
			wantMissing: []string{"rust"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := testHome(t)
			for _, installed := range tc.installed {
				installTool(t, cfg, installed.name, installed.version)
			}
			assertMissing(t, checkEvalDepsInHome(tc.deps, cfg.HomeDir), tc.wantMissing)
		})
	}
}

// State claiming a version whose directory is gone is not an installed tool.
// The decomposer that runs next needs the binary, so reporting the dependency
// missing is what gets it reinstalled instead of failing mid-decomposition.
func TestCheckEvalDepsInHome_StateWithoutDirectory(t *testing.T) {
	t.Parallel()

	cfg := testHome(t)
	installTool(t, cfg, "go", "1.23.4")
	if err := os.RemoveAll(cfg.ToolDir("go", "1.23.4")); err != nil {
		t.Fatalf("remove go directory: %v", err)
	}

	assertMissing(t, checkEvalDepsInHome([]string{"go"}, cfg.HomeDir), []string{"go"})
}

// A directory with no state entry behind it is not an installed tool either.
// That is the shape a hand-made directory leaves, and it is also all the old
// prefix match was ever really keying on.
func TestCheckEvalDepsInHome_DirectoryWithoutState(t *testing.T) {
	t.Parallel()

	cfg := testHome(t)
	if err := os.MkdirAll(cfg.ToolDir("go", "1.23.4"), 0o755); err != nil {
		t.Fatalf("create go directory: %v", err)
	}

	assertMissing(t, checkEvalDepsInHome([]string{"go"}, cfg.HomeDir), []string{"go"})
}

// Only a real directory at $TSUKU_HOME/tools/<tool>-<version> counts, which
// is what the ReadDir scan this replaced reported: DirEntry.IsDir is false for
// a plain file and does not follow symlinks.
func TestCheckEvalDepsInHome_OnlyARealDirectoryCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		place func(t *testing.T, path string)
	}{
		{
			name: "a file where the version directory should be",
			place: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("not a tool"), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
			},
		},
		{
			name: "a symlink where the version directory should be",
			place: func(t *testing.T, path string) {
				target := filepath.Join(t.TempDir(), "elsewhere")
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatalf("create %s: %v", target, err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("symlink %s: %v", path, err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := testHome(t)
			installTool(t, cfg, "go", "1.23.4")
			if err := os.RemoveAll(cfg.ToolDir("go", "1.23.4")); err != nil {
				t.Fatalf("remove go directory: %v", err)
			}
			tc.place(t, cfg.ToolDir("go", "1.23.4"))

			assertMissing(t, checkEvalDepsInHome([]string{"go"}, cfg.HomeDir), []string{"go"})
		})
	}
}

// A state file that cannot be read answers nothing, and nothing is not
// "installed". Reporting the dependency satisfied here would put the caller
// back where the prefix match left it: decomposing against a binary no one
// established was there.
func TestCheckEvalDepsInHome_UnreadableState(t *testing.T) {
	t.Parallel()

	cfg := testHome(t)
	installTool(t, cfg, "go", "1.23.4")
	if err := os.WriteFile(filepath.Join(cfg.HomeDir, "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("corrupt state.json: %v", err)
	}

	assertMissing(t, checkEvalDepsInHome([]string{"go"}, cfg.HomeDir), []string{"go"})
}

// The exported entry point reads $TSUKU_HOME, so drive it that way once.
func TestCheckEvalDeps_ResolvesTsukuHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)

	cfg := &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}

	installTool(t, cfg, "go-task", "3.44.0")
	assertMissing(t, CheckEvalDeps([]string{"go"}), []string{"go"})

	installTool(t, cfg, "go", "1.23.4")
	assertMissing(t, CheckEvalDeps([]string{"go"}), nil)
}

func TestCheckEvalDeps_EmptyList(t *testing.T) {
	t.Parallel()

	missing := CheckEvalDeps(nil)
	if missing != nil {
		t.Errorf("CheckEvalDeps(nil) = %v, want nil", missing)
	}

	missing = CheckEvalDeps([]string{})
	if missing != nil {
		t.Errorf("CheckEvalDeps([]) = %v, want nil", missing)
	}
}

// -- eval_deps.go: GetEvalDeps --

func TestGetEvalDeps_UnknownAction(t *testing.T) {
	t.Parallel()
	deps := GetEvalDeps("nonexistent_action_xyz")
	if deps != nil {
		t.Errorf("GetEvalDeps(unknown) = %v, want nil", deps)
	}
}

func TestGetEvalDeps_KnownAction(t *testing.T) {
	t.Parallel()
	deps := GetEvalDeps("gem_install")
	// gem_install has ruby as eval-time dep
	if len(deps) == 0 {
		t.Error("GetEvalDeps(gem_install) should return eval-time deps")
	}
}
