package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// --reinstall exists so an install that is already on disk can pick up a fix.
// The only way to tell whether it did is to look at the files afterwards: the
// flag parsing, the short-circuit, and the state write are all one layer short
// of what the user experiences. Every test here therefore tampers with the
// installed tree, runs the install, and reads the tree back.
//
// The install runs offline. A tool that is already installed has its plan in
// state.json, and getOrGeneratePlan prefers that cached plan, so seeding state
// the way a previous install would have left it is enough to drive the real
// installWithDependencies path without touching the network.

const (
	reinstallTool    = "reinstalltool"
	reinstallVersion = "dev"

	// reinstallGoodContent is what the plan writes. reinstallBadContent stands
	// in for whatever the tool's files became -- a bad write from an older
	// tsuku, or a modification tsuku verify reported.
	reinstallGoodContent = "installed-by-the-plan\n"
	reinstallBadContent  = "tampered-with\n"

	// reinstallEnvVar is the variable the recipe's set_env step exports. The
	// fragment carrying it is the file written outside the tool directory.
	reinstallEnvVar = "TSUKU_REINSTALL_MARKER"
)

// reinstallHarness owns a throwaway $TSUKU_HOME with one tool already
// installed, plus the recipe and cached plan that installed it.
type reinstallHarness struct {
	t    *testing.T
	home string
	cfg  *config.Config
	mgr  *install.Manager
}

// markerPathFor is the file inside a tool's directory that its plan writes and
// the tests tamper with.
func (h *reinstallHarness) markerPathFor(tool string) string {
	return filepath.Join(h.cfg.ToolDir(tool, reinstallVersion), "share", "marker.txt")
}

func (h *reinstallHarness) markerPath() string {
	return h.markerPathFor(reinstallTool)
}

// binaryPath is the tool's binary inside its install directory.
func (h *reinstallHarness) binaryPath() string {
	return filepath.Join(h.cfg.ToolDir(reinstallTool, reinstallVersion), "bin", reinstallTool)
}

// fragmentPath is where the set_env step's bash fragment lands, relative to
// $TSUKU_HOME. Shell fragments are keyed by tool and version, so a reinstall of
// the same version writes this same path rather than a second one.
func fragmentPath(shell string) string {
	return filepath.Join("share", "shell.d", "00-env-"+reinstallTool+"@"+reinstallVersion+"."+shell)
}

// reinstallPlanSteps is the installation a cached plan describes: a binary, a
// marker file with known content, and a set_env step that writes a shell
// fragment outside the tool directory.
//
// run_command is marked evaluable so plan validation accepts it; set_env's own
// default phase is post-install, so it runs after the tool reaches its
// permanent directory without the recipe having to name a phase.
func reinstallPlanSteps(tool string) []executor.ResolvedStep {
	return []executor.ResolvedStep{
		{
			Action:    "run_command",
			Evaluable: true,
			Params: map[string]any{
				"command": "mkdir -p {install_dir}/bin {install_dir}/share && " +
					"printf '#!/bin/sh\\necho ok\\n' > {install_dir}/bin/" + tool + " && " +
					"chmod 0755 {install_dir}/bin/" + tool + " && " +
					"printf '" + reinstallGoodContent + "' > {install_dir}/share/marker.txt",
			},
		},
		{
			Action:    "set_env",
			Evaluable: true,
			Params: map[string]any{
				"vars": []any{
					map[string]any{"name": reinstallEnvVar, "value": "{install_dir}"},
				},
			},
		},
	}
}

