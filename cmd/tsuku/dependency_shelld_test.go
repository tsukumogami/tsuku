package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// A dependency's post-install steps write outside the dependency's own install
// directory, and until issue #2462 nothing recorded that they had. The executor
// installs a dependency itself -- not through install.Manager -- so the files
// its steps wrote had no owner in state.json: removing the tool that pulled the
// dependency in, and removing the dependency itself, both left the fragment on
// disk, where the cache builder kept concatenating it into the user's shell
// forever.
//
// These tests drive the real executor dependency path and then remove
// everything, because the intermediate record is not the thing that matters. A
// recorded cleanup action that removal never executes leaves exactly the same
// orphan as no record at all.

const (
	depParentTool    = "parenttool"
	depParentVersion = "2.0.0"
	depTool          = "deptool"
	depVersion       = "0.1.0"

	// depInitMarker is the variable every fragment in these tests defines. Its
	// presence in the init cache is how a leftover fragment announces itself
	// after removal was supposed to have taken it away.
	depInitMarker = "TSUKU_DEP_INIT"
)

// depHarness owns a throwaway $TSUKU_HOME plus the executor and manager that
// operate on it.
type depHarness struct {
	t    *testing.T
	cfg  *config.Config
	mgr  *install.Manager
	home string
}

func newDepHarness(t *testing.T) *depHarness {
	t.Helper()

	home := t.TempDir()
	cfg := &config.Config{
		HomeDir:          home,
		ToolsDir:         filepath.Join(home, "tools"),
		CurrentDir:       filepath.Join(home, "tools", "current"),
		RecipesDir:       filepath.Join(home, "recipes"),
		RegistryDir:      filepath.Join(home, "registry"),
		LibsDir:          filepath.Join(home, "libs"),
		AppsDir:          filepath.Join(home, "apps"),
		CacheDir:         filepath.Join(home, "cache"),
		VersionCacheDir:  filepath.Join(home, "cache", "versions"),
		DownloadCacheDir: filepath.Join(home, "cache", "downloads"),

		CargoRegistryCacheDir: filepath.Join(home, "cache", "cargo-registry"),
		KeyCacheDir:           filepath.Join(home, "cache", "keys"),
		TapCacheDir:           filepath.Join(home, "cache", "taps"),
		ShareDir:              filepath.Join(home, "share"),
		ConfigFile:            filepath.Join(home, "config.toml"),
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}
	if err := cfg.EnsureEnvFile(); err != nil {
		t.Fatalf("EnsureEnvFile() error = %v", err)
	}

	return &depHarness{t: t, cfg: cfg, mgr: install.New(cfg), home: home}
}

// shellDDir is where every fragment in these tests lands.
func (h *depHarness) shellDDir() string {
	return filepath.Join(h.home, "share", "shell.d")
}

// writeInitBinaryStep produces the dependency's install phase: a tiny executable
// in the staging bin directory. install_shell_init's source_command mode refuses
// to run anything outside ToolInstallDir, so the dependency has to ship a real
// binary for the post-install step to have something legal to call.
func writeInitBinaryStep() executor.ResolvedStep {
	script := "#!/bin/sh\necho \"export TSUKU_DEP_INIT=$1\"\n"
	return executor.ResolvedStep{
		Action:    "run_command",
		Evaluable: true,
		Params: map[string]interface{}{
			"command": "mkdir -p {install_dir}/bin && " +
				"printf '%s' '" + script + "' > {install_dir}/bin/depinit && " +
				"chmod 0755 {install_dir}/bin/depinit",
		},
	}
}

// installParentWithDependency runs the whole path a real install takes: the
// executor installs the dependency and the main tool, the manager moves the main
// tool into place, and the cmd layer records what both left behind.
func (h *depHarness) installParentWithDependency(depSteps []executor.ResolvedStep) []executor.DependencyInstall {
	return h.installParentWithDependencyOpts(depSteps, false)
}

