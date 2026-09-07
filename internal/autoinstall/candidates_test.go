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
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/project"
)

// Every case in this file runs against internal/indexfixture, and the ones
// that matter run against CommandTwoProviders, where the declared recipe ranks
// *second*. That is what tells a working narrowing from a narrowing that never
// matches anything: with the declared recipe ranked first the two agree on
// every observable, so a test built that way passes either way.

// newFixtureRunner builds a Runner over the fixture's own $TSUKU_HOME, wired
// to the fixture index. Install and exec are recorded rather than performed --
// what these cases are about is which recipe and version reach them.
//
// The fixture home has no config.toml, so the configuration-permission gate
// passes; a case that wants that gate to fire writes one.
func newFixtureRunner(t *testing.T, fx *indexfixture.Fixture) (*Runner, *mockInstaller, *execRecorder, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	r := NewRunner(fx.Cfg, stdout, stderr)
	r.Lookup = fx.Lookup

	installer := &mockInstaller{}
	execRec := &execRecorder{}
	r.Installer = installer
	r.Exec = execRec.exec
	// Auto mode is the interesting mode here and it needs a verified recipe;
	// a case that wants the verification gate to fire overrides this.
	r.RecipeHasVerification = func(string) bool { return true }
	// A terminal is attached unless a case says otherwise, for the reason
	// newTestRunner's is: confirm mode with no terminal refuses rather than
	// prompting, and the cases about the prompt are not about the terminal.
	r.IsTerminal = func() bool { return true }
	return r, installer, execRec, stdout, stderr
}

// declaring builds the production project resolver over tools, given as
// recipe -> version. It is the real *project.Resolver rather than a double,
// so the declaration set these cases narrow on is the one `tsuku run` builds.
func declaring(tools map[string]string) *project.Resolver {
	reqs := make(map[string]project.ToolRequirement, len(tools))
	for name, version := range tools {
		reqs[name] = project.ToolRequirement{Version: version}
	}
	return project.NewResolver(&project.ConfigResult{
		Config: &project.ProjectConfig{Tools: reqs},
		Path:   "/project/.tsuku.toml",
	})
}

// layDownTool writes a binary where an installed recipe's would be, so the
// stat in Run's declared fast path finds it, and returns that path.
//
// It is not an install: nothing here writes state.json or touches the index,
// so the match this recipe comes back as still reports Installed=false. What
// the declared branch stats is the version directory, not that flag, which is
// why this is enough for the cases below and would not be for an undeclared
// one.
func layDownTool(t *testing.T, cfg *config.Config, recipe, version, command string) string {
	t.Helper()
	binDir := cfg.ToolBinDir(recipe, version)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", binDir, err)
	}
	path := filepath.Join(binDir, command)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// sibling is the provider of CommandTwoProviders the project did *not*
// declare -- the one the index ranks first, and the one every wrong answer
// below reaches for.
const sibling = indexfixture.RecipeDupFirst

// declaredOnly is the project configuration these cases run under: one
// provider of CommandTwoProviders, at the version both providers install at.
func declaredOnly() *project.Resolver {
	return declaring(map[string]string{
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	})
}

// promptShown reports whether the confirm prompt reached stdout.
func promptShown(stdout string) bool {
	return strings.Contains(stdout, "[y/N]")
}

// consent is a resolved mode together with where it came from, which is the
// pair Run takes. The two travel together because the same Mode means
// different things depending on whether anyone chose it: a confirm nobody set
// is what a declaration raises, and a confirm somebody set is not.
type consent struct {
	mode   Mode
	origin Origin
}

func (c consent) String() string {
	return c.mode.String() + "/" + c.origin.String()
}

// everyConsentState is every state a user can actually arrive in. The unset
// default is confirm, and each of the three modes can be set explicitly; a
// suggest or an auto whose origin is default is not a state, because neither
// is ever what saying nothing produces.
//
// The flag stands for all three explicit routes here. What separates them is
// which origin they produce, which is resolveMode's and is pinned there --
// below this line all three are the same value with a different name.
var everyConsentState = []consent{
	{ModeConfirm, OriginDefault},
	{ModeSuggest, OriginFlag},
	{ModeConfirm, OriginFlag},
	{ModeAuto, OriginFlag},
}

