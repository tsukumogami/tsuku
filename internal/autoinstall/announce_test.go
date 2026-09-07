package autoinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// Nothing changes the mode in silence, in either direction: the three gates
// lower it and say so, and a project declaration that determined what gets
// installed says so too.

// gatesAnnouncedIn reports which mode-lowering gates announced themselves in a
// stream, by identifier, in table order.
//
// It reads modeGates rather than a list written out here, and that is what
// makes AC30's "no gate intervened" assertion survive a fourth gate being
// added: a gate someone registers without writing a test is one this check can
// already see.
func gatesAnnouncedIn(stderr string) []string {
	found := []string{}
	for _, gate := range modeGates {
		if strings.Contains(stderr, gate.id) {
			found = append(found, gate.id)
		}
	}
	return found
}

// disclosureShown reports whether the elevation disclosure reached a stream.
func disclosureShown(stderr string) bool {
	return strings.Contains(stderr, DeclarationDisclosure)
}

// assertAC30 is the guard AC30 puts on every `suggest` demonstration: no gate
// diverted the mode and no declaration raised it, so what the demonstration
// observed was `suggest` being honored rather than some other state that
// happens to install nothing.
//
// Both halves are asserted by identifier rather than by enumerating the
// preconditions that would produce them. The enumeration is the thing that was
// tried twice while the criteria were written and was incomplete both times:
// picking a single-provider command with a checksum excludes two of the three
// gates and says nothing at all about the configuration-permission gate, whose
// precondition is filesystem state.
func assertAC30(t *testing.T, stderr string) {
	t.Helper()
	if got := gatesAnnouncedIn(stderr); len(got) != 0 {
		t.Errorf("gates %v diverted the mode, so this run observed confirm rather than suggest.\nstderr: %s",
			got, stderr)
	}
	if disclosureShown(stderr) {
		t.Errorf("a declaration determined an install here; suggest installs nothing.\nstderr: %s", stderr)
	}
}

// assertNotInstalled is AC30's fourth assertion: the recipe was not already
// installed at the declared version when the run began. Where it is, the
// already-installed fast path returns before any mode is consulted and the
// demonstration records a protection that was never exercised.
func assertNotInstalled(t *testing.T, cfg *config.Config, recipe, version, command string) {
	t.Helper()
	path := filepath.Join(cfg.ToolBinDir(recipe, version), command)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s@%s is already installed at %s; the fast path returns before the mode is consulted",
			recipe, version, path)
	}
}

// withGate registers one more mode-lowering gate for the duration of a test.
//
// A test that reaches for this is asking what happens to a gate nobody wired
// up by hand, which is the only way to observe the property the table exists
// for.
func withGate(t *testing.T, gate modeGate) {
	t.Helper()
	saved := modeGates
	modeGates = append(append([]modeGate(nil), modeGates...), gate)
	t.Cleanup(func() { modeGates = saved })
}