// installParentWithDependencyOpts is installParentWithDependency with the
// --no-shell-init flag exposed, since a dependency has to honour it too.
func (h *depHarness) installParentWithDependencyOpts(
	depSteps []executor.ResolvedStep,
	noShellInit bool,
) []executor.DependencyInstall {
	h.t.Helper()

	rec := &recipe.Recipe{Metadata: recipe.MetadataSection{Name: depParentTool, Type: "tool"}}
	exec, err := executor.NewWithVersion(rec, depParentVersion)
	if err != nil {
		h.t.Fatalf("NewWithVersion() error = %v", err)
	}
	defer exec.Cleanup()

	exec.SetToolsDir(h.cfg.ToolsDir)
	exec.SetLibsDir(h.cfg.LibsDir)
	exec.SetAppsDir(h.cfg.AppsDir)
	exec.SetCurrentDir(h.cfg.CurrentDir)
	exec.SetSkipCacheSecurityChecks(true)
	exec.SetNoShellInit(noShellInit)

	plan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          depParentTool,
		Version:       depParentVersion,
		Platform:      executor.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Dependencies: []executor.DependencyPlan{{
			Tool:       depTool,
			Version:    depVersion,
			RecipeType: "tool",
			Steps:      append([]executor.ResolvedStep{writeInitBinaryStep()}, depSteps...),
		}},
		Steps: []executor.ResolvedStep{},
	}

	if err := exec.ExecutePlan(context.Background(), plan); err != nil {
		h.t.Fatalf("ExecutePlan() error = %v", err)
	}

	opts := install.DefaultInstallOptions()
	if err := h.mgr.InstallWithOptions(lifecycleCtx(), depParentTool, depParentVersion, exec.WorkDir(), opts); err != nil {
		h.t.Fatalf("InstallWithOptions() error = %v", err)
	}

	deps := exec.GetDependencyInstalls()
	recordDependencyInstalls(h.cfg, h.mgr, depParentTool, deps, func(format string, args ...interface{}) {
		h.t.Errorf("unexpected warning while recording dependencies: "+format, args...)
	})
	return deps
}

// assertShellDEmpty is the acceptance criterion itself: after everything is
// removed, no fragment may remain in $TSUKU_HOME/share/shell.d. Tsuku's own
// bookkeeping files there are dot-prefixed and are allowed to stay -- a
// fragment cannot be, since validateShellDTarget rejects a leading dot -- but
// the init cache must no longer carry what the dependency wrote.
func (h *depHarness) assertShellDEmpty() {
	h.t.Helper()

	entries, err := os.ReadDir(h.shellDDir())
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		h.t.Fatalf("reading shell.d: %v", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			h.t.Errorf("%s survived the removal of both the tool and its dependency", e.Name())
			continue
		}
		if !strings.HasPrefix(e.Name(), ".init-cache.") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(h.shellDDir(), e.Name()))
		if readErr != nil {
			h.t.Fatalf("reading %s: %v", e.Name(), readErr)
		}
		if strings.Contains(string(data), depInitMarker) {
			h.t.Errorf("%s still sources the removed dependency's fragment:\n%s", e.Name(), data)
		}
	}
}

// fragmentPathsFor reads back what state records for one tool version, sorted so
// the comparison does not depend on the order the action wrote its shells in.
func (h *depHarness) fragmentPathsFor(tool, version string) []string {
	h.t.Helper()

	ts, err := h.mgr.GetToolState(tool)
	if err != nil {
		h.t.Fatalf("GetToolState(%s) error = %v", tool, err)
	}
	if ts == nil {
		return nil
	}
	var paths []string
	for _, ca := range ts.Versions[version].CleanupActions {
		paths = append(paths, ca.Path)
	}
	slices.Sort(paths)
	return paths
}