// AC1. Effective mode auto, one declared provider of a two-provider command,
// nothing installed: the declared recipe is installed at the declared version
// and executed, with no prompt.
func TestRun_AC1_AutoInstallsTheDeclaredRecipe(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, stdout, _ := newFixtureRunner(t, fx)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want the declared recipe %q (the index ranks %q first)",
			installer.recipe, indexfixture.DeclaredRecipe, sibling)
	}
	if installer.ver != indexfixture.SharedVersion {
		t.Errorf("installed version %q, want the declared %q", installer.ver, indexfixture.SharedVersion)
	}
	if promptShown(stdout.String()) {
		t.Errorf("a prompt appeared under auto: %q", stdout.String())
	}
	if !execRec.called {
		t.Error("exec was not called")
	}
}

// AC3. Effective mode confirm: the prompt names the declared recipe at the
// declared version and no other recipe.
//
// With nothing configured, a declared command is elevated to auto on the way
// in, so confirm is reached here the way a user would reach it -- by the
// verification gate lowering the mode. That is a real path rather than a
// contrivance: the gate fires whenever the recipe to be installed carries no
// checksum.
func TestRun_AC3_ConfirmPromptsForTheDeclaredRecipe(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, stdout, _ := newFixtureRunner(t, fx)
	r.RecipeHasVerification = func(string) bool { return false }
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeConfirm, OriginDefault, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	prompt := stdout.String()
	if !promptShown(prompt) {
		t.Fatalf("no prompt appeared: %q", prompt)
	}
	want := indexfixture.DeclaredRecipe + "@" + indexfixture.SharedVersion
	if !strings.Contains(prompt, want) {
		t.Errorf("prompt = %q, want it to name %q", prompt, want)
	}
	if strings.Contains(prompt, sibling) {
		t.Errorf("prompt = %q, want it to name no recipe but the declared one; it names %q",
			prompt, sibling)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("consent was given for %q but %q was installed", want, installer.recipe)
	}
}

// AC4, D1-3, and the suggest clause of AC5. A suggest somebody set is honored
// for a declared command: the instruction names the declared recipe at the
// declared version, nothing is installed, and nothing is executed.
//
// This replaces the test that pinned AC4 as unobservable. Under the
// unconditional elevation, a declared command was raised to auto whatever the
// user had set and the gates below only lower auto to confirm, so the suggest
// dispatch was unreachable for a declared command and the criterion had no
// state to be observed in.
//
// The three cases are the three origins an explicit suggest arrives by, which
// is D1-3's "flag, environment variable and configuration key in turn" as this
// package sees them. Which route produces which origin is resolveMode's half,
// and it is pinned in cmd/tsuku/cmd_run_test.go, where those three are read;
// the two halves together are the criterion.
//
// The sibling is laid down at the declared version because that is AC5's
// suggest clause: a provider the project did not declare is present and
// runnable, and suggest still executes nothing.
func TestRun_AC4_AnExplicitSuggestIsHonoredForADeclaredCommand(t *testing.T) {
	for _, origin := range []Origin{OriginFlag, OriginEnvironment, OriginConfig} {
		t.Run(origin.String(), func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)
			layDownTool(t, fx.Cfg, sibling, indexfixture.SharedVersion, indexfixture.CommandTwoProviders)

			err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
				ModeSuggest, origin, declaredOnly())

			if !errors.Is(err, ErrSuggestOnly) {
				t.Fatalf("Run() error = %v, want ErrSuggestOnly: a suggest set by the %s was not honored\nstdout: %q",
					err, origin, stdout.String())
			}
			// The positive observable. Without it every assertion below is
			// met by a run that failed before reaching the mode at all.
			want := "tsuku install " + indexfixture.DeclaredRecipe + "@" + indexfixture.SharedVersion
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want an instruction naming %q", stdout.String(), want)
			}
			if strings.Contains(stdout.String(), sibling) {
				t.Errorf("stdout = %q, want it to name no recipe but the declared one; it names %q",
					stdout.String(), sibling)
			}
			if installer.called {
				t.Errorf("installed %q under suggest", installer.recipe)
			}
			if execRec.called {
				t.Errorf("executed %q under suggest; the declared recipe is not installed and the "+
					"sibling that is was not the one declared", execRec.binary)
			}
			// Nothing on stderr. No gate can have fired here -- all three are
			// guarded on the mode already being auto -- so what this asserts
			// is the other half: that nothing raised the mode and then said
			// so. It is written against the stream rather than against a
			// particular message so that it goes on holding when the
			// announcements exist to be written.
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing: no gate fired and no mode was raised", stderr.String())
			}
		})
	}
}