// AC21. Starting from auto, each gate that changes the mode writes one line
// naming itself and the condition that fired it, and the three name themselves
// distinctly.
//
// Each case asserts the other two identifiers are absent as well as its own
// being present. Without that a single generic line containing all three names
// would pass every row, which is the shape a "simplification" collapsing them
// arrives in.
func TestRun_AC21_EachGateAnnouncesItselfAndItsCondition(t *testing.T) {
	tests := []struct {
		name string
		// state puts the runner in the case's state.
		state         func(t *testing.T, fx *indexfixture.Fixture, r *Runner)
		command       string
		wantGate      string
		wantCondition string
	}{
		{
			name: "the configuration-permission gate",
			state: func(t *testing.T, fx *indexfixture.Fixture, _ *Runner) {
				openPermissionsConfig(t, fx.Cfg)
			},
			command:       indexfixture.CommandOneProvider,
			wantGate:      gateConfigPermissions,
			wantCondition: "config.toml",
		},
		{
			name: "the verification gate",
			state: func(_ *testing.T, _ *indexfixture.Fixture, r *Runner) {
				r.RecipeHasVerification = func(string) bool { return false }
			},
			command:  indexfixture.CommandOneProvider,
			wantGate: gateRecipeVerification,
			// The recipe, which is what tells this line from a constant: the
			// gate reads a different recipe for a different command.
			wantCondition: indexfixture.RecipeSolo,
		},
		{
			name:     "the multiple-provider gate",
			state:    func(*testing.T, *indexfixture.Fixture, *Runner) {},
			command:  indexfixture.CommandTwoProviders,
			wantGate: gateMultipleProviders,
			// The command, for the reason above.
			wantCondition: indexfixture.CommandTwoProviders,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, _, _, stderr := newFixtureRunner(t, fx)
			r.ConsentReader = strings.NewReader("y\n")
			tt.state(t, fx, r)

			// Auto is named in the criterion because it is the only state the
			// gates run from; nothing is declared here, so nothing raises it
			// and the mode is the one this call passes.
			if err := r.Run(context.Background(), tt.command, nil, ModeAuto, OriginFlag, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			// The gate has to have changed the mode rather than merely
			// printed: consent was given at the prompt, so an install that
			// happened is an install that asked.
			if !installer.called {
				t.Fatalf("nothing was installed, so no prompt was answered and the mode never reached confirm.\nstderr: %s",
					stderr.String())
			}

			announced := gatesAnnouncedIn(stderr.String())
			if len(announced) != 1 || announced[0] != tt.wantGate {
				t.Fatalf("gates announced = %v, want exactly [%s].\nstderr: %s",
					announced, tt.wantGate, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantCondition) {
				t.Errorf("the line names no condition containing %q: %s", tt.wantCondition, stderr.String())
			}
			if lines := strings.Count(strings.TrimRight(stderr.String(), "\n"), "\n") + 1; lines != 1 {
				t.Errorf("the gate wrote %d lines, want one:\n%s", lines, stderr.String())
			}
		})
	}
}

// The identifiers are distinct, which every "no gate intervened" assertion
// rests on: two gates sharing an identifier make one of them unobservable, and
// gatesAnnouncedIn would report the wrong gate as having fired.
//
// It is asserted rather than left to inspection because the failure is silent.
// Nothing about a duplicated identifier breaks a build or a run.
func TestModeGates_IdentifiersAreDistinctAndNonEmpty(t *testing.T) {
	seen := make(map[string]int, len(modeGates))
	for i, gate := range modeGates {
		if gate.id == "" {
			t.Errorf("modeGates[%d] has no identifier; it cannot announce itself and no test can see it fire", i)
			continue
		}
		if first, dup := seen[gate.id]; dup {
			t.Errorf("modeGates[%d] and modeGates[%d] both announce themselves as %q", first, i, gate.id)
		}
		seen[gate.id] = i
	}
}

// AC21's "a constant string does not pass", for the gate that can fire for
// more than one reason.
//
// The configuration-permission gate has four ways to fire and they call for
// different actions: a mode a user should change, an owner they cannot change
// by chmod, a file they cannot read at all. Reporting permissions for a file
// owned by somebody else sends them to fix a thing that is not broken, which
// is the escape-hatch defect R10 names, arriving through a gate instead of
// through a message.
//
// Ownership is left to inspection rather than tested: giving a file away needs
// a second uid, which a test cannot have without privileges it should not ask
// for.
func TestConfigPermissionCondition_NamesWhichReason(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads a directory it has no search bit on, so the unreadable case cannot be built")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if got := configPermissionCondition(path); got != "" {
		t.Errorf("a config file that does not exist fires the gate: %q", got)
	}

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	if got := configPermissionCondition(path); got != "" {
		t.Errorf("a config file only its owner can read fires the gate: %q", got)
	}

	// Two permissive modes rather than one. A condition that named the reason
	// but not the state would pass a single case and would leave the user
	// guessing which bit to clear.
	conditions := make(map[string]string, 2)
	for _, perm := range []os.FileMode{0o644, 0o640} {
		if err := os.Chmod(path, perm); err != nil {
			t.Fatalf("chmod %s: %v", perm, err)
		}
		got := configPermissionCondition(path)
		if got == "" {
			t.Fatalf("a config file at %#o does not fire the gate", perm)
		}
		if !strings.Contains(got, fmt.Sprintf("%#o", perm)) {
			t.Errorf("the condition for %#o does not name it: %q", perm, got)
		}
		conditions[perm.String()] = got
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("restoring the mode: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod on %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	unreadable := configPermissionCondition(path)
	if unreadable == "" {
		t.Fatal("a config file that cannot be stat'ed does not fire the gate; the cautious direction is to fire")
	}
	for perm, condition := range conditions {
		if unreadable == condition {
			t.Errorf("the unreadable case reports the same condition as the %s case (%q); "+
				"a user acting on it would chmod a file they cannot read", perm, condition)
		}
	}
}

// The property the table exists for, and the one the shared predicates could
// not give: a gate registered once reaches *both* the announcement and the
// escape-hatch message, with nobody having to remember the second site.
//
// The failure it rules out is specific. A hatch computation that restated the
// gates instead of iterating them goes on saying "use --mode=auto" in a state
// the fourth gate blocks, so a user follows the flag verbatim, hits the same
// gate, and arrives back at the same message having changed nothing.
//
// The fourth gate is registered rather than described, and the message is
// observed before and after. Asserting that the message *would* change is the
// thing this test exists instead of.
func TestModeGates_AFourthGateReachesBothSites(t *testing.T) {
	const (
		fourthGate      = "fixture-fourth-gate"
		fourthCondition = "the fixture gate was registered"
	)

	// Before. Every registered gate passes in this state, so the message
	// names the hatch.
	fx := indexfixture.New(t)
	r, _, _, _, stderr := newHeadlessRunner(t, fx)
	err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil, ModeConfirm, OriginFlag, nil)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("Run() error = %v, want ErrNotInteractive", err)
	}
	before := stderr.String()
	if !strings.Contains(before, "--mode=auto") {
		t.Fatalf("with every gate passing the message should name the hatch: %q", before)
	}

	withGate(t, modeGate{
		id:     fourthGate,
		blocks: func(*Runner, gateSubject) string { return fourthCondition },
	})

	// After, in the same state, with nothing else changed.
	fxAfter := indexfixture.New(t)
	rAfter, _, _, _, stderrAfter := newHeadlessRunner(t, fxAfter)
	err = rAfter.Run(context.Background(), indexfixture.CommandOneProvider, nil, ModeConfirm, OriginFlag, nil)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("Run() error = %v, want ErrNotInteractive", err)
	}
	after := stderrAfter.String()

	if strings.Contains(after, "--mode=auto") {
		t.Errorf("the message still names --mode=auto in a state the fourth gate blocks, so the hatch "+
			"computation is not reading the table:\nbefore: %q\nafter:  %q", before, after)
	}
	if !strings.Contains(after, fourthCondition) {
		t.Errorf("the message names no condition from the fourth gate: %q", after)
	}

	// The other site, for the same registration: the gate lowers auto and
	// announces itself by the identifier it was registered under.
	rAnnounce, installer, _, _, announced := newFixtureRunner(t, fxAfter)
	rAnnounce.ConsentReader = strings.NewReader("y\n")
	if err := rAnnounce.Run(context.Background(), indexfixture.CommandOneProvider, nil,
		ModeAuto, OriginFlag, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !installer.called {
		t.Fatal("the fourth gate did not lower auto to confirm: nothing answered a prompt")
	}
	if !strings.Contains(announced.String(), fourthGate) {
		t.Errorf("the fourth gate lowered the mode without announcing itself: %q", announced.String())
	}
}