// newReinstallHarness sets up $TSUKU_HOME, registers the recipe, and puts the
// tool on disk and in state exactly as a completed install would have left it --
// including the cached plan that installWithDependencies will re-execute.
func newReinstallHarness(t *testing.T) *reinstallHarness {
	t.Helper()

	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("config.DefaultConfig() error = %v", err)
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// The install path threads globalCtx into plan execution; main sets it and
	// tests have to.
	origCtx := globalCtx
	t.Cleanup(func() { globalCtx = origCtx })
	globalCtx = lifecycleCtx()

	// Swap the package-level loader so the synthetic recipes resolve without
	// consulting the registry.
	origLoader := loader
	t.Cleanup(func() { loader = origLoader })
	loader = recipe.NewLoader()

	h := &reinstallHarness{t: t, home: home, cfg: cfg, mgr: install.New(cfg)}
	h.seedInstalledTool(reinstallTool, nil, nil)
	return h
}

// seedInstalledTool registers a recipe and leaves the tool on disk and in state
// exactly as a completed install would have -- including the cached plan that
// getOrGeneratePlan prefers, which is what lets the reinstall re-execute
// offline.
//
// The recipe has no [version] section, so version resolution fails and falls
// back to "dev". That keeps the resolved version deterministic, which is what
// makes the cached-plan lookup hit.
func (h *reinstallHarness) seedInstalledTool(tool string, deps, runtimeDeps []string) {
	h.t.Helper()

	loader.CacheRecipe(tool, &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:                tool,
			Binaries:            []string{"bin/" + tool},
			Dependencies:        deps,
			RuntimeDependencies: runtimeDeps,
		},
		Steps: []recipe.Step{
			{
				Action: "run_command",
				Params: map[string]any{"command": "true"},
			},
		},
		Verify: &recipe.VerifySection{Command: "true"},
	})

	// Marked, because a real completed install stores what GeneratePlan
	// produced and generation stamps the marker. An unmarked record is one the
	// plan cache treats as a miss, which would make the reinstall regenerate
	// instead of replaying.
	plan := &executor.InstallationPlan{
		FormatVersion:  executor.PlanFormatVersion,
		StorageVersion: install.PlanStorageVersion,
		Tool:           tool,
		Version:        reinstallVersion,
		Platform:       executor.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps:          reinstallPlanSteps(tool),
	}

	if err := h.mgr.GetState().UpdateTool(tool, func(ts *install.ToolState) {
		ts.Version = reinstallVersion
		ts.ActiveVersion = reinstallVersion
		ts.IsExplicit = true
		ts.Versions = map[string]install.VersionState{
			reinstallVersion: {
				Binaries: []string{"bin/" + tool},
				Plan:     executor.ToStoragePlan(plan),
			},
		}
	}); err != nil {
		h.t.Fatalf("seeding state for %s: %v", tool, err)
	}

	h.writeTreeFor(tool, reinstallGoodContent)
}

// writeInstalledTree lays down the tool directory with the given marker
// content, standing in for whatever the previous install left behind.
func (h *reinstallHarness) writeInstalledTree(markerContent string) {
	h.t.Helper()
	h.writeTreeFor(reinstallTool, markerContent)
}

