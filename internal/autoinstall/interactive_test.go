package autoinstall

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/project"
)

// The terminal check, at the point it decides. Everything here runs with no
// terminal, which is the state the check is about; the exit codes these
// outcomes map to are pinned in cmd/tsuku/run_terminal_check_test.go, because
// that is where exit codes live.

// newHeadlessRunner is newFixtureRunner with no terminal attached and a
// consent reader that is empty rather than absent.
//
// The empty reader is what makes a regression legible instead of silent: a
// check that stopped firing would fall through to the prompt, the prompt would
// read end-of-file, and the run would come back ErrUserDeclined with the
// question in stdout -- which is exactly the failure AC20 describes, rather
// than a hang or a read of the test process's own stdin.
func newHeadlessRunner(t *testing.T, fx *indexfixture.Fixture) (*Runner, *mockInstaller, *execRecorder, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)
	r.IsTerminal = func() bool { return false }
	r.ConsentReader = strings.NewReader("")
	return r, installer, execRec, stdout, stderr
}

// openPermissionsConfig writes the config file the configuration-permission
// gate refuses: readable by group and other, which is what a config someone
// else can write looks like from here. It is empty, so it sets nothing; only
// its mode is the point.
//
// This gate is the one these cases reach for because its precondition is
// filesystem state alone. It fires without the command needing any particular
// recipe property, so it can be combined with a declaration freely.
func openPermissionsConfig(t *testing.T, cfg *config.Config) {
	t.Helper()

	path := filepath.Join(cfg.HomeDir, "config.toml")
	if err := os.WriteFile(path, nil, 0o644); err != nil { //nolint:gosec // the permissive mode is the fixture
		t.Fatalf("writing %s: %v", path, err)
	}
}

// AC2. A declared provider of a two-provider command under auto, with no
// terminal: the run succeeds. It does not stop at the terminal check, and it
// does not stop at a prompt nobody could answer.
//
// The criterion says "rather than exiting 13", and 13 is what the prompt
// returns when its reader is empty -- so ErrUserDeclined here would be the
// failure named, arriving by the route named.
func TestRun_AC2_DeclaredAutoRunSucceedsWithNoTerminal(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, _ := newHeadlessRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want the declared recipe %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
	if !execRec.called {
		t.Error("exec was not called")
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt appeared with no terminal to answer it: %q", stdout.String())
	}
}

// AC17a. The consent half of AC17: with a configuration naming a recipe no
// index knows, every command is consented to as it would be with no
// .tsuku.toml at all -- including with no terminal, which is the case the old
// predicate got wrong and no correct declaration set could fix.
//
// AC10 compares the same two runs with a terminal attached. Without one, the
// old check skipped the terminal gate whenever the file declared anything,
// so this configuration exited 13 where an empty directory exited 12.
func TestRun_AC17a_UnknownRecipeConfigConsentsAsNoConfigDoes(t *testing.T) {
	const unknownRecipe = "fixture-absent-from-every-index"

	for _, command := range []string{indexfixture.CommandTwoProviders, indexfixture.CommandOneProvider} {
		for _, mode := range []Mode{ModeSuggest, ModeConfirm, ModeAuto} {
			t.Run(command+"/"+mode.String(), func(t *testing.T) {
				fx := indexfixture.New(t)
				withConfig := runOutcome(t, fx, command, mode,
					declaring(map[string]string{unknownRecipe: "1.0.0"}), withoutTerminal)
				noConfig := runOutcome(t, fx, command, mode, project.NewResolver(nil), withoutTerminal)

				if withConfig != noConfig {
					t.Errorf("with no terminal, a config declaring only %q changed the run.\nwith config: %+v\nno config:   %+v",
						unknownRecipe, withConfig, noConfig)
				}
			})
		}
	}
}

// AC20. A project declaring something, a command it does not declare, confirm
// mode, no terminal: the refusal, not a prompt written into a closed stdin.
func TestRun_AC20_UndeclaredCommandWithNoTerminalRefuses(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, stdout, stderr := newHeadlessRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil, ModeConfirm,
		declaring(map[string]string{indexfixture.DeclaredRecipe: indexfixture.SharedVersion}))

	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("Run() error = %v, want ErrNotInteractive", err)
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt was written with no terminal to answer it: %q", stdout.String())
	}
	if installer.called {
		t.Errorf("installed %q without consent", installer.recipe)
	}
	if !strings.Contains(stderr.String(), "requires a terminal") {
		t.Errorf("stderr does not say why the run stopped:\n%s", stderr.String())
	}
}