// The instruction names the recipe, not the configuration key, and this is the
// case that can tell the two apart: an org-scoped key carries a source
// component, and the bare key every other case here uses does not.
//
// The run path does not honor a source: the recipe name comes from the binary
// index, and the install this run declined to perform would have been of that
// bare name. An instruction carrying the key would send a user to `tsuku
// install org-a/registry:koto`, which is a different install, reached through
// a line a repository-supplied file decided the contents of. The refusal one
// file over does print the key, and that is not a contradiction: it is
// choosing between two declarations the user must be able to tell apart, and
// there is nothing here to choose between.
func TestRun_SuggestNamesTheRecipeRatherThanTheConfigurationKey(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, _, stdout, _ := newFixtureRunner(t, fx)
	const source = "org-a/"

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeSuggest, OriginFlag,
		declaring(map[string]string{source + indexfixture.DeclaredRecipe: indexfixture.SharedVersion}))
	if !errors.Is(err, ErrSuggestOnly) {
		t.Fatalf("Run() error = %v, want ErrSuggestOnly", err)
	}

	want := "tsuku install " + indexfixture.DeclaredRecipe + "@" + indexfixture.SharedVersion
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout = %q, want an instruction naming %q", stdout.String(), want)
	}
	if strings.Contains(stdout.String(), source) {
		t.Errorf("the instruction carries the source component %q, which this path does not honor: %q",
			source, stdout.String())
	}
}

// AC5, first half. A *different* provider is already laid down at the declared
// version. The declared one is installed anyway; the installed sibling is not
// executed.
//
// This is the case a lost narrowing gets wrong most quietly. Without it, the
// declared branch stats the sibling's version directory, finds the binary
// there, and execs it -- a run that installs nothing, prints nothing and
// executes the wrong tool.
func TestRun_AC5_InstalledSiblingIsNotExecuted(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, _, _ := newFixtureRunner(t, fx)
	siblingBinary := layDownTool(t, fx.Cfg, sibling, indexfixture.SharedVersion, indexfixture.CommandTwoProviders)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if execRec.binary == siblingBinary {
		t.Errorf("executed the installed sibling at %q; the project declared %q",
			siblingBinary, indexfixture.DeclaredRecipe)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want the declared recipe %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
}

// AC6. The declared provider is already there at the declared version: it is
// executed from that recipe's own version directory and nothing is installed,
// under every consent mode -- including an explicitly set suggest.
//
// The fast path returns above the elevation and above all four gates, so the
// mode genuinely cannot reach it. Pinned here so a later change to the mode
// machinery cannot alter that without a test saying so.
func TestRun_AC6_InstalledDeclaredVersionExecsUnderEveryMode(t *testing.T) {
	for _, state := range everyConsentState {
		t.Run(state.String(), func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, execRec, stdout, _ := newFixtureRunner(t, fx)
			want := layDownTool(t, fx.Cfg, indexfixture.DeclaredRecipe,
				indexfixture.SharedVersion, indexfixture.CommandTwoProviders)

			err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
				state.mode, state.origin, declaredOnly())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if execRec.binary != want {
				t.Errorf("executed %q, want the declared recipe's version directory %q",
					execRec.binary, want)
			}
			if installer.called {
				t.Errorf("installed %q, want nothing: the declared version is already there", installer.recipe)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}
		})
	}
}

// AC7. The declared provider is present at a version the project did not
// declare. That one is not executed; the declared version is installed.
func TestRun_AC7_OtherInstalledVersionIsNotExecuted(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, _, _ := newFixtureRunner(t, fx)
	const otherVersion = "9.9.9"
	stale := layDownTool(t, fx.Cfg, indexfixture.DeclaredRecipe, otherVersion, indexfixture.CommandTwoProviders)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if execRec.binary == stale {
		t.Errorf("executed %q, which is version %q; the project declared %q",
			stale, otherVersion, indexfixture.SharedVersion)
	}
	if installer.recipe != indexfixture.DeclaredRecipe || installer.ver != indexfixture.SharedVersion {
		t.Errorf("installed %s@%s, want %s@%s",
			installer.recipe, installer.ver, indexfixture.DeclaredRecipe, indexfixture.SharedVersion)
	}
}

