package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/term"

	"github.com/tsukumogami/tsuku/internal/activation"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/project"
)

// Every case here runs against a project that declares two recipes providing
// one command, in one of the two shapes that produces. R8 keeps such a
// configuration valid, so `tsuku install` and activation have to go on working
// over it; only dispatch of the bare command has no answer, and only dispatch
// refuses.

// bothProviders declares each provider of the fixture's two-provider command
// under its own name, at the version both install at. Two recipes, two names.
func bothProviders() map[string]string {
	return map[string]string{
		indexfixture.RecipeDupFirst: indexfixture.SharedVersion,
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	}
}

// twoRegistries is AC11a's configuration: two org-scoped keys whose org
// components differ and whose bare names agree, with no bare key. It is the
// harder shape, because both declarations carry the same recipe name and only
// the keys tell them apart.
func twoRegistries() map[string]string {
	return map[string]string{
		"org-a/" + indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
		"org-b/" + indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	}
}

// twoProviderProject builds a project declaring tools and points the whole
// `tsuku run` path at the fixture: the loader the install pipeline reads, the
// context it threads, and the lookup the run wiring opens the index through.
//
// It also moves the working directory into the project, because both commands
// driven below discover their configuration by walking up from cwd.
func twoProviderProject(t *testing.T, tools map[string]string) (*indexfixture.Fixture, string) {
	t.Helper()

	fx := indexfixture.New(t)

	origLoader := loader
	t.Cleanup(func() { loader = origLoader })
	loader = fx.Loader()

	origCtx := globalCtx
	t.Cleanup(func() { globalCtx = origCtx })
	globalCtx = lifecycleCtx()

	origLookup := binaryCommandLookup
	t.Cleanup(func() { binaryCommandLookup = origLookup })
	binaryCommandLookup = func(ctx context.Context, _ *config.Config, command string) ([]index.BinaryMatch, error) {
		return fx.Lookup(ctx, command)
	}

	// Neither belongs to what these cases assert. The update check spawns a
	// detached process; telemetry prints a first-run notice to stderr, which
	// is a stream the terminal case compares byte for byte, and its client
	// POSTs events to an external endpoint.
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "1")
	t.Setenv("TSUKU_NO_TELEMETRY", "1")

	dir := t.TempDir()
	t.Setenv(project.EnvCeilingPaths, filepath.Dir(dir))
	fx.WriteProjectConfig(t, dir, tools)
	chdir(t, dir)

	return fx, dir
}

// runTsukuRun drives `tsuku run <command>` with stdin bound to f, which is
// what decides whether a terminal is attached.
func runTsukuRun(t *testing.T, f *os.File, command string) commandOutcome {
	t.Helper()

	orig := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = orig }()

	return runCommandOutcome(t, func() error {
		runCmd.Run(runCmd, []string{command})
		return nil
	})
}

// pipeStdin is a stdin that is definitely not a terminal.
func pipeStdin(t *testing.T) *os.File {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if term.IsTerminal(int(r.Fd())) {
		t.Fatal("a pipe reports itself as a terminal; the halves of this case cannot be told apart")
	}
	return r
}

// terminalStdin is a stdin that is definitely a terminal: the master side of a
// pseudo-terminal, which answers the same ioctl a login shell's does.
//
// Nothing here reads from it, so no slave side is opened. It reports false
// rather than failing where the platform has no /dev/ptmx to open, since a
// test binary cannot conjure a terminal for itself and the half of the case
// that runs without one is still worth running.
func terminalStdin(t *testing.T) (*os.File, bool) {
	t.Helper()

	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, false
	}
	t.Cleanup(func() { _ = f.Close() })
	if !term.IsTerminal(int(f.Fd())) {
		return nil, false
	}
	return f, true
}

// AC11b. AC11a's configuration -- two org-scoped keys, one bare recipe name --
// refuses at `tsuku run` rather than resolving, and the refusal names both
// keys.
//
// AC11a is satisfied the moment the declaration set is right, and was, by the
// unit before this one. This is the half that could not be: between the two,
// the set was correct and unused, and only a run that refuses can tell.
func TestRunCmd_AC11b_TwoRegistriesForOneNameRefuse(t *testing.T) {
	twoProviderProject(t, twoRegistries())

	refusal := runTsukuRun(t, pipeStdin(t), indexfixture.CommandTwoProviders)

	if !refusal.exited || refusal.exit != ExitAmbiguous {
		t.Fatalf("exit = %d (exited = %v), want %d\nstderr:\n%s",
			refusal.exit, refusal.exited, ExitAmbiguous, refusal.stderr)
	}
	for key := range twoRegistries() {
		if !strings.Contains(refusal.stderr, key) {
			t.Errorf("the refusal does not name the configuration key %q:\n%s", key, refusal.stderr)
		}
	}
}

