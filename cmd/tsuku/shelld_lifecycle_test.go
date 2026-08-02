package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// These tests drive a synthetic tool through install, removal, activation and
// rollback, then read the result out of a real bash subshell that sources
// $TSUKU_HOME/env. The subshell is the point: a fragment can be correct on disk
// and still be missing from the init cache the user's shell actually sources,
// and a test that inspects share/shell.d passes in exactly that case.
//
// cmd/tsuku is where they live because it is the only package where install,
// actions and shellenv meet -- internal/install cannot import internal/actions.

const (
	// envDirVar is the variable the synthetic recipe's set_env step exports.
	// It names {install_dir}, so its value says which version the shell got.
	envDirVar = "TSUKU_E2E_DIR"

	// initVarPrefix names the variable a version's own init script sets. The
	// version is part of the name, so a fragment belonging to the wrong version
	// shows up on its own rather than being masked by whichever export ran last.
	initVarPrefix = "TSUKU_E2E_INIT_"

	// sawVarPrefix names where a version's init script records the value of
	// envDirVar at the moment it was sourced. Exports have to be concatenated
	// ahead of init scripts (the nvm.sh pattern), and only a source-time record
	// can tell the difference between that and a later export overwriting.
	sawVarPrefix = "TSUKU_E2E_SAW_"

	// lifecycleV1 and lifecycleV2 are the two versions every scenario installs.
	lifecycleV1 = "1.0.0"
	lifecycleV2 = "2.0.0"
)

// versionVarSuffix turns a version into a variable-name suffix.
func versionVarSuffix(version string) string {
	return strings.NewReplacer(".", "_", "-", "_", "+", "_").Replace(version)
}

func initVar(version string) string { return initVarPrefix + versionVarSuffix(version) }

func sawVar(version string) string { return sawVarPrefix + versionVarSuffix(version) }

// lifecycleCtx carries the Source the install manager's event bus requires.
func lifecycleCtx() context.Context {
	return installevents.WithSource(context.Background(), installevents.SourceManual)
}

// shellDHarness owns a throwaway $TSUKU_HOME and the manager that operates on
// it. Every mutation goes through the production code path; the assertions read
// the outcome back through bash.
type shellDHarness struct {
	t    *testing.T
	cfg  *config.Config
	mgr  *install.Manager
	home string
	tool string
	bash string
}