func (h *reinstallHarness) writeTreeFor(tool, markerContent string) {
	h.t.Helper()

	toolDir := h.cfg.ToolDir(tool, reinstallVersion)
	for _, sub := range []string{"bin", "share"} {
		if err := os.MkdirAll(filepath.Join(toolDir, sub), 0755); err != nil {
			h.t.Fatalf("creating %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin", tool), []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		h.t.Fatalf("writing binary: %v", err)
	}
	if err := os.WriteFile(h.markerPathFor(tool), []byte(markerContent), 0644); err != nil {
		h.t.Fatalf("writing marker: %v", err)
	}
}

// run drives the real install path, with reinstall on or off.
func (h *reinstallHarness) run(reinstall bool) error {
	h.t.Helper()
	return installWithDependencies(lifecycleCtx(), installArgs{
		Tool:       reinstallTool,
		IsExplicit: true,
		Reinstall:  reinstall,
		Reporter:   &countingReporter{},
	}, make(map[string]bool))
}

// markerContent reads back the file inside the tool directory.
func (h *reinstallHarness) markerContent() string {
	h.t.Helper()
	data, err := os.ReadFile(h.markerPath())
	if err != nil {
		h.t.Fatalf("reading marker: %v", err)
	}
	return string(data)
}

// recordedCleanup returns what state records for the installed version.
func (h *reinstallHarness) recordedCleanup() []install.CleanupAction {
	h.t.Helper()
	ts, err := h.mgr.GetToolState(reinstallTool)
	if err != nil || ts == nil {
		h.t.Fatalf("GetToolState() = %v, %v", ts, err)
	}
	return ts.Versions[reinstallVersion].CleanupActions
}

// sha256File hashes a file the same way the cleanup record does.
func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestReinstall_ReplacesTamperedFiles is the acceptance criterion at the level
// a user feels it. A file inside the install directory is wrong; the reinstall
// has to make it right, and the command has to still be there afterwards.
//
// The control case matters as much as the flag case: plain `tsuku install` on
// an already-installed version must stay idempotent, or every install of a
// satisfied dependency starts re-downloading.
func TestReinstall_ReplacesTamperedFiles(t *testing.T) {
	tests := []struct {
		name      string
		reinstall bool
		want      string
	}{
		{
			name:      "with --reinstall the modification is gone",
			reinstall: true,
			want:      reinstallGoodContent,
		},
		{
			name:      "without --reinstall the install stays idempotent",
			reinstall: false,
			want:      reinstallBadContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newReinstallHarness(t)
			h.writeInstalledTree(reinstallBadContent)

			if err := h.run(tt.reinstall); err != nil {
				t.Fatalf("install(reinstall=%v) error = %v", tt.reinstall, err)
			}

			if got := h.markerContent(); got != tt.want {
				t.Errorf("marker content = %q, want %q", got, tt.want)
			}

			// The binary has to survive either way. A reinstall that restored
			// the marker by removing the tool directory and stopping there
			// would pass the check above and leave the user without a command.
			info, err := os.Stat(h.binaryPath())
			if err != nil {
				t.Fatalf("binary missing after install: %v", err)
			}
			if info.Mode().Perm()&0111 == 0 {
				t.Errorf("binary mode = %v, want executable", info.Mode().Perm())
			}
		})
	}
}

// TestReinstall_RestoresMissingFile covers the other half of what `tsuku verify`
// reports: not a modified file but a deleted one.
func TestReinstall_RestoresMissingFile(t *testing.T) {
	h := newReinstallHarness(t)
	if err := os.Remove(h.markerPath()); err != nil {
		t.Fatalf("removing marker: %v", err)
	}

	if err := h.run(true); err != nil {
		t.Fatalf("install(reinstall=true) error = %v", err)
	}

	if got := h.markerContent(); got != reinstallGoodContent {
		t.Errorf("marker content = %q, want %q", got, reinstallGoodContent)
	}
}