// AC15. The refusal is the same thing twice: identical output, exit 10, with
// and without a terminal.
//
// The exit code is the install path's ExitAmbiguous, which is the same
// condition -- a name that narrows to more than one recipe -- arriving by
// another route.
//
// Both runs go through runCmd rather than through the runner directly. What
// AC15 is about lives in the command: the terminal is consulted there, and so
// is the exit code.
func TestRunCmd_AC15_TheRefusalIsTheSameWithAndWithoutATerminal(t *testing.T) {
	twoProviderProject(t, bothProviders())

	withoutTerminal := runTsukuRun(t, pipeStdin(t), indexfixture.CommandTwoProviders)

	if !withoutTerminal.exited || withoutTerminal.exit != ExitAmbiguous {
		t.Fatalf("exit = %d (exited = %v), want %d\nstderr:\n%s",
			withoutTerminal.exit, withoutTerminal.exited, ExitAmbiguous, withoutTerminal.stderr)
	}
	for recipe := range bothProviders() {
		if !strings.Contains(withoutTerminal.stderr, recipe) {
			t.Errorf("the refusal does not name the declared recipe %q:\n%s",
				recipe, withoutTerminal.stderr)
		}
	}
	if withoutTerminal.stdout != "" {
		t.Errorf("stdout = %q, want nothing: there is no picker to render and no prompt to answer",
			withoutTerminal.stdout)
	}

	terminal, ok := terminalStdin(t)
	if !ok {
		t.Skip("no terminal device could be opened here; the with-a-terminal half cannot run")
	}
	withTerminal := runTsukuRun(t, terminal, indexfixture.CommandTwoProviders)

	if withTerminal.exit != withoutTerminal.exit {
		t.Errorf("exit = %d with a terminal and %d without", withTerminal.exit, withoutTerminal.exit)
	}
	if withTerminal.stderr != withoutTerminal.stderr {
		t.Errorf("the refusal differs with a terminal attached.\nwith:\n%s\nwithout:\n%s",
			withTerminal.stderr, withoutTerminal.stderr)
	}
	if withTerminal.stdout != withoutTerminal.stdout {
		t.Errorf("stdout differs with a terminal attached: %q against %q",
			withTerminal.stdout, withoutTerminal.stdout)
	}
}

// AC26. The invocation the refusal prints, followed as printed, runs one of
// the declared recipes to completion.
//
// The commands are read back out of the message rather than rebuilt from the
// fixture's constants. A test that rebuilt them would pass against a refusal
// that printed something else entirely, which is the whole of what this
// criterion is about.
func TestRunCmd_AC26_TheInvocationInTheRefusalRunsADeclaredRecipe(t *testing.T) {
	twoProviderProject(t, bothProviders())

	refusal := runTsukuRun(t, pipeStdin(t), indexfixture.CommandTwoProviders)
	if refusal.exit != ExitAmbiguous {
		t.Fatalf("exit = %d, want %d\nstderr:\n%s", refusal.exit, ExitAmbiguous, refusal.stderr)
	}

	argument, binary := firstInvocation(t, refusal.stderr)

	installed := runCommandOutcome(t, func() error {
		installCmd.Run(installCmd, []string{argument})
		return nil
	})
	if installed.exited && installed.exit != ExitSuccess {
		t.Fatalf("`tsuku install %s` exited %d\nstdout:\n%s\nstderr:\n%s",
			argument, installed.exit, installed.stdout, installed.stderr)
	}

	out, err := exec.Command(binary).Output() //nolint:gosec // the path came from the fixture's own temp home
	if err != nil {
		t.Fatalf("running %s, the second line of the refusal: %v", binary, err)
	}
	// The fixture binaries print the path they were invoked as, which is the
	// only way "this is the declared recipe's copy and not its sibling's" is
	// observable at all.
	if got := strings.TrimSpace(string(out)); got != binary {
		t.Errorf("%s printed %q, want its own path", binary, got)
	}
}

// firstInvocation reads the first install-and-run pair out of a refusal: the
// argument `tsuku install` was given, and the path printed under it.
func firstInvocation(t *testing.T, refusal string) (argument, binary string) {
	t.Helper()

	lines := strings.Split(refusal, "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "tsuku install ")
		if !ok {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatalf("the refusal ends after its install line:\n%s", refusal)
		}
		return rest, strings.TrimSpace(lines[i+1])
	}

	t.Fatalf("the refusal names no `tsuku install` invocation:\n%s", refusal)
	return "", ""
}

// AC16. The same configuration the run path refuses is one `tsuku install`
// and activation both have an unambiguous job to do with, and R8 keeps it
// valid for them. Two providers of one command is a reason to refuse a
// dispatch, not a reason to reject a file.
func TestInstallCmd_AC16_TheTwoProviderConfigurationInstallsAndActivates(t *testing.T) {
	fx, dir := twoProviderProject(t, bothProviders())

	origYes := installYes
	t.Cleanup(func() { installYes = origYes })
	installYes = true

	installed := runCommandOutcome(t, func() error {
		installCmd.Run(installCmd, nil)
		return nil
	})
	if installed.exit != ExitSuccess {
		t.Fatalf("`tsuku install` over the project configuration exited %d\nstdout:\n%s\nstderr:\n%s",
			installed.exit, installed.stdout, installed.stderr)
	}

	manager := install.New(fx.Cfg)
	for recipe := range bothProviders() {
		state, err := manager.GetToolState(recipe)
		if err != nil || state == nil {
			t.Fatalf("GetToolState(%s) = %v, %v; the project install was meant to install both",
				recipe, state, err)
		}
		if state.ActiveVersion != indexfixture.SharedVersion {
			t.Errorf("%s active version = %q, want %q", recipe, state.ActiveVersion, indexfixture.SharedVersion)
		}
	}

	result, err := activation.ComputeActivation(dir, "", "", "", fx.Cfg, install.NewStateManager(fx.Cfg))
	if err != nil {
		t.Fatalf("activation over the same configuration: %v", err)
	}
	if result == nil {
		t.Fatal("activation produced nothing for a project directory declaring two tools")
	}
	if result.Unreadable != nil {
		t.Fatalf("activation could not read installation state: %+v", result.Unreadable)
	}
	for _, u := range result.Unhonorable {
		t.Errorf("activation did not honor %q (declared %q): %v", u.Tool, u.Declared, u.Reason)
	}
	for recipe := range bothProviders() {
		binDir := fx.Cfg.ToolBinDir(recipe, indexfixture.SharedVersion)
		if !strings.Contains(result.PATH, binDir) {
			t.Errorf("activation left %q off PATH:\n%s", binDir, result.PATH)
		}
	}
}
