// Package indexfixture builds the binary index that every multi-provider test
// in this repository is constructed against.
//
// R17 forbids obtaining a multi-provider case from the published registry: a
// test naming two real recipes stops exercising anything the moment the
// registry stops shipping that pair, and passes while doing so. So the recipes
// here have names that appear in no published manifest, and a static check
// (TestMultiProviderCasesUseTheFixture, in the repository root) fails a test in
// internal/autoinstall, internal/project, internal/index or cmd/tsuku that
// builds a multi-provider case some other way.
//
// What that check catches is worth stating, because it is narrower than "any
// other way" and reading it as total is how a gap goes unnoticed.
//
// It catches composite literals, and only composite literals:
//
//   - a `[]index.BinaryMatch` or `[N]index.BinaryMatch` with two or more
//     elements, including one whose type is elided one level inside a
//     container -- `map[string][]index.BinaryMatch{"vi": {{...}, {...}}}`,
//     the usual shape for a command-keyed LookupFunc stub;
//   - a `map[string][]byte` where two keys hold the same recipe: read as the
//     command a value's inline TOML declares, or -- for a value whose contents
//     cannot be read -- as the helper argument or expression text two keys
//     have in common.
//
// Everything else passes. A slice built by append or in a loop, a named slice
// type, an elided literal two levels down, a recipe map assembled by statement
// rather than written as a literal, and the one most likely to be reached for
// by accident: two *different* variables that happen to hold the same recipe,
// which nothing here can see. That list is the shapes worth knowing about
// rather than a closed set, and a construct missing from it is not thereby
// sanctioned -- if you find yourself checking whether the rule will catch what
// you are about to write, that is the question answering itself. Build it here.
//
// The whole fixture is offline. "Offline" means no external network rather
// than no sockets: installation runs a run_command step that writes a shell
// script, and the local endpoint New starts serves version manifests to
// whatever asks for them. Nothing reaches the internet.
//
// # The property that makes the fixture worth having
//
// For [CommandTwoProviders], the recipe a project declares -- [DeclaredRecipe]
// -- ranks *second* in the index's own ordering, which is `installed DESC,
// recipe ASC`. That is deliberate and load-bearing. If the declared recipe
// ranked first, a narrowing that never matches anything would be
// indistinguishable from a narrowing that works, because index ranking and
// declaration would agree. New asserts the property on every construction so
// no consumer has to remember it.
package indexfixture

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/registry"
)

// Commands the fixture recipes provide. Each is a name no real tool uses, so a
// lookup that accidentally reaches the real index returns nothing rather than
// something plausible.
const (
	// CommandTwoProviders has exactly two providers, which resolve to the
	// same version string. This is the command the mode-resolution criteria
	// are run against.
	CommandTwoProviders = "fixturedup"

	// CommandThreeProviders has exactly three providers, none installed, so
	// Lookup ranks them lexicographically.
	CommandThreeProviders = "fixturetrio"

	// CommandOneProvider has exactly one provider. It is the single-provider
	// regression case: R18 freezes its behavior.
	CommandOneProvider = "fixturesolo"

	// CommandInstalledFirst has two providers, of which the one that sorts
	// *later* by name is installed. It separates the `installed DESC` half of
	// the ordering from the `recipe ASC` half; a pair where the installed
	// recipe also sorts first proves nothing.
	CommandInstalledFirst = "fixtureranked"
)