// TestDependencyShellD_RecordedAgainstTheDependency covers the whole defect at
// the level a user feels it: install a tool whose dependency writes a shell.d
// fragment, remove both, and assert nothing is left behind.
//
// The two step shapes are the two actions that write outside the install
// directory, and both used to fail here for different reasons. source_command
// validated its executable against an empty ToolInstallDir and rejected
// everything; set_env refused outright. Both now run, so both now have to be
// cleaned up.
func TestDependencyShellD_RecordedAgainstTheDependency(t *testing.T) {
	tests := []struct {
		name      string
		step      executor.ResolvedStep
		fragments []string
	}{
		{
			name: "install_shell_init with source_command",
			step: executor.ResolvedStep{
				Action:    "install_shell_init",
				Phase:     "post-install",
				Evaluable: true,
				Params: map[string]interface{}{
					"source_command": "{install_dir}/bin/depinit {shell}",
					"target":         depTool,
					"shells":         []interface{}{"bash"},
				},
			},
			fragments: []string{"share/shell.d/" + depTool + "@" + depVersion + ".bash"},
		},
		{
			name: "set_env",
			step: executor.ResolvedStep{
				Action:    "set_env",
				Evaluable: true,
				Params: map[string]interface{}{
					"vars": []interface{}{
						map[string]interface{}{"name": depInitMarker, "value": "{install_dir}"},
					},
				},
			},
			fragments: []string{
				"share/shell.d/00-env-" + depTool + "@" + depVersion + ".bash",
				"share/shell.d/00-env-" + depTool + "@" + depVersion + ".zsh",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDepHarness(t)
			deps := h.installParentWithDependency([]executor.ResolvedStep{tt.step})

			// The executor has to hand the dependency's cleanup actions back.
			// Dropping them is the original defect; everything below is
			// unreachable without this.
			if len(deps) != 1 {
				t.Fatalf("GetDependencyInstalls() returned %d entries, want 1", len(deps))
			}
			if len(deps[0].CleanupActions) == 0 {
				t.Fatal("the dependency's steps recorded no cleanup actions; they were discarded")
			}

			// The fragment belongs to the dependency, not to the tool that
			// pulled it in: it is named after the dependency and keyed on the
			// dependency's version, so the state entry that decides whether it
			// belongs in the shell cache has to be the dependency's too.
			if got := h.fragmentPathsFor(depTool, depVersion); !slices.Equal(got, tt.fragments) {
				t.Errorf("%s@%s records %v, want %v", depTool, depVersion, got, tt.fragments)
			}
			if got := h.fragmentPathsFor(depParentTool, depParentVersion); len(got) != 0 {
				t.Errorf("%s@%s records %v, want nothing -- the fragment is not its file",
					depParentTool, depParentVersion, got)
			}

			for _, fragment := range tt.fragments {
				onDisk := filepath.Join(h.home, filepath.FromSlash(fragment))
				if _, err := os.Stat(onDisk); err != nil {
					t.Fatalf("the dependency's post-install step did not write %s: %v", fragment, err)
				}
			}

			if err := h.mgr.RemoveAllVersions(lifecycleCtx(), depParentTool); err != nil {
				t.Fatalf("RemoveAllVersions(%s) error = %v", depParentTool, err)
			}
			if err := h.mgr.RemoveAllVersions(lifecycleCtx(), depTool); err != nil {
				t.Fatalf("RemoveAllVersions(%s) error = %v", depTool, err)
			}

			h.assertShellDEmpty()
		})
	}
}

// TestDependencyShellD_HiddenAndAttributed checks the shape of the state entry
// the dependency gets. It is deliberately not a tool the user asked for: it must
// not show up in `tsuku list`, and it must name the tool that pulled it in so
// removing it warns rather than silently breaking that tool.
func TestDependencyShellD_HiddenAndAttributed(t *testing.T) {
	h := newDepHarness(t)
	h.installParentWithDependency(nil)

	ts, err := h.mgr.GetToolState(depTool)
	if err != nil || ts == nil {
		t.Fatalf("GetToolState(%s) = %v, %v; the dependency has no state entry", depTool, ts, err)
	}
	if !ts.IsHidden || !ts.IsExecutionDependency {
		t.Errorf("dependency state is IsHidden=%v IsExecutionDependency=%v, want both true",
			ts.IsHidden, ts.IsExecutionDependency)
	}
	if ts.IsExplicit {
		t.Error("the user never asked for this tool, so it must not be marked explicit")
	}
	if ts.ActiveVersion != depVersion {
		t.Errorf("ActiveVersion = %q, want %q", ts.ActiveVersion, depVersion)
	}
	if len(ts.RequiredBy) != 1 || ts.RequiredBy[0] != depParentTool {
		t.Errorf("RequiredBy = %v, want [%s]", ts.RequiredBy, depParentTool)
	}

	visible, err := h.mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, tool := range visible {
		if tool.Name == depTool {
			t.Errorf("%s appears in the default tool list; a dependency the user never asked for should stay hidden", depTool)
		}
	}
}