func newShellDHarness(t *testing.T) *shellDHarness {
	t.Helper()

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	home := t.TempDir()
	cfg := &config.Config{
		HomeDir:               home,
		ToolsDir:              filepath.Join(home, "tools"),
		CurrentDir:            filepath.Join(home, "tools", "current"),
		RecipesDir:            filepath.Join(home, "recipes"),
		RegistryDir:           filepath.Join(home, "registry"),
		LibsDir:               filepath.Join(home, "libs"),
		AppsDir:               filepath.Join(home, "apps"),
		CacheDir:              filepath.Join(home, "cache"),
		VersionCacheDir:       filepath.Join(home, "cache", "versions"),
		DownloadCacheDir:      filepath.Join(home, "cache", "downloads"),
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

	return &shellDHarness{
		t:    t,
		cfg:  cfg,
		mgr:  install.New(cfg),
		home: home,
		tool: "e2etool",
		bash: bashPath,
	}
}

// initScriptFor is the init script a version of the tool ships. It exports one
// variable naming itself and one recording what set_env's export looked like at
// source time, both keyed on the version.
func initScriptFor(version string) string {
	return "export " + initVar(version) + "=\"" + version + "\"\n" +
		"export " + sawVar(version) + "=\"${" + envDirVar + "-<unset>}\"\n"
}

// installVersion runs the whole install pipeline for one version: the copy into
// tools/<tool>-<version>, then both shell.d writers the way the post-install
// phase runs them, then the cleanup recording and cache rebuild that follows.
//
// set_env exports a variable naming {install_dir}; install_shell_init copies a
// file out of the tool directory whose content differs per version. Two writers,
// two failure modes -- a version key that only one of them applies is a bug.
func (h *shellDHarness) installVersion(version string) {
	h.t.Helper()

	workDir := h.t.TempDir()
	installDir := filepath.Join(workDir, ".install")
	if err := os.MkdirAll(filepath.Join(installDir, "bin"), 0755); err != nil {
		h.t.Fatalf("failed to create staging bin dir: %v", err)
	}
	binary := filepath.Join(installDir, "bin", h.tool)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho "+version+"\n"), 0755); err != nil {
		h.t.Fatalf("failed to write tool binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte(initScriptFor(version)), 0644); err != nil {
		h.t.Fatalf("failed to write tool init script: %v", err)
	}

	opts := install.DefaultInstallOptions()
	opts.Binaries = []string{filepath.Join("bin", h.tool)}
	if err := h.mgr.InstallWithOptions(lifecycleCtx(), h.tool, version, workDir, opts); err != nil {
		h.t.Fatalf("InstallWithOptions(%s) error = %v", version, err)
	}

	// The post-install phase, with ToolInstallDir pointing at the final
	// directory the way cmd/tsuku sets it before ExecutePhase("post-install").
	execCtx := &actions.ExecutionContext{
		Context:        context.Background(),
		WorkDir:        workDir,
		InstallDir:     installDir,
		ToolInstallDir: h.cfg.ToolDir(h.tool, version),
		ToolsDir:       h.cfg.ToolsDir,
		LibsDir:        h.cfg.LibsDir,
		Version:        version,
		Recipe:         &recipe.Recipe{Metadata: recipe.MetadataSection{Name: h.tool}},
	}

	setEnv := &actions.SetEnvAction{}
	if err := setEnv.Execute(execCtx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": envDirVar, "value": "{install_dir}"},
		},
	}); err != nil {
		h.t.Fatalf("set_env Execute(%s) error = %v", version, err)
	}

	shellInit := &actions.InstallShellInitAction{}
	if err := shellInit.Execute(execCtx, map[string]interface{}{
		"source_file": "init.sh",
		"target":      h.tool,
		"shells":      []interface{}{"bash"},
	}); err != nil {
		h.t.Fatalf("install_shell_init Execute(%s) error = %v", version, err)
	}

	finishPostInstall(h.cfg, h.mgr, h.tool, version, execCtx.CleanupActions, func(format string, args ...interface{}) {
		h.t.Errorf("post-install warning for %s: "+format, append([]interface{}{version}, args...)...)
	})
}

// shellVar sources $TSUKU_HOME/env in a hermetic bash subshell and prints one
// variable. --norc --noprofile and a three-entry environment keep the reading
// free of anything the developer's own shell would contribute; the "${VAR-}"
// form makes unset and empty indistinguishable on purpose, since both mean the
// user's shell did not get the value.
func (h *shellDHarness) shellVar(name string) string {
	h.t.Helper()

	cmd := exec.Command(h.bash, "--norc", "--noprofile", "-c",
		`. "$TSUKU_HOME/env"; printf %s "${`+name+`-}"`)
	cmd.Env = []string{"TSUKU_HOME=" + h.home, "HOME=" + h.home, "PATH=/usr/bin:/bin"}
	out, err := cmd.Output()
	if err != nil {
		h.t.Fatalf("sourcing $TSUKU_HOME/env to read %s failed: %v", name, err)
	}
	return string(out)
}

// assertShellSees checks that a real shell got exactly one version of the tool:
// the export points at that version's directory, that version's init script ran
// and saw the export before setting anything of its own, and none of the other
// named versions contributed anything at all.
func (h *shellDHarness) assertShellSees(version string, absent ...string) {
	h.t.Helper()

	wantDir := h.cfg.ToolDir(h.tool, version)
	if got := h.shellVar(envDirVar); got != wantDir {
		h.t.Errorf("the user's shell has %s=%q, want %q (version %s)", envDirVar, got, wantDir, version)
	}
	if got := h.shellVar(initVar(version)); got != version {
		h.t.Errorf("version %s's init script did not reach the shell: %s=%q", version, initVar(version), got)
	}
	if got := h.shellVar(sawVar(version)); got != wantDir {
		h.t.Errorf("version %s's init script saw %s=%q at source time, want %q; the exports must be "+
			"concatenated into the cache ahead of the tool's own init script", version, envDirVar, got, wantDir)
	}
	for _, other := range absent {
		if got := h.shellVar(initVar(other)); got != "" {
			h.t.Errorf("version %s's init script also reached the shell (%s=%q); only the active "+
				"version's fragment belongs in the cache", other, initVar(other), got)
		}
	}
}