// AC8. Both providers are present at the declared version. The declared one is
// executed.
//
// Both branches of the fast path find a binary here, so the only thing that
// separates a right answer from a wrong one is which recipe's directory was
// stated -- which is the narrowing, and nothing else.
func TestRun_AC8_DeclaredRecipeWinsOverAnInstalledSibling(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, _, _ := newFixtureRunner(t, fx)
	want := layDownTool(t, fx.Cfg, indexfixture.DeclaredRecipe,
		indexfixture.SharedVersion, indexfixture.CommandTwoProviders)
	layDownTool(t, fx.Cfg, sibling, indexfixture.SharedVersion, indexfixture.CommandTwoProviders)

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if execRec.binary != want {
		t.Errorf("executed %q, want the declared recipe's %q", execRec.binary, want)
	}
	if installer.called {
		t.Errorf("installed %q, want nothing", installer.recipe)
	}
}

// AC9. Three declared providers of one command: the refusal names all three,
// installs nothing and executes nothing.
//
// The versions and an invocation that reaches a specific one of the recipes
// belong to the refusal itself, which formats this error. What this unit owes
// is that the error can be formatted at all -- that it carries every
// declaration, and that the message it makes in the meantime tells them apart.
func TestRun_AC9_ThreeDeclaredProvidersAreAllNamed(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, execRec, _, _ := newFixtureRunner(t, fx)

	declared := map[string]string{
		indexfixture.RecipeTrioFirst:  indexfixture.SharedVersion,
		indexfixture.RecipeTrioSecond: indexfixture.SharedVersion,
		indexfixture.RecipeTrioThird:  indexfixture.SharedVersion,
	}
	err := r.Run(context.Background(), indexfixture.CommandThreeProviders, nil, ModeAuto, OriginFlag, declaring(declared))

	var ambiguous *AmbiguousDeclarationError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Run() error = %v, want an AmbiguousDeclarationError", err)
	}
	if len(ambiguous.Declarations) != len(declared) {
		t.Errorf("error carries %d declarations, want %d", len(ambiguous.Declarations), len(declared))
	}
	for recipe := range declared {
		if !strings.Contains(ambiguous.Error(), recipe) {
			t.Errorf("refusal %q does not name declared recipe %q", ambiguous.Error(), recipe)
		}
	}
	if installer.called {
		t.Errorf("installed %q, want nothing", installer.recipe)
	}
	if execRec.called {
		t.Errorf("executed %q, want nothing", execRec.binary)
	}
}

// Two registries declaring one recipe name are two declarations, and the
// refusal has to tell them apart. Recipe is a match key rather than an
// identity, so both carry the same bare name and only the configuration key
// separates them.
//
// This is the case that keeps the interim message honest. AC9's fixture
// declares bare keys, where the key and the recipe are the same string, so the
// whole message could be built from Recipe alone and AC9 would not notice.
// Here it would read "the project declares 2 recipes ... koto, koto", which is
// the confusion the declaration set exists to remove.
//
// The refusal's own criteria for this configuration are AC11b and AC14 and
// belong to the unit that formats the message. What is asserted here is only
// that the two are distinguishable at all.
func TestRun_TwoRegistriesForOneNameAreDistinguishable(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, _, _, _ := newFixtureRunner(t, fx)

	keyA := "org-a/" + indexfixture.DeclaredRecipe
	keyB := "org-b/" + indexfixture.DeclaredRecipe
	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag,
		declaring(map[string]string{
			keyA: indexfixture.SharedVersion,
			keyB: indexfixture.SharedVersion,
		}))

	var ambiguous *AmbiguousDeclarationError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Run() error = %v, want an AmbiguousDeclarationError", err)
	}
	if len(ambiguous.Declarations) != 2 {
		t.Fatalf("error carries %d declarations, want 2", len(ambiguous.Declarations))
	}
	for _, key := range []string{keyA, keyB} {
		if !strings.Contains(ambiguous.Error(), key) {
			t.Errorf("refusal %q does not name the configuration key %q; the two "+
				"declarations share a recipe name and nothing else tells them apart",
				ambiguous.Error(), key)
		}
	}
}

