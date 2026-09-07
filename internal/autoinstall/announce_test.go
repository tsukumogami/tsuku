package autoinstall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
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

// assertAC30 is the guard AC30 puts on a `suggest` demonstration -- this
// package's; cmd/tsuku has its own twin for the end-to-end one: no gate
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
			r, installer, _, stdout, stderr := newFixtureRunner(t, fx)
			r.ConsentReader = strings.NewReader("y\n")
			tt.state(t, fx, r)

			// Auto is named in the criterion because it is the only state the
			// gates run from; nothing is declared here, so nothing raises it
			// and the mode is the one this call passes.
			if err := r.Run(context.Background(), tt.command, nil, ModeAuto, OriginFlag, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			// AC21 is about a gate that *changes* the mode, so the change has
			// to be observed rather than inferred. The run started at auto, so
			// a gate that printed its line and returned the mode untouched
			// would install just the same -- installer.called cannot tell the
			// two apart. The prompt can: it appears only from confirm.
			if !promptShown(stdout.String()) {
				t.Fatalf("no prompt appeared, so the gate announced itself without lowering the mode.\nstderr: %s",
					stderr.String())
			}
			if !installer.called {
				t.Fatalf("nothing was installed, so no prompt was answered.\nstderr: %s", stderr.String())
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

// Two gates would fire, and the announcement is the first one's alone.
//
// R11 is about a gate that *changes* the mode, and only the first does: by the
// time the second is reached the mode is already confirm, so a line from it
// would report a change nobody made. This is also where the identifier
// lowerMode returns is pinned to the first rather than the last -- R12a's
// record names the gate that lowered the mode, and with two firing, a
// traversal that ran on, or ran backwards, names the wrong one while every
// single-gate case in this file still passes.
//
// autoBlockedBy has this case already, in AC25and27's "two gates would lower
// it" row. The announcement site had none, which is exactly the asymmetry a
// registration table is supposed to remove.
func TestLowerMode_AnnouncesOnlyTheGateThatChangedTheMode(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, stdout, stderr := newFixtureRunner(t, fx)
	// The first and second gates in table order both fire.
	openPermissionsConfig(t, fx.Cfg)
	r.RecipeHasVerification = func(string) bool { return false }
	r.ConsentReader = strings.NewReader("y\n")

	subject := gateSubject{
		command: indexfixture.CommandOneProvider,
		match:   index.BinaryMatch{Recipe: indexfixture.RecipeSolo, Command: indexfixture.CommandOneProvider},
		matches: []index.BinaryMatch{{Recipe: indexfixture.RecipeSolo, Command: indexfixture.CommandOneProvider}},
	}
	mode, gate := r.lowerMode(ModeAuto, subject)
	if mode != ModeConfirm {
		t.Fatalf("lowerMode(auto) = %v, want confirm: this case needs both gates able to fire", mode)
	}
	if gate != gateConfigPermissions {
		t.Errorf("lowerMode named %q, want %q -- the gate that changed the mode is the first one to fire",
			gate, gateConfigPermissions)
	}

	// And on stderr, through Run, where a user reads it.
	if err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil,
		ModeAuto, OriginFlag, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !promptShown(stdout.String()) || !installer.called {
		t.Fatalf("the run did not reach confirm: prompt = %v, installed = %v",
			promptShown(stdout.String()), installer.called)
	}
	if got := gatesAnnouncedIn(stderr.String()); len(got) != 1 || got[0] != gateConfigPermissions {
		t.Errorf("gates announced = %v, want exactly [%s]. Two gates would fire here and only the first "+
			"changed the mode; the second would be reporting a change it did not make.\nstderr: %s",
			got, gateConfigPermissions, stderr.String())
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
// The configuration-permission gate has five ways to fire and they call for
// different actions: a mode a user should change, an owner they cannot change
// by chmod, a file they cannot read at all, ownership it cannot determine, and
// no configured path to look at. Reporting permissions for a file owned by
// somebody else sends them to fix a thing that is not broken, which is the
// escape-hatch defect R10 names, arriving through a gate instead of through a
// message.
//
// Ownership is tested by asking about a uid the file does not have, which is
// why configPermissionCondition takes the owner as a parameter: giving a file
// away needs a second account, and asking the question from the other side
// needs nothing. Only the undeterminable-ownership branch is left to
// inspection, and it is unreachable on Linux -- os.FileInfo.Sys() is always a
// *syscall.Stat_t there.
func TestConfigPermissionCondition_NamesWhichReason(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads a directory it has no search bit on, so the unreadable case cannot be built")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	me := os.Getuid()

	// No path at all, which is not the same state as no file. os.Stat("")
	// fails with ENOENT, so IsNotExist reports true and the does-not-exist
	// branch would answer "fine" for a Runner that does not know where its
	// config file is. The gate has to fail closed there, and the condition has
	// to say which of the two it means.
	unset := configPermissionCondition("", me)
	if unset == "" {
		t.Error("a Runner with no configured config file path passes the gate; it cannot check anything, " +
			"so the safe direction is to fire")
	}
	if strings.Contains(unset, "the permissions on") {
		t.Errorf("the condition for an unset path reports permissions, which there is no file to have: %q", unset)
	}
	if !strings.Contains(unset, "no path is configured") {
		t.Errorf("the condition for an unset path does not say the path is what is missing: %q", unset)
	}

	if got := configPermissionCondition(path, me); got != "" {
		t.Errorf("a config file that does not exist fires the gate: %q", got)
	}

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	if got := configPermissionCondition(path, me); got != "" {
		t.Errorf("a config file only its owner can read fires the gate: %q", got)
	}

	// The ownership branch, which is the reason this function returns a
	// condition at all: a file at 0600 that somebody else owns is one this
	// user cannot fix by chmod, and a line telling them to is worse than
	// none. Asking about a uid that is not the file's stands in for the
	// second account a test cannot have.
	owned := configPermissionCondition(path, me+1)
	if owned == "" {
		t.Fatal("a config file owned by another user does not fire the gate")
	}
	if strings.Contains(owned, "the permissions on") {
		t.Errorf("the condition for a file owned by someone else talks about permissions, "+
			"which the user would then go and change: %q", owned)
	}

	// Two permissive modes rather than one. A condition that named the reason
	// but not the state would pass a single case and would leave the user
	// guessing which bit to clear.
	conditions := make(map[string]string, 2)
	for _, perm := range []os.FileMode{0o644, 0o640} {
		if err := os.Chmod(path, perm); err != nil {
			t.Fatalf("chmod %s: %v", perm, err)
		}
		got := configPermissionCondition(path, me)
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

	unreadable := configPermissionCondition(path, me)
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

	// And the third site, which is what carries AC30's "survives a fourth gate
	// being added" out of this package. A GateIdentifiers that listed the
	// constants instead of walking the table passes every other assertion
	// here, and the end-to-end suggest guard would then stop seeing new gates.
	if !slices.Contains(GateIdentifiers(), fourthGate) {
		t.Errorf("GateIdentifiers() = %v, which omits the registered fourth gate; the guards that read "+
			"it would not see a gate added later", GateIdentifiers())
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
// A disclosure keyed on the elevation fails here, and also fails
// TestRun_TheDisclosureNamesTheRecipeRatherThanTheConfigurationKey, which
// arranges the same state for a different purpose, and
// TestRun_TheDisclosurePrecedesThePrompt, whose confirm was set by the flag.
// What none of those shows is what this one is named for: the silent install
// itself, which is the outcome R3b changed and the reason the rule is keyed on
// determination in the first place.
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

// The disclosure's fourth fact, at every value it takes.
//
// Without this the whole corpus asserts the literal "registry", which is the
// only source internal/indexfixture produces -- so `return "registry"` passes
// every other case in this file while removing the one thing the design says
// the disclosure must carry. "installed" is the value #2552 produces and is
// the reason the fact is required at all.
func TestRecipeSource_NamesEveryValueTheIndexRecords(t *testing.T) {
	tests := []struct {
		recorded string
		want     string
	}{
		{"registry", "registry"},
		// A recipe that exists only because something installed it locally,
		// which is what a non-interactively registered source leaves behind.
		{"installed", "installed"},
		// A match nothing filled in. Reported rather than dropped: a
		// disclosure quietly missing a fact still looks like a disclosure.
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := recipeSource(index.BinaryMatch{Recipe: "solo", Source: tt.recorded}); got != tt.want {
				t.Errorf("recipeSource(%q) = %q, want %q", tt.recorded, got, tt.want)
			}
		})
	}
}

// The disclosure states all four facts, including the three that can be absent.
//
// A declaration carrying no version is ordinary: `jq = {}` in a .tsuku.toml
// parses to one and R20 passes it through verbatim. Dropping the fact leaves a
// line that reads as though the version were beside the point, when what it
// actually means is that the installer will choose.
//
// The other two absences are unreachable today -- ConfigPath comes from a
// discovered .tsuku.toml and Source is filled in by the index -- and are
// covered anyway, because a fallback nobody exercises is a fallback nobody can
// rely on. Each carries a fact the two functions argue must never go missing
// quietly -- discloseDeclaration for the path, recipeSource for the source --
// on the same ground: a line with no authorizing file still looks like a
// disclosure.
func TestDiscloseDeclaration_StatesEveryFact(t *testing.T) {
	tests := []struct {
		name       string
		match      index.BinaryMatch
		version    string
		configPath string
		want       []string
	}{
		{
			name:       "a pinned declaration from the registry",
			match:      index.BinaryMatch{Recipe: "solo", Source: "registry"},
			version:    "1.2.3",
			configPath: declaredConfigPath,
			want:       []string{DeclarationDisclosure, "solo", "1.2.3", declaredConfigPath, "registry"},
		},
		{
			name:       "a declaration with no version, of a locally installed recipe",
			match:      index.BinaryMatch{Recipe: "solo", Source: "installed"},
			version:    "",
			configPath: declaredConfigPath,
			want:       []string{DeclarationDisclosure, "solo", "no declared version", declaredConfigPath, "installed"},
		},
		{
			name:       "a declaration whose authorizing file and source are both unrecorded",
			match:      index.BinaryMatch{Recipe: "solo"},
			version:    "1.2.3",
			configPath: "",
			want:       []string{DeclarationDisclosure, "solo", "1.2.3", "unrecorded", "unknown"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := &bytes.Buffer{}
			r := NewRunner(nil, &bytes.Buffer{}, stderr)
			r.discloseDeclaration(tt.match, tt.version, tt.configPath)

			for _, want := range tt.want {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("the disclosure does not state %q: %q", want, stderr.String())
				}
			}
		})
	}
}

// D5 rejected per-invocation disclosure by name, and this is the case that
// would make it per-invocation: most invocations of a declared tool install
// nothing, because the already-installed fast path returns above every mode.
// A line there reports a decision that is not being made, and trains the
// reader to skip the line that matters.
//
// AC6 covers the same path and asserts what runs; this asserts what is not
// said. Neither substitutes for the other.
func TestRun_TheAlreadyInstalledFastPathDisclosesNothing(t *testing.T) {
	for _, state := range everyConsentState {
		t.Run(state.String(), func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, execRec, _, stderr := newFixtureRunner(t, fx)
			layDownTool(t, fx.Cfg, indexfixture.DeclaredRecipe, indexfixture.SharedVersion,
				indexfixture.CommandTwoProviders)

			err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
				state.mode, state.origin, declaredOnly())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if installer.called || !execRec.called {
				t.Fatalf("this run did not take the fast path: installed = %v, exec = %v",
					installer.called, execRec.called)
			}
			if disclosureShown(stderr.String()) {
				t.Errorf("an invocation that installed nothing carried a per-install disclosure: %q",
					stderr.String())
			}
		})
	}
}

// The identifiers are output, so they are pinned as the literals a user reads
// and a log holds.
//
// Everything else in this file reads them through the constants, which is what
// keeps those assertions honest across a rename -- and is exactly why a rename
// would otherwise be silent. "Stable" is a claim about the strings themselves,
// so one place has to spell them.
func TestAnnouncementIdentifiersAreStable(t *testing.T) {
	want := []string{"config-permissions", "recipe-verification", "multiple-providers"}
	if got := GateIdentifiers(); !slices.Equal(got, want) {
		t.Errorf("GateIdentifiers() = %v, want %v.\nThese are printed output and the audit entry's "+
			"gate field records them. Renaming one is a user-visible change, not a refactor.", got, want)
	}
	if DeclarationDisclosure != "project-declaration" {
		t.Errorf("DeclarationDisclosure = %q, want %q", DeclarationDisclosure, "project-declaration")
	}
}

// AC35 in the state that actually needs it, and the one no auto-dispatch case
// can reach: the elevation raised the mode, a gate put it back at confirm, and
// the user consented at the prompt.
//
// An install happened, so R11a's "before or at the first install the elevation
// enables" applies in full. What makes it worth its own case is that the state
// is an unverified recipe, which is the ordinary case rather than a corner:
// narrowing the condition to `effectiveMode == ModeAuto` would leave the
// commonest declared install saying nothing, and AC35 unmet outright.
//
// TestRun_TheDisclosurePrecedesThePrompt reaches the site at confirm too and
// catches that narrowing as well. Neither is the other's spare: that one
// starts from an explicitly set confirm nobody raised, so no elevation is in
// play there at all, while this one starts from a confirm the elevation raised
// and a gate put back -- which is the state AC35 is about. The elevation also
// enables the install in the D1-6 case above, and that one is the elevation
// working; this one is the elevation surviving a gate.
func TestRun_TheDisclosureSurvivesAGateLoweringTheRaisedMode(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)
	r.RecipeHasVerification = func(string) bool { return false }
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeConfirm, OriginDefault, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// The state the case claims to be in: raised, lowered again, consented to.
	if got := gatesAnnouncedIn(stderr.String()); len(got) != 1 || got[0] != gateRecipeVerification {
		t.Fatalf("gates announced = %v, want exactly [%s]: this case needs the raised mode lowered again",
			got, gateRecipeVerification)
	}
	if !promptShown(stdout.String()) {
		t.Fatalf("no prompt appeared, so the mode never reached confirm: %q", stdout.String())
	}
	if !installer.called || !execRec.called {
		t.Fatalf("no install reached: installed = %v, exec = %v", installer.called, execRec.called)
	}

	if !disclosureShown(stderr.String()) {
		t.Errorf("an install the elevation enabled was not disclosed. A gate lowered the raised mode "+
			"back to confirm, so a disclosure conditioned on the mode still being auto sees nothing "+
			"here.\nstderr: %q", stderr.String())
	}
}

// consentSnapshot answers the prompt and records what the user had been told
// by the time it was put to them.
//
// The call site's stated contract is that the prompt is where the person
// decides, so the disclosure precedes it. Without this the disclosure could
// move below the dispatch to just above the install with every other assertion
// here still passing -- and a user would answer [y/N] before being told which
// file authorized the install or where the recipe came from.
type consentSnapshot struct {
	stderr    *bytes.Buffer
	toldSoFar string
	answer    *strings.Reader
}

func (c *consentSnapshot) Read(p []byte) (int, error) {
	if c.toldSoFar == "" {
		c.toldSoFar = c.stderr.String()
	}
	return c.answer.Read(p)
}

func TestRun_TheDisclosurePrecedesThePrompt(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, stderr := newFixtureRunner(t, fx)
	consent := &consentSnapshot{stderr: stderr, answer: strings.NewReader("y\n")}
	r.ConsentReader = consent

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		ModeConfirm, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !installer.called {
		t.Fatal("nothing was installed, so no prompt was answered")
	}
	if !disclosureShown(consent.toldSoFar) {
		t.Errorf("the user was asked to consent before being told what a declaration had determined. "+
			"Facts delivered after the decision are not disclosure.\ntold by then: %q\ntold in the end: %q",
			consent.toldSoFar, stderr.String())
	}
}

// The disclosure names the recipe, not the configuration key, and this is the
// case that can tell them apart: an org-scoped key carries a source component
// and a bare key does not, so everywhere else in this corpus the two strings
// are equal and a disclosure built from either passes.
//
// The run path does not honor a source: the recipe name comes from the binary
// index, and what will be installed is the bare name. A disclosure naming the
// key would print "org-a/registry:fixture-dup-omega" beside a fourth fact
// saying the recipe came from the registry -- two statements about provenance,
// one of them describing an install this run is not performing, in a line
// whose whole purpose is to say where the thing came from.
//
// The suggest instruction one dispatch over has this case and this hazard
// written down. The disclosure inherited the exposure and neither.
func TestRun_TheDisclosureNamesTheRecipeRatherThanTheConfigurationKey(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, stderr := newFixtureRunner(t, fx)
	const keySource = "org-a/"

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag,
		declaring(map[string]string{keySource + indexfixture.DeclaredRecipe: indexfixture.SharedVersion}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Fatalf("installed %q, want the bare %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
	if !disclosureShown(stderr.String()) {
		t.Fatalf("nothing was disclosed: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), keySource) {
		t.Errorf("the disclosure carries the source component %q from the configuration key, which this "+
			"path does not honor, next to a fourth fact naming where the recipe actually came from: %q",
			keySource, stderr.String())
	}
	if !strings.Contains(stderr.String(), indexfixture.DeclaredRecipe) {
		t.Errorf("the disclosure does not name the recipe that will be installed: %q", stderr.String())
	}
}

// lowerMode names the gate that lowered the mode, which is the value R12a's
// record will carry. Nothing reads it yet, so this is where it is pinned --
// without it `return ModeConfirm, ""` changes no outcome anywhere and the
// value is computed for nobody.
func TestLowerMode_NamesTheGateThatFired(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, _, _, _ := newFixtureRunner(t, fx)
	solo := index.BinaryMatch{Recipe: indexfixture.RecipeSolo, Command: indexfixture.CommandOneProvider}
	subject := gateSubject{
		command: indexfixture.CommandOneProvider,
		match:   solo,
		matches: []index.BinaryMatch{solo},
	}

	r.RecipeHasVerification = func(string) bool { return false }
	if mode, gate := r.lowerMode(ModeAuto, subject); mode != ModeConfirm || gate != gateRecipeVerification {
		t.Errorf("lowerMode(auto) = %v, %q; want confirm, %q", mode, gate, gateRecipeVerification)
	}

	// No gate fires, so there is no gate to name.
	r.RecipeHasVerification = func(string) bool { return true }
	if mode, gate := r.lowerMode(ModeAuto, subject); mode != ModeAuto || gate != "" {
		t.Errorf("lowerMode(auto) = %v, %q; want auto and no gate", mode, gate)
	}

	// A mode that is not auto is returned untouched and consults no gate, so
	// there is nothing to name there either.
	r.RecipeHasVerification = func(string) bool { return false }
	if mode, gate := r.lowerMode(ModeSuggest, subject); mode != ModeSuggest || gate != "" {
		t.Errorf("lowerMode(suggest) = %v, %q; want suggest and no gate", mode, gate)
	}
}