// fragmentsFor returns the shell.d paths state records for one version,
// $TSUKU_HOME-relative.
func (h *shellDHarness) fragmentsFor(version string) []string {
	h.t.Helper()

	ts, err := h.mgr.GetToolState(h.tool)
	if err != nil {
		h.t.Fatalf("GetToolState() error = %v", err)
	}
	if ts == nil {
		return nil
	}
	var paths []string
	for _, ca := range ts.Versions[version].CleanupActions {
		if strings.HasPrefix(ca.Path, "share/shell.d/") {
			paths = append(paths, ca.Path)
		}
	}
	return paths
}

// assertFragmentsOnDisk checks the named paths still exist. Deletion is the
// failure mode this catches, and it is silent: doctor walks directory entries,
// so a fragment that is simply gone produces no complaint of any kind.
func (h *shellDHarness) assertFragmentsOnDisk(paths []string) {
	h.t.Helper()

	if len(paths) == 0 {
		h.t.Fatal("no shell.d fragments recorded; the scenario did not set up what it meant to")
	}
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(h.home, filepath.FromSlash(rel))); err != nil {
			h.t.Errorf("%s was deleted: %v", rel, err)
		}
	}
}

// assertNoFragmentFor checks that nothing in shell.d is still keyed on a version
// that is gone.
func (h *shellDHarness) assertNoFragmentFor(version string) {
	h.t.Helper()

	entries, err := os.ReadDir(filepath.Join(h.home, "share", "shell.d"))
	if err != nil {
		h.t.Fatalf("reading shell.d: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "@"+version+".") {
			h.t.Errorf("%s survived the removal of version %s", e.Name(), version)
		}
	}
}

// assertDoctorClean runs the real doctor checks and then the state-versus-disk
// comparison behind doctor's hash diagnostic. The PATH checks fail in a
// throwaway home, so the assertion is on the shell-integration line rather than
// on doctor's overall verdict.
func (h *shellDHarness) assertDoctorClean() {
	h.t.Helper()

	stdout, stderr := captureDoctorOutput(func() {
		runDoctorChecks(h.cfg, h.home)
	})
	if !strings.Contains(stdout, "Shell integration ... ok") {
		h.t.Errorf("doctor did not pass shell integration:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	for _, complaint := range []string{"content hash mismatch", "cache is stale", "syntax error"} {
		if strings.Contains(stderr, complaint) {
			h.t.Errorf("doctor reported %q:\n%s", complaint, stderr)
		}
	}

	state, err := h.mgr.LoadState()
	if err != nil {
		h.t.Fatalf("LoadState() error = %v", err)
	}
	for name, ts := range state.Installed {
		for version, vs := range ts.Versions {
			for _, ca := range vs.CleanupActions {
				if !strings.HasPrefix(ca.Path, "share/shell.d/") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(h.home, filepath.FromSlash(ca.Path)))
				if err != nil {
					h.t.Errorf("state records %s for %s@%s, but it is not on disk: %v", ca.Path, name, version, err)
					continue
				}
				if got := sha256Hex(data); got != ca.ContentHash {
					h.t.Errorf("%s@%s records ContentHash %s for %s, disk has %s",
						name, version, ca.ContentHash, ca.Path, got)
				}
			}
		}
	}
}