// outcome is everything AC10 compares between two runs.
type outcome struct {
	err              string
	stdout           string
	stderr           string
	installedRecipe  string
	installedVersion string
	execBinary       string
}

// Whether a terminal is attached, named at the call sites so the comparisons
// below say which half of the criterion they are running.
const (
	withTerminal    = true
	withoutTerminal = false
)

func runOutcome(t *testing.T, fx *indexfixture.Fixture, command string, state consent, resolver ProjectDeclarationResolver, terminal bool) outcome {
	t.Helper()
	r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)
	r.IsTerminal = func() bool { return terminal }
	// Consent is answered so a run that reaches the prompt completes rather
	// than differing only in how it was cut short. Whether the prompt appeared
	// at all is still compared, through stdout.
	r.ConsentReader = strings.NewReader("y\n")

	err := r.Run(context.Background(), command, nil, state.mode, state.origin, resolver)
	got := outcome{
		stdout:           stdout.String(),
		stderr:           stderr.String(),
		installedRecipe:  installer.recipe,
		installedVersion: installer.ver,
		execBinary:       execRec.binary,
	}
	if err != nil {
		got.err = err.Error()
	}
	return got
}

// AC10. A project declaring a recipe that provides no executed command is
// indistinguishable from no .tsuku.toml at all: same error, same prompt, same
// recipe and version, same output.
//
// This is R5's requirement and it is the one the three-way branch exists for.
// A narrowing that treated zero declarations as "narrow to nothing" would turn
// every undeclared command into ErrNoMatch; one that treated it as "narrow to
// the first" would switch off the conflict gate, which is why the
// two-provider command is compared here and not only the single-provider one.
func TestRun_AC10_UnknownRecipeConfigMatchesNoConfigAtAll(t *testing.T) {
	const unknownRecipe = "fixture-absent-from-every-index"

	for _, command := range []string{indexfixture.CommandTwoProviders, indexfixture.CommandOneProvider} {
		for _, state := range everyConsentState {
			t.Run(command+"/"+state.String(), func(t *testing.T) {
				fx := indexfixture.New(t)
				withConfig := runOutcome(t, fx, command, state,
					declaring(map[string]string{unknownRecipe: "1.0.0"}), withTerminal)
				noConfig := runOutcome(t, fx, command, state, project.NewResolver(nil), withTerminal)

				if withConfig != noConfig {
					t.Errorf("a config declaring only %q changed the run.\nwith config: %+v\nno config:   %+v",
						unknownRecipe, withConfig, noConfig)
				}
			})
		}
	}
}

// R5's passthrough as its own claim, because AC10's comparison would hold if
// both sides broke together.
//
// An undeclared command keeps every provider the index found, which is
// observable because the conflict gate counts them: under auto, two providers
// lower the mode to confirm. Narrowing the zero-declaration case to a single
// recipe would switch that gate off and install without asking.
func TestRun_UndeclaredCommandKeepsEveryProvider(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, _ := newFixtureRunner(t, fx)
	r.ConsentReader = strings.NewReader("") // consent unavailable

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag,
		declaring(map[string]string{"fixture-absent-from-every-index": "1.0.0"}))

	if !errors.Is(err, ErrUserDeclined) {
		t.Fatalf("Run() error = %v, want ErrUserDeclined: two providers should lower auto to confirm", err)
	}
	if installer.called {
		t.Errorf("installed %q without consent", installer.recipe)
	}
}

// AC19. The verification gate is asked about the recipe that will actually be
// installed, not about the sibling the index ranked first.
//
// The subject is which recipe the gate is asked about, not what it answers, so
// the verdict is fixed and the name recorded. A narrowing that reaches the
// installer but not the gates passes every other case here: the right recipe
// is installed, and the wrong one decided whether that needed a prompt.
func TestRun_AC19_VerificationGateAsksAboutTheDeclaredRecipe(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, _ := newFixtureRunner(t, fx)

	var asked []string
	r.RecipeHasVerification = func(recipe string) bool {
		asked = append(asked, recipe)
		return true
	}

	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag, declaredOnly())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(asked) != 1 {
		t.Fatalf("verification gate was asked about %v, want exactly one recipe", asked)
	}
	if asked[0] != indexfixture.DeclaredRecipe {
		t.Errorf("verification gate was asked about %q, want the declared recipe %q "+
			"(%q is the sibling the index ranks first)", asked[0], indexfixture.DeclaredRecipe, sibling)
	}
	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want %q", installer.recipe, indexfixture.DeclaredRecipe)
	}
}