// Recipe names. The suffixes are ordering markers, not descriptions: Lookup
// ranks uninstalled providers by recipe name ascending, so alpha comes before
// mu comes before omega comes before zulu.
const (
	// RecipeDupFirst ranks first for CommandTwoProviders.
	RecipeDupFirst = "fixture-dup-alpha"

	// RecipeDupSecond ranks second for CommandTwoProviders. See DeclaredRecipe.
	RecipeDupSecond = "fixture-dup-omega"

	// RecipeTrioFirst, RecipeTrioSecond and RecipeTrioThird provide
	// CommandThreeProviders, in that rank order.
	RecipeTrioFirst  = "fixture-trio-alpha"
	RecipeTrioSecond = "fixture-trio-mu"
	RecipeTrioThird  = "fixture-trio-omega"

	// RecipeSolo is the only provider of CommandOneProvider.
	RecipeSolo = "fixture-solo"

	// RecipeRankedInstalled provides CommandInstalledFirst and is recorded as
	// installed, so it ranks first despite sorting last by name.
	//
	// "Installed" here means installed *in the index*, and nowhere else. No
	// files exist under $TSUKU_HOME/tools for it and state.json does not know
	// it, so install.Manager.GetToolState returns nil for it.
	//
	// So handing CommandInstalledFirst to autoinstall.Runner.Run needs care.
	// Run's already-installed fast path reads matches[0].Installed straight
	// off the index, so for an *undeclared* command it fires here and execs a
	// binary that was never laid down. The branch is an else-if under the
	// project-declared case, so a declared command takes the other path and
	// stats a real file instead -- which is why this only bites the undeclared
	// case.
	//
	// This pair exists to exercise Lookup's ordering, not to stand in for a
	// completed install. A unit that needs a real one writes the tool tree
	// itself, and which path depends on which branch it is aiming at: the
	// declared branch stats Cfg.ToolBinDir(<the declared recipe>, <the
	// declared version>), the undeclared one execs from Cfg.CurrentDir. Those
	// name the same recipe only once Issue 3's narrowing lands -- before it,
	// the declared branch builds its path from matches[0].Recipe.
	RecipeRankedInstalled = "fixture-ranked-zulu"

	// RecipeRankedUninstalled provides CommandInstalledFirst and is not
	// installed, so it ranks second despite sorting first by name.
	RecipeRankedUninstalled = "fixture-ranked-alpha"
)

// DeclaredRecipe is the recipe a project declaration names for
// CommandTwoProviders. It is RecipeDupSecond rather than RecipeDupFirst
// because a declaration that agrees with the index ranking cannot tell a
// working narrowing from a narrowing that never runs.
const DeclaredRecipe = RecipeDupSecond

// SharedVersion is the version every fixture recipe installs at, and the one
// a fixture .tsuku.toml declares. Both providers of CommandTwoProviders
// resolving to the same string is one of the properties the multi-provider
// criteria need: a refusal that names two recipes has to name two versions,
// and they have to be able to be equal.
//
// It resolves with no network at all. HTTPJSONProvider.ResolveVersion passes
// an exact version straight through -- the manifest only ever carries the
// latest one, so there is nothing to look a pinned version up in -- so an
// install pinned to this version never opens a socket.
const SharedVersion = "1.0.0"

// recipeVersionHost is the host the fixture recipes' [version] url names.
//
// It is deliberately unreachable. .invalid is reserved by RFC 2606 and never
// resolves, so a fixture install that somehow reached version *fetching* fails
// at once instead of leaving the machine's network as the thing that decides
// whether a test passes.
//
// It has to be https for a reason the design got wrong: recipe.ValidateRecipe
// rejects a non-HTTPS http_json url, and cmd/tsuku runs that validator on the
// install path, before any plan is generated. A recipe pointing at a plain
// http endpoint therefore cannot be installed at all.
const recipeVersionHost = "https://tsuku-fixture.invalid"

// LatestVersionKeyword is the version string a declaration uses to ask for
// whatever the provider reports.
//
// It resolves offline through the production http_json provider against the
// fixture's own endpoint -- see Fixture.VersionURL -- where "offline" means no
// external network rather than no sockets. It does *not* resolve through the
// install path, because the recipes point at recipeVersionHost there.
//
// A *prefix* version (for example "1.") does not resolve against this fixture
// at all. ResolveWithinBoundary narrows a prefix only for providers
// implementing VersionLister, and HTTPJSONProvider implements none, so the
// constraint passes through verbatim. Closing that gap needs a fixture version
// provider, which is deliberately not built here.
//
// A unit reaching for a prefix should read R20 first. R20 says resolution of a
// non-exact version stays the installer's job, unchanged, and that what a
// declaration carries is the recipe identity -- which reads as though a
// criterion about a prefix declaration can be met at the declaration layer,
// where the string is carried verbatim and never resolved. The PLAN says
// instead that AC18's prefix half needs a fixture provider or is cut. The two
// have not been reconciled; whoever picks up that criterion has to, and should
// not assume this fixture settled it.
const LatestVersionKeyword = "latest"

