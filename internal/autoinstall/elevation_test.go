package autoinstall

import (
	"context"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/project"
)

// The bounded elevation, which is the decision this work exists to record.
//
// What separates it from the unconditional elevation it replaces is not that a
// declared tool installs without asking -- that was true before, and a test
// asserting only that cannot tell the two apart. It is where the elevation
// stops: at the commands the file declares, and at the modes nobody set.

// The rule itself, at every input it has. Run discards the origin it returns
// because nothing on that path writes a record yet, so this is where the value
// the record will carry is pinned -- without it the second half of the rule is
// computed and read by nobody, and a later reader would be free to disagree
// with it.
func TestElevate(t *testing.T) {
	tests := []struct {
		name       string
		mode       Mode
		origin     Origin
		declared   bool
		wantMode   Mode
		wantOrigin Origin
	}{
		{"the unset default, declared", ModeConfirm, OriginDefault, true, ModeAuto, OriginProject},
		{"the unset default, undeclared", ModeConfirm, OriginDefault, false, ModeConfirm, OriginDefault},
		// The zero value, which is a caller that resolved no origin rather
		// than one that resolved "nobody set a mode". It raises nothing, and
		// the mode it is paired with is the one that would be raised wrongly.
		{"no origin at all, declared", ModeSuggest, OriginUnset, true, ModeSuggest, OriginUnset},
		{"suggest by the flag", ModeSuggest, OriginFlag, true, ModeSuggest, OriginFlag},
		{"confirm by the environment", ModeConfirm, OriginEnvironment, true, ModeConfirm, OriginEnvironment},
		{"confirm by the config", ModeConfirm, OriginConfig, true, ModeConfirm, OriginConfig},
		{"auto by the flag", ModeAuto, OriginFlag, true, ModeAuto, OriginFlag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, origin := elevate(tt.mode, tt.origin, tt.declared)
			if mode != tt.wantMode || origin != tt.wantOrigin {
				t.Errorf("elevate(%v, %v, %v) = %v, %v; want %v, %v",
					tt.mode, tt.origin, tt.declared, mode, origin, tt.wantMode, tt.wantOrigin)
			}
		})
	}
}

// AC36's first half and D1-1. With nothing configured anywhere, a declared
// command installs without prompting.
//
// No ConsentReader is wired, so a run that fell through to the prompt would
// read end-of-file and come back ErrUserDeclined rather than installing. The
// declared recipe is the one the index ranks second, so an install of the
// sibling is a distinguishable wrong answer rather than a coincidence.
func TestRun_AC36_TheUnsetDefaultIsRaisedForADeclaredCommand(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeConfirm, OriginDefault, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if installer.recipe != indexfixture.DeclaredRecipe || installer.ver != indexfixture.SharedVersion {
		t.Errorf("installed %s@%s, want the declared %s@%s",
			installer.recipe, installer.ver, indexfixture.DeclaredRecipe, indexfixture.SharedVersion)
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt appeared with nothing configured and the command declared: %q", stdout.String())
	}
	if !execRec.called {
		t.Error("exec was not called")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing: no gate fired here", stderr.String())
	}
}

// D1-1 in the state the shell hook puts it in: nothing configured, the command
// declared, and no terminal to answer a prompt on. The elevation is what
// carries it past the terminal check, and this is the ordinary case rather
// than a corner -- a command-not-found hook has no terminal by construction.
//
// AC2 is the neighboring case and does not cover it: its auto is explicitly
// set, so the run succeeds there whether or not a declaration raises anything.
func TestRun_TheRaisedDefaultSurvivesWithNoTerminal(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, _ := newHeadlessRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeConfirm, OriginDefault, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
	if !execRec.called {
		t.Error("exec was not called")
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt appeared with no terminal to answer it: %q", stdout.String())
	}
}