// TestReinstall_ShellFragmentMatchesFreshHash covers the acceptance criterion
// about files written outside the tool directory. The seeded state describes a
// fragment whose recorded hash no longer matches the file on disk -- the shape
// of the shell.d repair the issue names. After the reinstall the file and the
// record have to agree, and there has to be exactly one fragment per shell
// rather than the old one plus a new one.
func TestReinstall_ShellFragmentMatchesFreshHash(t *testing.T) {
	h := newReinstallHarness(t)

	// A fragment the previous install left behind, with a stale recorded hash.
	stalePath := filepath.Join(h.home, fragmentPath("bash"))
	if err := os.MkdirAll(filepath.Dir(stalePath), 0755); err != nil {
		t.Fatalf("creating shell.d: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("export "+reinstallEnvVar+"=wrong\n"), 0644); err != nil {
		t.Fatalf("writing stale fragment: %v", err)
	}
	if err := h.mgr.GetState().UpdateTool(reinstallTool, func(ts *install.ToolState) {
		vs := ts.Versions[reinstallVersion]
		vs.CleanupActions = []install.CleanupAction{
			{Action: "delete_file", Path: fragmentPath("bash"), ContentHash: "stalehash"},
		}
		ts.Versions[reinstallVersion] = vs
	}); err != nil {
		t.Fatalf("seeding stale cleanup: %v", err)
	}

	if err := h.run(true); err != nil {
		t.Fatalf("install(reinstall=true) error = %v", err)
	}

	recorded := h.recordedCleanup()

	// The record has to name exactly the fragments this install writes -- one
	// per shell set_env covers. Anything more means the stale entry was kept
	// alongside the fresh one.
	var gotPaths []string
	for _, ca := range recorded {
		gotPaths = append(gotPaths, ca.Path)
	}
	slices.Sort(gotPaths)
	wantPaths := []string{fragmentPath("bash"), fragmentPath("zsh")}
	slices.Sort(wantPaths)
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("recorded cleanup paths = %v, want %v", gotPaths, wantPaths)
	}

	for _, ca := range recorded {
		abs := filepath.Join(h.home, ca.Path)
		if got := sha256File(t, abs); got != ca.ContentHash {
			t.Errorf("%s: file hash %s, recorded %s", ca.Path, got, ca.ContentHash)
		}
	}

	// The stale hash must be gone from the record, not sitting alongside the
	// fresh one.
	for _, ca := range recorded {
		if ca.ContentHash == "stalehash" {
			t.Errorf("stale cleanup record survived the reinstall: %+v", ca)
		}
	}

	// One fragment per shell, not one per install.
	entries, err := os.ReadDir(filepath.Join(h.home, "share", "shell.d"))
	if err != nil {
		t.Fatalf("reading shell.d: %v", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) != "" && e.Name()[0] != '.' {
			count++
		}
	}
	if count != len(recorded) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("shell.d holds %d fragment(s) %v but state records %d", count, names, len(recorded))
	}
}

// TestReinstall_DeletesOrphanedFragment covers the case where the recipe has
// stopped writing a file the previous install wrote. Nothing else would ever
// delete it: removal only consults what state records for the version, and the
// reinstall replaces that record wholesale. Without the stale-cleanup pass the
// file stays on disk forever, still concatenated into the user's shell by the
// init cache.
func TestReinstall_DeletesOrphanedFragment(t *testing.T) {
	h := newReinstallHarness(t)

	orphanRel := filepath.Join("share", "shell.d", "legacy-"+reinstallTool+"@"+reinstallVersion+".bash")
	orphanAbs := filepath.Join(h.home, orphanRel)
	if err := os.MkdirAll(filepath.Dir(orphanAbs), 0755); err != nil {
		t.Fatalf("creating shell.d: %v", err)
	}
	if err := os.WriteFile(orphanAbs, []byte("export "+reinstallEnvVar+"=legacy\n"), 0644); err != nil {
		t.Fatalf("writing orphan fragment: %v", err)
	}
	if err := h.mgr.GetState().UpdateTool(reinstallTool, func(ts *install.ToolState) {
		vs := ts.Versions[reinstallVersion]
		vs.CleanupActions = []install.CleanupAction{
			{Action: "delete_file", Path: orphanRel},
		}
		ts.Versions[reinstallVersion] = vs
	}); err != nil {
		t.Fatalf("seeding orphan cleanup: %v", err)
	}

	if err := h.run(true); err != nil {
		t.Fatalf("install(reinstall=true) error = %v", err)
	}

	if _, err := os.Stat(orphanAbs); !os.IsNotExist(err) {
		t.Errorf("orphaned fragment %s survived the reinstall (stat err = %v)", orphanRel, err)
	}

	// The fragments this install does write must still be there -- a
	// stale-cleanup pass that deleted everything the previous install recorded
	// would also pass the check above.
	for _, ca := range h.recordedCleanup() {
		if _, err := os.Stat(filepath.Join(h.home, ca.Path)); err != nil {
			t.Errorf("fragment %s recorded but missing: %v", ca.Path, err)
		}
	}
}