// AC44. The single-provider case: the recipe and version chosen, whether a
// prompt appears, and what Run returns, across every consent state, declared
// and not, installed and not.
//
// The undeclared rows are unchanged and are what AC44 asks about. The declared
// rows are the elevation itself and this is where its whole shape is visible
// at once -- one row raised, three honored -- which is why they are read
// against the origin rather than against the mode alone.
//
// The undeclared-and-installed corner is not here. It turns on the index's own
// Installed flag rather than on a version directory, the fixture's single
// provider is not recorded as installed, and nothing exported can change that;
// TestRun_AlreadyInstalled_ExecImmediately covers it and was not modified by
// this work, which is the other half of what AC44 asks.
func TestRun_AC44_SingleProviderBehavior(t *testing.T) {
	tests := []struct {
		name        string
		state       consent
		declared    bool
		laidDown    bool
		wantErr     error
		wantPrompt  bool
		wantInstall bool
	}{
		// Declared, with nothing configured: the declaration raises the unset
		// default and the install is silent. An already-present declared
		// version execs above the mode entirely.
		{"declared/default", consent{ModeConfirm, OriginDefault}, true, false, nil, false, true},
		{"declared/default/present", consent{ModeConfirm, OriginDefault}, true, true, nil, false, false},

		// Declared, with a mode set: it is honored as given. These are the
		// bounded half of the elevation. The first two read the opposite way
		// before it -- a declaration raised a set suggest and a set confirm to
		// auto -- and the third is here because a rule that stopped raising
		// anything at all would pass the first two.
		{"declared/suggest", consent{ModeSuggest, OriginFlag}, true, false, ErrSuggestOnly, false, false},
		{"declared/confirm", consent{ModeConfirm, OriginFlag}, true, false, nil, true, true},
		{"declared/auto", consent{ModeAuto, OriginFlag}, true, false, nil, false, true},

		// Undeclared: the consent mode is honored as given.
		{"undeclared/suggest", consent{ModeSuggest, OriginFlag}, false, false, ErrSuggestOnly, false, false},
		{"undeclared/confirm", consent{ModeConfirm, OriginFlag}, false, false, nil, true, true},
		{"undeclared/auto", consent{ModeAuto, OriginFlag}, false, false, nil, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := indexfixture.New(t)
			r, installer, execRec, stdout, _ := newFixtureRunner(t, fx)
			r.ConsentReader = strings.NewReader("y\n")
			if tt.laidDown {
				layDownTool(t, fx.Cfg, indexfixture.RecipeSolo,
					indexfixture.SharedVersion, indexfixture.CommandOneProvider)
			}

			resolver := project.NewResolver(nil)
			wantVersion := ""
			if tt.declared {
				resolver = declaring(map[string]string{
					indexfixture.RecipeSolo: indexfixture.SharedVersion,
				})
				wantVersion = indexfixture.SharedVersion
			}

			err := r.Run(context.Background(), indexfixture.CommandOneProvider, nil,
				tt.state.mode, tt.state.origin, resolver)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Run() error = %v, want %v", err, tt.wantErr)
			}
			if got := promptShown(stdout.String()); got != tt.wantPrompt {
				t.Errorf("prompt appeared = %v, want %v (stdout %q)", got, tt.wantPrompt, stdout.String())
			}
			if installer.called != tt.wantInstall {
				t.Fatalf("installed = %v, want %v", installer.called, tt.wantInstall)
			}
			if !tt.wantInstall {
				return
			}
			if installer.recipe != indexfixture.RecipeSolo {
				t.Errorf("installed %q, want %q", installer.recipe, indexfixture.RecipeSolo)
			}
			if installer.ver != wantVersion {
				t.Errorf("installed version %q, want %q", installer.ver, wantVersion)
			}
			if !execRec.called {
				t.Error("exec was not called after the install")
			}
		})
	}
}

