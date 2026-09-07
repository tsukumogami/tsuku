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
// index knows, where the resolver finds nothing to declare at all. Here it
// finds a real declaration, of a real recipe, that provides a different
// command -- so a rule reading "this project declares something" rather than
// "this project declares this command" is caught here and is not there.
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
			// doing. A gate that lowered a raised auto back to confirm would
			// name itself here, and the outcome would look the same.
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing: this prompt is the mode's, not a gate's", stderr.String())
			}
		})
	}
}