// TestReinstall_DoesNotCascadeToDependencies pins the scoping decision:
// --reinstall repairs the tool the user named and leaves its dependencies
// alone. Both dependency kinds are covered because they take different routes.
// An install-time dependency is installed with IsExplicit false and returns at
// the already-installed check near the top of installWithDependencies; a
// runtime dependency is installed with IsExplicit true and runs all the way to
// the version short-circuit, which is where a cascaded flag would push it into
// a full re-execution.
func TestReinstall_DoesNotCascadeToDependencies(t *testing.T) {
	tests := []struct {
		name    string
		runtime bool
	}{
		{name: "install-time dependency", runtime: false},
		{name: "runtime dependency", runtime: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const dep = "reinstalldep"

			h := newReinstallHarness(t)
			h.seedInstalledTool(dep, nil, nil)

			// Re-register the parent recipe with the dependency declared.
			if tt.runtime {
				h.seedInstalledTool(reinstallTool, nil, []string{dep})
			} else {
				h.seedInstalledTool(reinstallTool, []string{dep}, nil)
			}

			// Both trees are wrong. Only the named tool's may be repaired.
			h.writeTreeFor(reinstallTool, reinstallBadContent)
			h.writeTreeFor(dep, reinstallBadContent)

			if err := h.run(true); err != nil {
				t.Fatalf("install(reinstall=true) error = %v", err)
			}

			if got := h.markerContent(); got != reinstallGoodContent {
				t.Errorf("named tool marker = %q, want %q (the reinstall must repair it)", got, reinstallGoodContent)
			}

			depMarker, err := os.ReadFile(h.markerPathFor(dep))
			if err != nil {
				t.Fatalf("reading dependency marker: %v", err)
			}
			if string(depMarker) != reinstallBadContent {
				t.Errorf("dependency marker = %q, want %q (reinstall must not cascade)", depMarker, reinstallBadContent)
			}
		})
	}
}

// TestReinstall_RepairsHiddenTool covers the other shortcut on the way in.
// A hidden tool -- one installed as somebody's execution dependency -- is
// normally handled by exposing it, which only creates symlinks and never looks
// at the payload. Someone repairing a hidden tool's files that way would get
// links to the same broken files and a success message.
func TestReinstall_RepairsHiddenTool(t *testing.T) {
	h := newReinstallHarness(t)
	if err := h.mgr.GetState().UpdateTool(reinstallTool, func(ts *install.ToolState) {
		ts.IsHidden = true
		ts.IsExecutionDependency = true
		ts.Binaries = []string{"bin/" + reinstallTool}
	}); err != nil {
		t.Fatalf("marking tool hidden: %v", err)
	}
	h.writeInstalledTree(reinstallBadContent)

	if err := h.run(true); err != nil {
		t.Fatalf("install(reinstall=true) error = %v", err)
	}

	if got := h.markerContent(); got != reinstallGoodContent {
		t.Errorf("marker content = %q, want %q (exposing a hidden tool does not repair it)", got, reinstallGoodContent)
	}
}