// The case that pins candidates' non-empty postcondition. Nothing else can:
// the production resolver cannot produce a declaration naming a recipe none of
// the matches provide, so only a deliberately broken one reaches the guard.
func TestCandidates_DeclarationNamingNoMatchIsAnError(t *testing.T) {
	fx := indexfixture.New(t)
	r, _, _, _, _ := newFixtureRunner(t, fx)

	matches, declaration, err := r.candidates(context.Background(), indexfixture.CommandOneProvider, &brokenResolver{})
	if err == nil {
		t.Fatalf("candidates() = %v, %+v, nil; want an error", matches, declaration)
	}
	if matches != nil || declaration != nil {
		t.Errorf("candidates() returned %v, %+v alongside its error; want nothing", matches, declaration)
	}
}

// brokenResolver declares a recipe that provides nothing it was asked about.
type brokenResolver struct{}

func (b *brokenResolver) DeclarationsFor(context.Context, []index.BinaryMatch) ([]project.ProjectDeclaration, error) {
	return []project.ProjectDeclaration{{
		Recipe:    "fixture-provides-nothing",
		Version:   indexfixture.SharedVersion,
		ConfigKey: "fixture-provides-nothing",
	}}, nil
}

// A resolver error stops the run rather than being read as "nothing declared",
// which would silently install whatever the index ranked first.
func TestCandidates_ResolverErrorStopsTheRun(t *testing.T) {
	fx := indexfixture.New(t)
	r, installer, _, _, _ := newFixtureRunner(t, fx)

	sentinel := errors.New("config unreadable")
	err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil, ModeAuto, OriginFlag,
		&mockDeclarationResolver{err: sentinel})

	if !errors.Is(err, sentinel) {
		t.Fatalf("Run() error = %v, want it to wrap %v", err, sentinel)
	}
	if installer.called {
		t.Errorf("installed %q despite the resolver failing", installer.recipe)
	}
}

// The lookup's own failure modes stay above the declaration branch, on the raw
// list: an index that was never built is a lookup failure whatever the project
// declares, and a stale one still yields usable results.
func TestCandidates_LookupFailuresPrecedeTheDeclarationBranch(t *testing.T) {
	fx := indexfixture.New(t)

	t.Run("index not built", func(t *testing.T) {
		r, _, _, _, stderr := newFixtureRunner(t, fx)
		r.Lookup = func(context.Context, string) ([]index.BinaryMatch, error) {
			return nil, index.ErrIndexNotBuilt
		}
		_, _, err := r.candidates(context.Background(), indexfixture.CommandTwoProviders, declaredOnly())
		if !errors.Is(err, ErrIndexNotBuilt) {
			t.Fatalf("candidates() error = %v, want ErrIndexNotBuilt", err)
		}
		if !strings.Contains(stderr.String(), "update-registry") {
			t.Errorf("stderr = %q, want it to name update-registry", stderr.String())
		}
	})

	t.Run("stale index still narrows", func(t *testing.T) {
		r, _, _, _, stderr := newFixtureRunner(t, fx)
		r.Lookup = func(ctx context.Context, command string) ([]index.BinaryMatch, error) {
			matches, err := fx.Lookup(ctx, command)
			if err != nil {
				return nil, err
			}
			return matches, index.StaleIndexWarning{}
		}
		matches, declaration, err := r.candidates(context.Background(),
			indexfixture.CommandTwoProviders, declaredOnly())
		if err != nil {
			t.Fatalf("candidates() error = %v, want the stale warning to be survivable", err)
		}
		if declaration == nil || declaration.Recipe != indexfixture.DeclaredRecipe {
			t.Errorf("declaration = %+v, want %q", declaration, indexfixture.DeclaredRecipe)
		}
		if len(matches) != 1 {
			t.Errorf("matches = %v, want the list narrowed to one", matches)
		}
		if !strings.Contains(stderr.String(), "Warning") {
			t.Errorf("stderr = %q, want the stale-index warning", stderr.String())
		}
	})

	t.Run("no providers", func(t *testing.T) {
		r, _, _, _, _ := newFixtureRunner(t, fx)
		_, _, err := r.candidates(context.Background(), "fixture-command-nothing-provides", declaredOnly())
		if !errors.Is(err, ErrNoMatch) {
			t.Fatalf("candidates() error = %v, want ErrNoMatch", err)
		}
	})
}