// Fixture is a throwaway $TSUKU_HOME holding the fixture recipes, a rebuilt
// binary index, and a local version endpoint.
type Fixture struct {
	// Cfg is the config rooted at the throwaway $TSUKU_HOME. Its RecipesDir
	// holds every fixture recipe in the flat layout NewLocalProvider reads,
	// and its RegistryDir holds the same TOMLs in the letter layout the
	// registry cache uses.
	Cfg *config.Config

	// Registry reads the fixture recipes out of Cfg.RegistryDir. It is the
	// same *registry.Registry the real rebuild uses.
	Registry *registry.Registry

	// Index is the rebuilt binary index. Prefer the Lookup method over
	// calling this directly.
	//
	// It is exported so a unit that changes installed state can rebuild. If
	// you do, re-check what New asserts: rebuilding with a different installed
	// set can move DeclaredRecipe to the top of CommandTwoProviders, which
	// silently turns every criterion resting on the ranking property into one
	// that passes either way.
	Index index.BinaryIndex

	// VersionBaseURL is the root of the local endpoint serving version JSON.
	VersionBaseURL string

	installed map[string]bool
}

// New builds the fixture: it points $TSUKU_HOME at a temp directory, writes
// every fixture recipe into both the recipes directory and the registry cache,
// starts the version endpoint, and rebuilds the index in process.
//
// It then asserts the ranking property described in the package comment, so a
// later edit to a recipe name cannot silently turn every criterion that
// depends on it into a tautology.
//
// Two consequences of the $TSUKU_HOME redirection, because it goes through
// t.Setenv:
//
//   - A test that calls New cannot call t.Parallel afterwards: t.Parallel
//     panics once t.Setenv has run. That fails loudly rather than corrupting
//     a neighbor, but it is a constraint on every consumer.
//   - Every config.DefaultConfig() after this call reads the fixture's home,
//     including one built by code that has never heard of this package. If a
//     test builds its own *config.Config as well, the two disagree about where
//     $TSUKU_HOME is, and which one a given code path reads is decided by
//     which one it was handed. Prefer f.Cfg.
func New(t *testing.T) *Fixture {
	t.Helper()

	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("indexfixture: config.DefaultConfig() error = %v", err)
	}
	if cfg.HomeDir != home {
		t.Fatalf("indexfixture: config rooted at %q, want %q; TSUKU_HOME redirection is not taking effect", cfg.HomeDir, home)
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("indexfixture: EnsureDirectories() error = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One document per recipe, all reporting SharedVersion. Per-recipe
		// paths rather than one shared path so a future test can serve a
		// different version for one provider without touching the others.
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json")
		if !recipeNames()[name] {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version": %q}`, SharedVersion)
	}))
	t.Cleanup(srv.Close)

	f := &Fixture{
		Cfg:            cfg,
		Registry:       registry.New(cfg.RegistryDir),
		VersionBaseURL: srv.URL,
		installed:      map[string]bool{RecipeRankedInstalled: true},
	}

	for name, command := range recipeCommands {
		toml := f.recipeTOML(name, command)
		f.writeRecipe(t, name, toml)
	}

	f.rebuild(t)
	f.assertDeclaredRecipeRanksSecond(t)
	return f
}

// recipeCommands maps every fixture recipe to the single command it provides.
var recipeCommands = map[string]string{
	RecipeDupFirst:          CommandTwoProviders,
	RecipeDupSecond:         CommandTwoProviders,
	RecipeTrioFirst:         CommandThreeProviders,
	RecipeTrioSecond:        CommandThreeProviders,
	RecipeTrioThird:         CommandThreeProviders,
	RecipeSolo:              CommandOneProvider,
	RecipeRankedInstalled:   CommandInstalledFirst,
	RecipeRankedUninstalled: CommandInstalledFirst,
}