// AC36's second half and D1-2, which is the criterion that distinguishes a
// bounded elevation from an unbounded one. An undeclared command in a project
// that declares something else behaves exactly as it would in a directory with
// no .tsuku.toml in it.
//
// The comparison is against the whole outcome rather than against a chosen
// observable, because "unchanged" is the claim and any difference falsifies
// it. It runs in every consent state: the unset default is where an unbounded
// elevation would leak, and the three explicit ones are where a rule keyed on
// the file rather than on the command would.
//
// AC10 compares the same two runs over a configuration naming a recipe no
// index knows. This one declares a real recipe that provides a different
// command, which is the configuration a working repository actually has. Both
// reach Run with no declaration, because the resolver is asked about this
// command's matches and answers about those alone -- so what this pins is the
// composition rather than the resolver: whatever the file holds, an undeclared
// command comes out of Run the way it does with no file at all.
func TestRun_AC36_TheElevationDoesNotReachAnUndeclaredCommand(t *testing.T) {
	for _, state := range everyConsentState {
		t.Run(state.String(), func(t *testing.T) {
			fx := indexfixture.New(t)
			inProject := runOutcome(t, fx, indexfixture.CommandOneProvider, state,
				declaredOnly(), withTerminal)
			noProject := runOutcome(t, fx, indexfixture.CommandOneProvider, state,
				project.NewResolver(nil), withTerminal)

			if inProject != noProject {
				t.Errorf("a project declaring %q changed how the undeclared command %q ran.\nin project: %+v\nno project: %+v",
					indexfixture.DeclaredRecipe, indexfixture.CommandOneProvider, inProject, noProject)
			}
			// The two agreeing is worth nothing if they agree on the wrong
			// thing, and the unset default is the state where that would
			// matter: what it must produce is the prompt.
			if state.origin == OriginDefault && !promptShown(inProject.stdout) {
				t.Errorf("with nothing configured, the undeclared command did not prompt: %+v", inProject)
			}
		})
	}
}

// AC38 and D1-4. An explicitly set confirm is honored for a declared command:
// it prompts, for the declared recipe at the declared version.
//
// An explicitly set confirm and the unset default are the same Mode and
// different states, and this is the pair that says so -- the same command,
// under the same mode, installs unattended in the test above and asks here.
// Nothing but the origin differs.
//
// The environment row is also D1-5's second half. The state D1-5 describes --
// TSUKU_AUTO_INSTALL_MODE=auto with no corroborating config -- reaches this
// package as exactly this pair, because the escalation restriction returns a
// confirm the environment supplied. That it does so is resolveMode's, and the
// two halves are run together end to end in cmd/tsuku.
func TestRun_AC38_AnExplicitConfirmIsHonoredForADeclaredCommand(t *testing.T) {
	for _, origin := range []Origin{OriginFlag, OriginEnvironment, OriginConfig} {
		t.Run(origin.String(), func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, _, stdout, stderr := newFixtureRunner(t, fx)
			r.ConsentReader = strings.NewReader("y\n")

			err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
				ModeConfirm, origin, declaredOnly())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if !promptShown(stdout.String()) {
				t.Fatalf("no prompt appeared: a confirm set by the %s was raised by the declaration\nstdout: %q",
					origin, stdout.String())
			}
			want := indexfixture.DeclaredRecipe + "@" + indexfixture.SharedVersion
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("prompt = %q, want it to name %q", stdout.String(), want)
			}
			if installer.recipe != indexfixture.DeclaredRecipe {
				t.Errorf("consent was given for %q but %q was installed", want, installer.recipe)
			}
			// The prompt has to be the honored mode's rather than a gate's
			// doing: a gate lowering a raised auto back to confirm produces
			// the same prompt from the opposite state. What rules the gates
			// out is the fixture -- a verified recipe, a list narrowed to one,
			// and no config.toml to have permissions -- and this assertion is
			// the part of that which is observable, since the
			// configuration-permission gate is the one that announces itself.
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing: this prompt is the mode's, not a gate's", stderr.String())
			}
		})
	}
}