// TestDependencyShellD_KeepsTheUsersActiveVersion covers the case where the tool
// a plan pins as a dependency is already installed at another version. Installing
// the dependency must not move the active version, because the active version is
// what the shell.d projection sources and what `tsuku run` resolves.
func TestDependencyShellD_KeepsTheUsersActiveVersion(t *testing.T) {
	h := newDepHarness(t)

	// The user installed this tool themselves, at a different version.
	userVersion := "9.9.9"
	if err := h.mgr.GetState().UpdateTool(depTool, func(ts *install.ToolState) {
		ts.ActiveVersion = userVersion
		ts.IsExplicit = true
		ts.Versions = map[string]install.VersionState{userVersion: {}}
	}); err != nil {
		t.Fatalf("seeding state error = %v", err)
	}

	h.installParentWithDependency([]executor.ResolvedStep{{
		Action:    "set_env",
		Evaluable: true,
		Params: map[string]interface{}{
			"vars": []interface{}{
				map[string]interface{}{"name": depInitMarker, "value": "{install_dir}"},
			},
		},
	}})

	ts, err := h.mgr.GetToolState(depTool)
	if err != nil || ts == nil {
		t.Fatalf("GetToolState(%s) = %v, %v", depTool, ts, err)
	}
	if ts.ActiveVersion != userVersion {
		t.Errorf("ActiveVersion = %q, want %q -- installing a dependency is not a request to switch versions",
			ts.ActiveVersion, userVersion)
	}
	if ts.IsHidden {
		t.Error("a tool the user installed explicitly must not become hidden because something depended on it")
	}

	// The record must land on the version that was installed, not on whichever
	// version happens to be active.
	if got := h.fragmentPathsFor(depTool, depVersion); len(got) == 0 {
		t.Errorf("%s@%s records nothing, want its own fragment", depTool, depVersion)
	}
	if got := h.fragmentPathsFor(depTool, userVersion); len(got) != 0 {
		t.Errorf("%s@%s records %v, want nothing -- that version wrote none of it", depTool, userVersion, got)
	}
}

// TestDependencyShellD_HonoursNoShellInit checks that --no-shell-init reaches a
// dependency. The flag is the user saying they manage their own shell config;
// honouring it for the tool they named while a dependency writes to shell.d
// behind their back would be worse than not offering the flag.
func TestDependencyShellD_HonoursNoShellInit(t *testing.T) {
	h := newDepHarness(t)

	deps := h.installParentWithDependencyOpts([]executor.ResolvedStep{{
		Action:    "set_env",
		Evaluable: true,
		Params: map[string]interface{}{
			"vars": []interface{}{
				map[string]interface{}{"name": depInitMarker, "value": "{install_dir}"},
			},
		},
	}}, true)

	if len(deps) != 1 {
		t.Fatalf("GetDependencyInstalls() returned %d entries, want 1", len(deps))
	}
	if len(deps[0].CleanupActions) != 0 {
		t.Errorf("the dependency recorded %v under --no-shell-init, want nothing",
			deps[0].CleanupActions)
	}

	entries, err := os.ReadDir(h.shellDDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading shell.d: %v", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			t.Errorf("--no-shell-init still let the dependency write %s", e.Name())
		}
	}
}