// disclosingInstaller records what the user had already been told at the
// moment the install began, which is where "before the install begins" is
// actually observable.
type disclosingInstaller struct {
	mockInstaller
	stderr    *bytes.Buffer
	toldSoFar string
}

func (d *disclosingInstaller) Install(ctx context.Context, recipe, version string) error {
	d.toldSoFar = d.stderr.String()
	return d.mockInstaller.Install(ctx, recipe, version)
}

// D1-6 and AC35. An install the elevation enabled discloses the recipe, the
// version, the authorizing file's path and the recipe's source, before it
// begins, on stderr rather than in the audit log.
//
// All four facts are asserted, and the fourth is the one to keep. The run path
// cannot register a source, but tsukumogami/tsuku#2552 can: once it has, the
// index carries the recipe under a bare name and a bare declaration matches it
// here with nothing naming where it came from. A disclosure of the other three
// facts would tell the user an install happened, which they can already see.
func TestRun_D1_6_TheElevationIsDisclosedBeforeTheInstall(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, execRec, _, stderr := newFixtureRunner(t, fx)
	installer := &disclosingInstaller{stderr: stderr}
	r.Installer = installer

	assertNotInstalled(t, fx.Cfg, indexfixture.DeclaredRecipe, indexfixture.SharedVersion,
		indexfixture.CommandTwoProviders)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeConfirm, OriginDefault, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !installer.called || !execRec.called {
		t.Fatalf("no install reached: installed = %v, exec = %v", installer.called, execRec.called)
	}

	// Everything the rule names, read out of what the user had been told by
	// the time the install started rather than out of the finished stream.
	for _, want := range []string{
		indexfixture.DeclaredRecipe,
		indexfixture.SharedVersion,
		declaredConfigPath,
		"registry",
	} {
		if !strings.Contains(installer.toldSoFar, want) {
			t.Errorf("the disclosure does not name %q before the install begins: %q", want, installer.toldSoFar)
		}
	}
	if !disclosureShown(installer.toldSoFar) {
		t.Errorf("the disclosure carries no identifier, so nothing can assert its absence: %q",
			installer.toldSoFar)
	}
}