func recipeNames() map[string]bool {
	names := make(map[string]bool, len(recipeCommands))
	for name := range recipeCommands {
		names[name] = true
	}
	return names
}

// RecipesFor returns the recipes providing command, in the order the index
// ranks them. It is derived from the fixture's own declarations rather than
// hardcoded, so a test that asks for "the declared recipe's rank" gets the
// answer the index would give.
func (f *Fixture) RecipesFor(t *testing.T, command string) []string {
	t.Helper()
	matches, err := f.Lookup(context.Background(), command)
	if err != nil {
		t.Fatalf("indexfixture: Lookup(%q) error = %v", command, err)
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m.Recipe)
	}
	return names
}

// Lookup has the signature of autoinstall.LookupFunc and of the lookup
// function internal/project's resolver takes, so it can be assigned to either
// without an adapter.
func (f *Fixture) Lookup(ctx context.Context, command string) ([]index.BinaryMatch, error) {
	return f.Index.Lookup(ctx, command)
}

// Loader returns a recipe loader whose only provider is the fixture's recipes
// directory. main.go makes that the highest-priority provider with the same
// flat layout, so a recipe loaded here is loaded through production code.
//
// This is what makes the fixture reachable from the `tsuku install` path as
// well as from `tsuku run`.
//
// Three conditions come with driving an install. Only the first fails
// silently, which is why it is first:
//
//  1. Pin the install to [SharedVersion]. An exact version resolves with no
//     network, but an empty or `latest` constraint sends the install to the
//     recipe's version endpoint, which is deliberately unreachable -- see
//     recipeVersionHost. An empty constraint then falls back to the "dev"
//     version with only a warning, so the install succeeds and lands somewhere
//     nobody expected; `latest` fails outright.
//
//  2. In cmd/tsuku, assign this loader to the package-level `loader` variable
//     and restore it afterwards. The install pipeline reads that variable, not
//     a loader passed in. Skipping this gives a recipe-not-found error.
//
//  3. In cmd/tsuku, set the package-level `globalCtx`. main sets it and tests
//     have to. Skipping this panics somewhere below plan generation, in a
//     stack that names neither globalCtx nor this package.
//
// See cmd/tsuku/install_fixture_test.go for the whole shape.
func (f *Fixture) Loader() *recipe.Loader {
	return recipe.NewLoader(recipe.NewLocalProvider(f.Cfg.RecipesDir))
}

// VersionURL is the fixture's local manifest endpoint for one recipe. It
// serves the document an http_json provider reads, so `latest` resolves
// through production version-resolution code with no external network.
//
// The fixture recipes do not point at it: they name recipeVersionHost, because
// the install path validates the recipe and rejects a non-HTTPS http_json url.
// A test that wants `latest` resolution builds a provider against this URL
// directly.
func (f *Fixture) VersionURL(recipeName string) string {
	return f.VersionBaseURL + "/" + recipeName + ".json"
}

// WriteProjectConfig writes a .tsuku.toml into dir declaring tools as
// recipe -> version, and returns the file's path.
func (f *Fixture) WriteProjectConfig(t *testing.T, dir string, tools map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("[tools]\n")
	for _, name := range sortedKeys(tools) {
		fmt.Fprintf(&b, "%q = %q\n", name, tools[name])
	}
	path := filepath.Join(dir, ".tsuku.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("indexfixture: writing %s: %v", path, err)
	}
	return path
}