// The case that catches a declaredness term left in the predicate. A declared
// command is raised to auto, the configuration-permission gate lowers it back
// to confirm, and there is no terminal: the refusal, not a prompt.
//
// Nothing else here catches it. AC20's command is undeclared, so a check
// carrying "and the project declares nothing" fires either way; AC2 pins auto
// with no gate firing, so its check does not fire at all. Only a command that
// *is* declared and arrives at confirm anyway can tell the two predicates
// apart -- which is why the elevation and a mode-lowering gate both have to be
// in the state.
func TestRun_DeclaredCommandLoweredByAGateRefusesRatherThanPrompting(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, stdout, stderr := newHeadlessRunner(t, fx)
	openPermissionsConfig(t, fx.Cfg)

	err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil, ModeConfirm,
		declaring(map[string]string{indexfixture.RecipeSolo: indexfixture.SharedVersion}))

	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("Run() error = %v, want ErrNotInteractive.\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	if promptShown(stdout.String()) {
		t.Errorf("the declared command was prompted about with no terminal attached: %q", stdout.String())
	}
	if installer.called {
		t.Errorf("installed %q without consent", installer.recipe)
	}
	// The gate has to have fired for this case to be the case it claims to be.
	if !strings.Contains(stderr.String(), "permissions are too open") {
		t.Errorf("the configuration-permission gate did not fire, so this run never reached confirm the way it meant to:\n%s",
			stderr.String())
	}
}

// AC54. No declaration, no terminal, and the tool already installed: the run
// execs it. The terminal check is below the already-installed fast path now,
// so a command that was never going to prompt is no longer stopped as though
// it were.
//
// "Exits with the tool's own code" is syscall.Exec's doing and is not
// observable in process -- the recorder stands in for it. What this pins is
// the half that is this package's: the run reaches exec, and returns no error.
// ErrNotInteractive is the only route to exit 12 after this change, and it is
// not what came back.
func TestRun_AC54_InstalledToolExecsWithNoTerminal(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, _ := newHeadlessRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandInstalledFirst, nil, ModeConfirm, project.NewResolver(nil))
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !execRec.called {
		t.Fatal("exec was not called for a tool the index reports installed")
	}
	if want := filepath.Join(fx.Cfg.CurrentDir, indexfixture.CommandInstalledFirst); execRec.binary != want {
		t.Errorf("exec ran %q, want %q", execRec.binary, want)
	}
	if installer.called {
		t.Errorf("installed %q for a tool that was already installed", installer.recipe)
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt appeared for an installed tool: %q", stdout.String())
	}
}

// AC55. A command no recipe provides is reported before the run gives up.
// Moving the terminal check below the lookup is what made this necessary:
// before, this state exited 12 with a line explaining itself.
func TestRun_AC55_NoProviderIsReported(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, _, _, stderr := newHeadlessRunner(t, fx)

	const absent = "fixture-command-no-recipe-provides"
	err := r.Run(context.Background(), absent, nil, ModeConfirm, project.NewResolver(nil))

	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("Run() error = %v, want ErrNoMatch", err)
	}
	if !strings.Contains(stderr.String(), absent) {
		t.Errorf("the report does not name the command:\n%s", stderr.String())
	}
	if got := hatchesIn(stderr.String()); len(got) != 0 {
		t.Errorf("the report names %v; no invocation completes a command no recipe provides, so it must name none", got)
	}
}

// hatchVocabulary is every form an escape hatch takes in this package's
// output, plus the forms it does not take. AC27 asks for exactness rather than
// presence, so the check has to be able to see a hatch the message was not
// supposed to name -- including one added later by someone who did not read
// this test.
var hatchVocabulary = []string{
	"--mode=auto",
	"--mode=confirm",
	"--mode=suggest",
	"TSUKU_AUTO_INSTALL_MODE",
	"auto_install_mode",
	"tsuku install",
	"tsuku update-registry",
}

// hatchesIn reports which hatches a message names, in vocabulary order.
func hatchesIn(message string) []string {
	found := []string{}
	for _, hatch := range hatchVocabulary {
		if strings.Contains(message, hatch) {
			found = append(found, hatch)
		}
	}
	return found
}

