package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/term"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// The exit codes the moved terminal check produces. What the check decides is
// pinned next to the decision, in internal/autoinstall/interactive_test.go;
// these cases are here because an exit code exists only at this layer, and
// because the predicate that was wrong lived here.

// noProject is twoProviderProject's wiring over a directory with no
// .tsuku.toml in it or anywhere above it.
func noProject(t *testing.T) *indexfixture.Fixture {
	t.Helper()
	fx, _ := twoProviderProject(t, nil)
	return fx
}

// closedStdin is a stdin that is not a terminal and is already at end of file.
//
// pipeStdin's write end stays open until the test ends, so a run that reached
// a prompt would block on it for as long as the package is allowed to run. A
// regression in the terminal check is exactly a run that reaches the prompt,
// and a hung package is a worse way to hear about it than a failed assertion:
// at end of file the prompt returns at once and the case fails on the exit
// code, naming the outcome it got instead.
func closedStdin(t *testing.T) *os.File {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	t.Cleanup(func() { _ = r.Close() })
	if term.IsTerminal(int(r.Fd())) {
		t.Fatal("a pipe reports itself as a terminal; this case cannot tell its two halves apart")
	}
	return r
}

// openPermissionsConfig writes a config file the configuration-permission gate
// refuses. It is empty, so it changes no setting; only its mode matters.
func openPermissionsConfig(t *testing.T, homeDir string) {
	t.Helper()

	path := filepath.Join(homeDir, "config.toml")
	if err := os.WriteFile(path, nil, 0o644); err != nil { //nolint:gosec // the permissive mode is the fixture
		t.Fatalf("writing %s: %v", path, err)
	}
}

// AC20. A project declaring a tool, a command it does not declare, no
// terminal: the not-interactive code and the message, not a prompt written
// into a closed stdin.
//
// This case failed before the check moved. The old predicate skipped the check
// whenever the configuration declared anything at all, so this run reached the
// prompt, read end-of-file from the pipe, and exited 13.
func TestRunCmd_AC20_UndeclaredCommandWithNoTerminalIsNotInteractive(t *testing.T) {
	twoProviderProject(t, map[string]string{indexfixture.DeclaredRecipe: indexfixture.SharedVersion})

	got := runTsukuRun(t, closedStdin(t), indexfixture.CommandOneProvider)

	if !got.exited || got.exit != ExitNotInteractive {
		t.Fatalf("exit = %d (exited = %v), want %d\nstdout:\n%s\nstderr:\n%s",
			got.exit, got.exited, ExitNotInteractive, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "requires a terminal") {
		t.Errorf("stderr does not say why the run stopped:\n%s", got.stderr)
	}
	if strings.Contains(got.stdout, "[y/N]") {
		t.Errorf("a prompt was written to a stdin nobody can answer: %q", got.stdout)
	}
}

// AC17a. A configuration naming a recipe no index knows consents exactly as no
// configuration does, with no terminal attached: same exit code, same output.
//
// This is the criterion the old predicate could not meet however correct the
// declaration set became. `len(Tools) > 0` was true for this file, so the
// unknown-recipe project exited 13 at a prompt while the empty directory
// exited 12 without one -- two exit codes for two states that R5 says are the
// same state.
func TestRunCmd_AC17a_AnUnknownRecipeConfigConsentsLikeNoConfig(t *testing.T) {
	const unknownRecipe = "fixture-absent-from-every-index"

	var withConfig, noConfig commandOutcome
	t.Run("declaring a recipe no index knows", func(t *testing.T) {
		twoProviderProject(t, map[string]string{unknownRecipe: "1.0.0"})
		withConfig = runTsukuRun(t, closedStdin(t), indexfixture.CommandOneProvider)
	})
	t.Run("no .tsuku.toml at all", func(t *testing.T) {
		noProject(t)
		noConfig = runTsukuRun(t, closedStdin(t), indexfixture.CommandOneProvider)
	})

	if withConfig.exit != noConfig.exit {
		t.Errorf("exit = %d with the unknown-recipe config and %d with no config", withConfig.exit, noConfig.exit)
	}
	if withConfig.stderr != noConfig.stderr {
		t.Errorf("stderr differs.\nwith config:\n%s\nno config:\n%s", withConfig.stderr, noConfig.stderr)
	}
	if withConfig.stdout != noConfig.stdout {
		t.Errorf("stdout differs: %q against %q", withConfig.stdout, noConfig.stdout)
	}
	if withConfig.exit != ExitNotInteractive {
		t.Errorf("both runs exited %d, want %d: they agree, but on the wrong outcome",
			withConfig.exit, ExitNotInteractive)
	}
}

// The case that catches a declaredness term left in the moved predicate: a
// declared command raised to auto, lowered back to confirm by the
// configuration-permission gate, with no terminal.
//
// A predicate still asking whether the command is declared would skip the
// check here and prompt, exiting 13 at end-of-file. AC20 cannot catch that --
// its command is undeclared, so the check fires under either predicate.
func TestRunCmd_ADeclaredCommandLoweredByAGateIsNotInteractive(t *testing.T) {
	fx, _ := twoProviderProject(t, map[string]string{indexfixture.RecipeSolo: indexfixture.SharedVersion})
	openPermissionsConfig(t, fx.Cfg.HomeDir)

	got := runTsukuRun(t, closedStdin(t), indexfixture.CommandOneProvider)

	if !got.exited || got.exit != ExitNotInteractive {
		t.Fatalf("exit = %d (exited = %v), want %d\nstdout:\n%s\nstderr:\n%s",
			got.exit, got.exited, ExitNotInteractive, got.stdout, got.stderr)
	}
	// Without this the case could pass for the wrong reason: a run that never
	// reached auto never exercised the elevation the criterion is about.
	if !strings.Contains(got.stderr, "config-permissions") {
		t.Errorf("the configuration-permission gate did not fire, so this run never reached confirm the way it meant to:\n%s",
			got.stderr)
	}
	if strings.Contains(got.stdout, "[y/N]") {
		t.Errorf("the declared command was prompted about with no terminal attached: %q", got.stdout)
	}
}

// AC55. A command no recipe provides is reported on stderr before the exit.
//
// Before the move this state exited 12 with a line explaining itself, because
// the terminal check ran above the lookup. Below it, the lookup answers first,
// and without this line the run would end at a bare exit 1 saying nothing --
// a worse outcome than the one being replaced.
func TestRunCmd_AC55_NoProviderIsReportedBeforeExiting(t *testing.T) {
	noProject(t)

	const absent = "fixture-command-no-recipe-provides"
	got := runTsukuRun(t, closedStdin(t), absent)

	if !got.exited || got.exit != ExitGeneral {
		t.Fatalf("exit = %d (exited = %v), want %d\nstderr:\n%s", got.exit, got.exited, ExitGeneral, got.stderr)
	}
	if !strings.Contains(got.stderr, absent) {
		t.Errorf("stderr does not name the command nothing provides:\n%s", got.stderr)
	}
	if !strings.Contains(got.stderr, "No recipe provides") {
		t.Errorf("stderr does not report that no recipe provides it:\n%s", got.stderr)
	}
}