// The disclosure is keyed on what the declaration *determined*, not on whether
// it raised the mode -- and this is the case that tells those two apart.
//
// The mode here is auto, set explicitly, so elevate raises nothing and the
// declaration moved no mode at all. What it did move is the recipe: R3b stops
// the multiple-provider gate firing once a declaration has narrowed the
// candidates, so this command installs silently where before it prompted. No
// elevation, no gate, and a repository-supplied file turning a prompt into an
// unattended install.
//
// A disclosure keyed on the elevation passes every other case in this file and
// fails only here, which is what makes this the case worth writing.
func TestRun_TheDisclosureFollowsTheDeterminationRatherThanTheElevation(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, stdout, stderr := newFixtureRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// The state the case claims to be in: an unattended install, of the
	// declared recipe, with no gate having intervened and no mode raised.
	if promptShown(stdout.String()) {
		t.Fatalf("this run prompted, so it is not the silent-install case: %q", stdout.String())
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Fatalf("installed %q, want the declared %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
	if got := gatesAnnouncedIn(stderr.String()); len(got) != 0 {
		t.Fatalf("gates %v fired, so something other than the declaration settled this: %s",
			got, stderr.String())
	}

	if !disclosureShown(stderr.String()) {
		t.Errorf("an ambiguous command was installed silently because the project declared one provider, "+
			"and nothing said so. The mode was auto before the declaration and auto after it, so a "+
			"disclosure keyed on the elevation sees nothing here.\nstderr: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), declaredConfigPath) {
		t.Errorf("the disclosure does not name the file that authorized it: %q", stderr.String())
	}
}

// An undeclared command discloses nothing. The disclosure is about what a
// repository-supplied file determined, so a run no such file touched has
// nothing to disclose, and a line here would be the per-invocation noise D5
// rejected.
func TestRun_NothingIsDisclosedForAnUndeclaredCommand(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, stderr := newFixtureRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !installer.called {
		t.Fatal("nothing was installed, so this run never reached the point a disclosure would be written")
	}
	if disclosureShown(stderr.String()) {
		t.Errorf("an undeclared command was disclosed as though a declaration determined it: %q", stderr.String())
	}
}