// sortedKeys orders the declarations so the written file is deterministic --
// a test asserting on the file's text should not depend on map iteration.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// recipeTOML renders one fixture recipe.
//
// metadata.binaries is what the index reads (it takes precedence over step
// inspection) and what the installer symlinks, so declaring it there covers
// both. The install step writes a shell script that prints the path it was
// invoked as, which is how "this run executed recipe X" is observed at all:
// the path contains the recipe name and the version.
func (f *Fixture) recipeTOML(name, command string) []byte {
	// The install command is a TOML multi-line literal string: no escape
	// processing at all, which is the only way to get a shell script that
	// contains both quote kinds through a recipe file intact.
	return fmt.Appendf(nil, `[metadata]
name = %q
description = "Fixture recipe providing %s; exists only in tests"
version_format = "semver"
binaries = ["bin/%s"]

[version]
source = "http_json"
url = %q
version_path = "version"

[[steps]]
action = "run_command"
command = '''mkdir -p {install_dir}/bin && ( echo '#!/bin/sh'; echo 'echo "$0"' ) > {install_dir}/bin/%s && chmod 0755 {install_dir}/bin/%s'''

[verify]
command = "%s"
`, name, command, command, recipeVersionHost+"/"+name+".json", command, command, command)
}

// writeRecipe puts the TOML where both the recipe loader and the registry
// cache will find it: the recipes directory uses a flat layout, the registry
// cache a letter directory.
func (f *Fixture) writeRecipe(t *testing.T, name string, data []byte) {
	t.Helper()

	flat := filepath.Join(f.Cfg.RecipesDir, name+".toml")
	if err := os.WriteFile(flat, data, 0o600); err != nil {
		t.Fatalf("indexfixture: writing %s: %v", flat, err)
	}
	if err := f.Registry.CacheRecipe(name, data); err != nil {
		t.Fatalf("indexfixture: caching recipe %s: %v", name, err)
	}
}

// stateReader reports which fixture recipes count as installed. It stands in
// for the adapter cmd/tsuku builds over *install.StateManager; internal/index
// never sees install.ToolState either way.
type stateReader struct {
	installed map[string]bool
	commands  map[string]string
}

func (s *stateReader) AllTools() (map[string]index.ToolInfo, error) {
	tools := make(map[string]index.ToolInfo, len(s.installed))
	for name := range s.installed {
		tools[name] = index.ToolInfo{
			ActiveVersion: SharedVersion,
			Source:        "registry",
			Versions: map[string]index.VersionInfo{
				SharedVersion: {Binaries: []string{"bin/" + s.commands[name]}},
			},
		}
	}
	return tools, nil
}

// rebuild opens the index at the config's own path and rebuilds it from the
// registry cache. No network, no `tsuku update-registry`: GetCached and
// ListCached are plain reads of the cache directory.
func (f *Fixture) rebuild(t *testing.T) {
	t.Helper()

	idx, err := index.Open(f.Cfg.BinaryIndexPath(), "")
	if err != nil {
		t.Fatalf("indexfixture: index.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	state := &stateReader{installed: f.installed, commands: recipeCommands}
	if err := idx.Rebuild(context.Background(), f.Registry, state); err != nil {
		t.Fatalf("indexfixture: Rebuild() error = %v", err)
	}
	f.Index = idx
}

// assertDeclaredRecipeRanksSecond fails the construction if the property the
// package comment describes has stopped holding. It runs on every New rather
// than in one test because every consumer depends on it and none of them
// restate it.
func (f *Fixture) assertDeclaredRecipeRanksSecond(t *testing.T) {
	t.Helper()

	ranked := f.RecipesFor(t, CommandTwoProviders)
	if len(ranked) != 2 {
		t.Fatalf("indexfixture: %q has %d providers %v, want exactly 2",
			CommandTwoProviders, len(ranked), ranked)
	}
	// With exactly two providers, "ranks second" and "is not first" are the
	// same claim, and "provides the command at all" falls out of it too.
	if ranked[1] != DeclaredRecipe {
		t.Fatalf("indexfixture: %q ranks %v for %q, so the declared recipe %q is not second. "+
			"It must rank second or later, or a narrowing that never matches anything passes "+
			"every criterion that uses this fixture",
			CommandTwoProviders, ranked, CommandTwoProviders, DeclaredRecipe)
	}
}