// TestReinstallLibrary_ReplacesTamperedFiles covers the second message
// `tsuku verify` prints. Libraries take their own install path with their own
// two reuse checks -- one before the plan runs and one after -- and a flag that
// existed but hit either of them would answer an integrity failure with
// "reusing" and change nothing on disk.
func TestReinstallLibrary_ReplacesTamperedFiles(t *testing.T) {
	tests := []struct {
		name      string
		reinstall bool
		want      string
	}{
		{
			name:      "with --reinstall the modification is gone",
			reinstall: true,
			want:      reinstallGoodContent,
		},
		{
			name:      "without --reinstall the existing install is reused",
			reinstall: false,
			want:      reinstallBadContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const lib = "reinstalllib"

			home := t.TempDir()
			t.Setenv("TSUKU_HOME", home)

			cfg, err := config.DefaultConfig()
			if err != nil {
				t.Fatalf("config.DefaultConfig() error = %v", err)
			}
			if err := cfg.EnsureDirectories(); err != nil {
				t.Fatalf("EnsureDirectories() error = %v", err)
			}

			origCtx := globalCtx
			t.Cleanup(func() { globalCtx = origCtx })
			globalCtx = lifecycleCtx()

			origLoader := loader
			t.Cleanup(func() { loader = origLoader })
			loader = recipe.NewLoader()
			loader.CacheRecipe(lib, &recipe.Recipe{
				Metadata: recipe.MetadataSection{Name: lib, Type: "library"},
				Steps: []recipe.Step{
					{Action: "run_command", Params: map[string]any{
						"command": "mkdir -p {install_dir}/lib && " +
							"printf '" + reinstallGoodContent + "' > {install_dir}/lib/marker.txt",
					}},
				},
			})

			mgr := install.New(cfg)
			run := func(reinstall bool) error {
				return installWithDependencies(lifecycleCtx(), installArgs{
					Tool:       lib,
					IsExplicit: true,
					Reinstall:  reinstall,
					Reporter:   &countingReporter{},
				}, make(map[string]bool))
			}

			if err := run(false); err != nil {
				t.Fatalf("first install error = %v", err)
			}

			version := mgr.GetInstalledLibraryVersion(lib)
			if version == "" {
				t.Fatalf("library not installed after first install")
			}
			marker := filepath.Join(mgr.LibDir(lib, version), "lib", "marker.txt")
			if err := os.WriteFile(marker, []byte(reinstallBadContent), 0644); err != nil {
				t.Fatalf("tampering with library marker: %v", err)
			}

			if err := run(tt.reinstall); err != nil {
				t.Fatalf("second install(reinstall=%v) error = %v", tt.reinstall, err)
			}

			got, err := os.ReadFile(marker)
			if err != nil {
				t.Fatalf("reading library marker: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("library marker = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInstallAdviceNamesRegisteredFlags is the guard for the defect that
// produced this issue: `tsuku verify` told users to run
// `tsuku install <tool> --reinstall` for two releases while no such flag was
// registered, so the advice failed with an unknown-flag error at exactly the
// moment the user needed it.
//
// The check reads every Go source in this package, finds each line that offers
// a `tsuku install` command with flags on it, and requires the install command
// to actually have them. It deliberately covers the whole package rather than
// the two known messages, because the next stale suggestion will be somewhere
// else.
func TestInstallAdviceNamesRegisteredFlags(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}

	// A flag on a line that also offers "tsuku install". The trailing class
	// stops at the first character that cannot be part of a flag name, so
	// "--fresh\n" and "--reinstall'" both yield the bare name.
	flagPattern := regexp.MustCompile(`--([a-z][a-z0-9-]*)`)

	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			idx := strings.Index(line, "tsuku install ")
			if idx < 0 {
				continue
			}
			for _, m := range flagPattern.FindAllStringSubmatch(line[idx:], -1) {
				checked++
				if installCmd.Flags().Lookup(m[1]) == nil {
					t.Errorf("%s:%d suggests 'tsuku install ... --%s', which is not a registered install flag:\n  %s",
						name, i+1, m[1], strings.TrimSpace(line))
				}
			}
		}
	}

	// A pattern that matched nothing would make this test pass forever without
	// looking at anything.
	if checked == 0 {
		t.Fatal("found no 'tsuku install' suggestions with flags; the scan is not looking at anything")
	}
}

// TestWithInstallFlags_CarriesReinstall pins the wiring between the parsed flag
// and the install pipeline. Registering --reinstall and never reading it would
// leave the two `tsuku verify` messages pointing at a flag that parses and does
// nothing, which is a worse outcome than the unknown-flag error they used to
// produce.
func TestWithInstallFlags_CarriesReinstall(t *testing.T) {
	orig := installReinstall
	t.Cleanup(func() { installReinstall = orig })

	for _, want := range []bool{true, false} {
		installReinstall = want
		if got := withInstallFlags(installArgs{Tool: reinstallTool}).Reinstall; got != want {
			t.Errorf("withInstallFlags() Reinstall = %v, want %v", got, want)
		}
	}

	// Fields the caller set have to survive.
	installReinstall = true
	got := withInstallFlags(installArgs{Tool: reinstallTool, ReqVersion: "1.2.3", IsExplicit: true})
	if got.Tool != reinstallTool || got.ReqVersion != "1.2.3" || !got.IsExplicit {
		t.Errorf("withInstallFlags() clobbered caller fields: %+v", got)
	}
}

// TestReinstall_FromCommandEntryPoint runs the whole chain the CLI runs: the
// package-level flag cobra writes into, the entry point that turns it into
// install arguments, and the install itself. The other tests call
// installWithDependencies directly and would all still pass if the entry point
// stopped applying the flags.
func TestReinstall_FromCommandEntryPoint(t *testing.T) {
	h := newReinstallHarness(t)
	h.writeInstalledTree(reinstallBadContent)

	orig := installReinstall
	t.Cleanup(func() { installReinstall = orig })
	installReinstall = true

	if err := runInstall(lifecycleCtx(), installArgs{Tool: reinstallTool, IsExplicit: true}); err != nil {
		t.Fatalf("runInstall() error = %v", err)
	}

	if got := h.markerContent(); got != reinstallGoodContent {
		t.Errorf("marker content = %q, want %q", got, reinstallGoodContent)
	}
}

// TestReinstall_CachedPlanVersusFresh pins which kind of fix --reinstall picks
// up on its own.
//
// getOrGeneratePlan prefers the plan stored in state.json, and only --fresh
// bypasses it. ValidateCachedPlan checks the format version and the platform;
// CacheKeyFor never populates ContentHash, so a recipe that has changed since
// the plan was stored does not invalidate it.
//
// So the two cases differ. A fix in tsuku's own code reaches the user through a
// plain --reinstall, because the stored steps are re-executed by the new
// binary. A fix in the recipe does not: the stored plan still describes the old
// steps, and regenerating it takes --fresh as well.
func TestReinstall_CachedPlanVersusFresh(t *testing.T) {
	// The harness's stored plan writes reinstallGoodContent; the recipe
	// registered below writes something else, so which plan ran is visible in
	// the file.
	const recipeContent = "written-by-the-current-recipe\n"

	tests := []struct {
		name  string
		fresh bool
		want  string
	}{
		{
			name:  "--reinstall alone replays the stored plan",
			fresh: false,
			want:  reinstallGoodContent,
		},
		{
			name:  "--fresh --reinstall regenerates from the recipe",
			fresh: true,
			want:  recipeContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newReinstallHarness(t)

			// Re-register the recipe so a freshly generated plan writes
			// different content than the stored plan does. This stands in for a
			// recipe that has been fixed since the tool was installed.
			loader.CacheRecipe(reinstallTool, &recipe.Recipe{
				Metadata: recipe.MetadataSection{
					Name:     reinstallTool,
					Binaries: []string{"bin/" + reinstallTool},
				},
				Steps: []recipe.Step{
					{
						Action: "run_command",
						Params: map[string]any{
							"command": "mkdir -p {install_dir}/bin {install_dir}/share && " +
								"printf '#!/bin/sh\\necho ok\\n' > {install_dir}/bin/" + reinstallTool + " && " +
								"chmod 0755 {install_dir}/bin/" + reinstallTool + " && " +
								"printf '" + recipeContent + "' > {install_dir}/share/marker.txt",
						},
					},
				},
				Verify: &recipe.VerifySection{Command: "true"},
			})

			origFresh := installFresh
			t.Cleanup(func() { installFresh = origFresh })
			installFresh = tt.fresh

			if err := h.run(true); err != nil {
				t.Fatalf("install(reinstall=true, fresh=%v) error = %v", tt.fresh, err)
			}

			if got := h.markerContent(); got != tt.want {
				t.Errorf("marker content = %q, want %q", got, tt.want)
			}
		})
	}
}
