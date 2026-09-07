package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/install"
)

// TestFixtureRecipeInstallsOffline is the property R19 states last and the
// refusal criterion depends on: the same fixture that feeds `tsuku run` has to
// be reachable from the `tsuku install` path too. Without it, the criterion
// that installs both declared recipes and checks neither errors has no fixture
// to run against.
//
// It also demonstrates the two fixture properties an assertion on names cannot
// reach.
//
// Version resolution reaches no network, and the pass itself is the proof
// rather than an assertion about it. The fixture recipes name an unresolvable
// .invalid host as their version endpoint, and getOrGeneratePlan turns a
// version-resolution failure into a hard error whenever the constraint is
// non-empty -- which it is here. So a run that reached version *fetching*
// could not pass; the exact pin resolves through HTTPJSONProvider.
// ResolveVersion, which returns the requested string without opening a socket.
//
// The install *steps* reach no network either, but that is by construction
// rather than proved here: the only step is a run_command that writes a shell
// script. Nothing in this test would catch a step that downloaded something,
// so keep the fixture recipes to steps that cannot.
//
// And the installed binary prints the path it was invoked as, which is the
// only way "this run executed recipe X rather than its sibling" is observable
// at all: the path carries the recipe name and the version.
func TestFixtureRecipeInstallsOffline(t *testing.T) {
	fx := indexfixture.New(t)

	// The install path threads globalCtx into plan execution; main sets it and
	// tests have to.
	origCtx := globalCtx
	t.Cleanup(func() { globalCtx = origCtx })
	globalCtx = lifecycleCtx()

	// Swap the package-level loader for one reading the fixture's recipes
	// directory. main.go builds the same provider over the same directory, so
	// this is the production chain with its highest-priority entry pointed at
	// the fixture.
	origLoader := loader
	t.Cleanup(func() { loader = origLoader })
	loader = fx.Loader()

	for _, name := range []string{indexfixture.RecipeDupFirst, indexfixture.DeclaredRecipe} {
		err := installWithDependencies(globalCtx, installArgs{
			Tool:              name,
			ReqVersion:        indexfixture.SharedVersion,
			VersionConstraint: indexfixture.SharedVersion,
			IsExplicit:        true,
			Reporter:          &countingReporter{},
		}, make(map[string]bool))
		if err != nil {
			t.Fatalf("installing %s: %v", name, err)
		}
	}

	mgr := install.New(fx.Cfg)
	for _, name := range []string{indexfixture.RecipeDupFirst, indexfixture.DeclaredRecipe} {
		ts, err := mgr.GetToolState(name)
		if err != nil || ts == nil {
			t.Fatalf("GetToolState(%s) = %v, %v", name, ts, err)
		}
		if ts.ActiveVersion != indexfixture.SharedVersion {
			t.Errorf("%s active version = %q, want %q", name, ts.ActiveVersion, indexfixture.SharedVersion)
		}
	}

	// Both providers installed the same command name into their own version
	// directories, and each binary reports which one it came from.
	for _, name := range []string{indexfixture.RecipeDupFirst, indexfixture.DeclaredRecipe} {
		binary := filepath.Join(
			fx.Cfg.ToolBinDir(name, indexfixture.SharedVersion),
			indexfixture.CommandTwoProviders,
		)
		out, err := exec.Command(binary).Output() //nolint:gosec // path built from the fixture's own temp home
		if err != nil {
			t.Fatalf("running %s: %v", binary, err)
		}
		got := strings.TrimSpace(string(out))
		if got != binary {
			t.Errorf("%s printed %q, want its own path %q", binary, got, binary)
		}
		if !strings.Contains(got, name) {
			t.Errorf("binary output %q does not name the recipe that installed it (%q)", got, name)
		}
	}
}