// TestShellDLifecycle_RemoveVersion installs two versions of a tool and removes
// one, then asks a real shell which version it got. Both directions matter:
// removing the inactive version must leave the active one alone, and removing
// the active one must promote the survivor into the cache.
func TestShellDLifecycle_RemoveVersion(t *testing.T) {
	tests := []struct {
		name     string
		remove   string
		survivor string
	}{
		{"removing the inactive version keeps the active one sourced", lifecycleV1, lifecycleV2},
		{"removing the active version promotes the remaining one", lifecycleV2, lifecycleV1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newShellDHarness(t)
			h.installVersion(lifecycleV1)
			h.installVersion(lifecycleV2)

			// Both versions are installed and both have fragments on disk; only
			// the active one may reach the shell.
			h.assertShellSees(lifecycleV2, lifecycleV1)

			if err := h.mgr.RemoveVersion(lifecycleCtx(), h.tool, tt.remove); err != nil {
				t.Fatalf("RemoveVersion(%s) error = %v", tt.remove, err)
			}

			h.assertShellSees(tt.survivor, tt.remove)
			h.assertNoFragmentFor(tt.remove)
			h.assertFragmentsOnDisk(h.fragmentsFor(tt.survivor))
			h.assertDoctorClean()
		})
	}
}

// TestShellDLifecycle_ActivateAndRollback switches between two installed
// versions and back. Nothing on disk changes -- both versions' fragments are
// present throughout -- so the only thing that can put the right one in the
// user's shell is a cache rebuilt against the new active version.
func TestShellDLifecycle_ActivateAndRollback(t *testing.T) {
	tests := []struct {
		name     string
		switchTo func(h *shellDHarness, version string) error
	}{
		{"activate", func(h *shellDHarness, version string) error {
			return h.mgr.Activate(lifecycleCtx(), h.tool, version)
		}},
		{"rollback", func(h *shellDHarness, version string) error {
			return h.mgr.Rollback(lifecycleCtx(), h.tool, version)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newShellDHarness(t)
			h.installVersion(lifecycleV1)
			h.installVersion(lifecycleV2)
			h.assertShellSees(lifecycleV2, lifecycleV1)

			if err := tt.switchTo(h, lifecycleV1); err != nil {
				t.Fatalf("switching to %s error = %v", lifecycleV1, err)
			}
			h.assertShellSees(lifecycleV1, lifecycleV2)

			if err := tt.switchTo(h, lifecycleV2); err != nil {
				t.Fatalf("switching back to %s error = %v", lifecycleV2, err)
			}
			h.assertShellSees(lifecycleV2, lifecycleV1)
			h.assertDoctorClean()
		})
	}
}

// TestShellDLifecycle_UpdateThenRollback covers the sequence version-keyed
// filenames made dangerous. After an update, every fragment the old version
// recorded looks stale -- the new version records different paths entirely --
// so a stale cleanup without the retained-path guard deletes the rollback
// target's shell integration. The deletion is silent, and the user only finds
// out when the shell they roll back into has lost the tool.
func TestShellDLifecycle_UpdateThenRollback(t *testing.T) {
	h := newShellDHarness(t)

	h.installVersion(lifecycleV1)
	oldCleanup := h.cleanupActionsFor(lifecycleV1)

	h.installVersion(lifecycleV2)
	newCleanup := h.cleanupActionsFor(lifecycleV2)

	// What `tsuku update` does once the new version is in place.
	stale := install.StaleCleanupActions(oldCleanup, newCleanup)
	if len(stale) == 0 {
		t.Fatal("expected the old version's version-keyed fragments to read as stale")
	}
	h.mgr.ExecuteStaleCleanup(stale)

	h.assertFragmentsOnDisk(h.fragmentsFor(lifecycleV1))
	h.assertShellSees(lifecycleV2, lifecycleV1)

	if err := h.mgr.Rollback(lifecycleCtx(), h.tool, lifecycleV1); err != nil {
		t.Fatalf("Rollback(%s) error = %v", lifecycleV1, err)
	}
	h.assertShellSees(lifecycleV1, lifecycleV2)
	h.assertDoctorClean()
}

// cleanupActionsFor returns every cleanup action state records for a version,
// which is what the update path snapshots before installing over it.
func (h *shellDHarness) cleanupActionsFor(version string) []install.CleanupAction {
	h.t.Helper()

	ts, err := h.mgr.GetToolState(h.tool)
	if err != nil {
		h.t.Fatalf("GetToolState() error = %v", err)
	}
	if ts == nil {
		h.t.Fatalf("tool %q is not installed", h.tool)
	}
	return ts.Versions[version].CleanupActions
}