// AC25 and AC27, together, because they are two halves of one property: the
// message names the hatches that work from the state it appeared in (AC25),
// and only those (AC27).
//
// The four states are the four ways confirm-with-no-terminal is reached. In
// the first, --mode=auto survives every gate and completes the command, and
// the message says so. In the other three a mode-lowering gate would put auto
// straight back at confirm, so following the flag verbatim would arrive back
// at this same message -- and the message names no hatch at all. That is the
// half a static message gets wrong, and it is not a corner: every recipe
// without a checksum is in the third state.
//
// The mode is read back out of the message rather than rebuilt from a
// constant, and through ParseMode, which is the same function the --mode flag
// is parsed by. A test that rebuilt it would pass against a message naming
// something else entirely.
func TestRun_AC25andAC27_TheMessageNamesTheHatchesThatWork(t *testing.T) {
	tests := []struct {
		name string
		// state prepares the runner and its home for the case.
		state       func(t *testing.T, fx *indexfixture.Fixture, r *Runner)
		command     string
		wantHatches string
		// wantReason is the gate the message names in place of a hatch.
		wantReason string
	}{
		{
			name:        "auto survives every gate",
			state:       func(*testing.T, *indexfixture.Fixture, *Runner) {},
			command:     indexfixture.CommandOneProvider,
			wantHatches: "--mode=auto",
		},
		{
			name: "the configuration-permission gate would lower it",
			state: func(t *testing.T, fx *indexfixture.Fixture, _ *Runner) {
				openPermissionsConfig(t, fx.Cfg)
			},
			command:    indexfixture.CommandOneProvider,
			wantReason: "config.toml",
		},
		{
			name: "the verification gate would lower it",
			state: func(_ *testing.T, _ *indexfixture.Fixture, r *Runner) {
				r.RecipeHasVerification = func(string) bool { return false }
			},
			command:    indexfixture.CommandOneProvider,
			wantReason: indexfixture.RecipeSolo,
		},
		{
			name:        "the multiple-provider gate would lower it",
			state:       func(*testing.T, *indexfixture.Fixture, *Runner) {},
			command:     indexfixture.CommandTwoProviders,
			wantReason:  "more than one recipe",
			wantHatches: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := indexfixture.New(t)
			r, _, _, _, stderr := newHeadlessRunner(t, fx)
			tt.state(t, fx, r)

			err := r.Run(context.Background(), tt.command, nil, ModeConfirm, project.NewResolver(nil))
			if !errors.Is(err, ErrNotInteractive) {
				t.Fatalf("Run() error = %v, want ErrNotInteractive: this case is about the message that comes with it", err)
			}
			message := stderr.String()

			if got := strings.Join(hatchesIn(message), " "); got != tt.wantHatches {
				t.Fatalf("the message names %q, want %q:\n%s", got, tt.wantHatches, message)
			}
			if tt.wantReason != "" && !strings.Contains(message, tt.wantReason) {
				t.Errorf("the message names no hatch and does not say %q is why:\n%s", tt.wantReason, message)
			}
			if tt.wantHatches == "" {
				// The omission is a claim in its own right: auto really does
				// not get through from here.
				assertAutoAlsoRefuses(t, fx, tt.state, tt.command)
				return
			}

			assertHatchCompletes(t, fx, tt.state, tt.command, message)
		})
	}
}

// assertHatchCompletes follows the flag the message names, from the state that
// produced it, and requires the command to complete: installed and executed.
func assertHatchCompletes(t *testing.T, fx *indexfixture.Fixture,
	state func(*testing.T, *indexfixture.Fixture, *Runner), command, message string) {
	t.Helper()

	mode := modeFromMessage(t, message)
	r, installer, execRec, _, stderr := newHeadlessRunner(t, fx)
	state(t, fx, r)

	if err := r.Run(context.Background(), command, nil, mode, project.NewResolver(nil)); err != nil {
		t.Fatalf("following the message's own hatch (mode %s) failed: %v\n%s", mode, err, stderr.String())
	}
	if !installer.called {
		t.Error("the hatch did not install the command")
	}
	if !execRec.called {
		t.Error("the hatch did not execute the command")
	}
}

// assertAutoAlsoRefuses is why a message with no hatch is right rather than
// merely unhelpful: --mode=auto, from this state, arrives back here.
func assertAutoAlsoRefuses(t *testing.T, fx *indexfixture.Fixture,
	state func(*testing.T, *indexfixture.Fixture, *Runner), command string) {
	t.Helper()

	r, _, _, _, _ := newHeadlessRunner(t, fx)
	state(t, fx, r)

	err := r.Run(context.Background(), command, nil, ModeAuto, project.NewResolver(nil))
	if !errors.Is(err, ErrNotInteractive) {
		t.Errorf("--mode=auto returned %v from a state whose message names no hatch; if it completes the command, the message is the thing that is wrong", err)
	}
}

// modeFromMessage reads the --mode= value out of a message and parses it the
// way the flag is parsed.
func modeFromMessage(t *testing.T, message string) Mode {
	t.Helper()

	_, rest, ok := strings.Cut(message, "--mode=")
	if !ok {
		t.Fatalf("the message names no --mode= hatch:\n%s", message)
	}
	name := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '.' || r == ','
	})
	if len(name) == 0 {
		t.Fatalf("the message's --mode= names nothing:\n%s", message)
	}
	mode, valid := ParseMode(name[0])
	if !valid {
		t.Fatalf("the message names --mode=%s, which ParseMode rejects", name[0])
	}
	return mode
}
