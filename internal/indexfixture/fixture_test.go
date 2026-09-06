package indexfixture_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// TestDeclaredRecipeRanksSecond states the property the whole fixture exists
// to carry, in one place, under a name a reader can search for.
//
// New already asserts it on every construction. This test exists anyway
// because the assertion inside New is a guard, not a statement of intent: a
// reader looking for "why is the declared recipe the omega one" needs to find
// something that says so.
func TestDeclaredRecipeRanksSecond(t *testing.T) {
	fx := indexfixture.New(t)

	ranked := fx.RecipesFor(t, indexfixture.CommandTwoProviders)
	want := []string{indexfixture.RecipeDupFirst, indexfixture.RecipeDupSecond}
	if !reflect.DeepEqual(ranked, want) {
		t.Fatalf("RecipesFor(%q) = %v, want %v", indexfixture.CommandTwoProviders, ranked, want)
	}
	if indexfixture.DeclaredRecipe != ranked[1] {
		t.Errorf("DeclaredRecipe = %q, want the second-ranked provider %q; "+
			"a declaration that agrees with the index ranking cannot distinguish "+
			"a working narrowing from one that never runs",
			indexfixture.DeclaredRecipe, ranked[1])
	}
}

// TestThreeProviderCommand covers the three-provider case R19 requires for the
// refusal, and pins the lexicographic tiebreaker among uninstalled providers.
func TestThreeProviderCommand(t *testing.T) {
	fx := indexfixture.New(t)

	ranked := fx.RecipesFor(t, indexfixture.CommandThreeProviders)
	want := []string{
		indexfixture.RecipeTrioFirst,
		indexfixture.RecipeTrioSecond,
		indexfixture.RecipeTrioThird,
	}
	if !reflect.DeepEqual(ranked, want) {
		t.Fatalf("RecipesFor(%q) = %v, want %v", indexfixture.CommandThreeProviders, ranked, want)
	}
}

// TestInstalledOutranksName separates the two halves of `installed DESC,
// recipe ASC`. The installed provider sorts *last* by name, so it can only
// rank first if the installed flag is being honored.
func TestInstalledOutranksName(t *testing.T) {
	fx := indexfixture.New(t)

	matches, err := fx.Lookup(context.Background(), indexfixture.CommandInstalledFirst)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Lookup(%q) returned %d matches, want 2", indexfixture.CommandInstalledFirst, len(matches))
	}
	if matches[0].Recipe != indexfixture.RecipeRankedInstalled || !matches[0].Installed {
		t.Errorf("matches[0] = %q installed=%v, want %q installed=true",
			matches[0].Recipe, matches[0].Installed, indexfixture.RecipeRankedInstalled)
	}
	if matches[1].Recipe != indexfixture.RecipeRankedUninstalled || matches[1].Installed {
		t.Errorf("matches[1] = %q installed=%v, want %q installed=false",
			matches[1].Recipe, matches[1].Installed, indexfixture.RecipeRankedUninstalled)
	}
	if indexfixture.RecipeRankedInstalled < indexfixture.RecipeRankedUninstalled {
		t.Errorf("the installed recipe %q sorts before the uninstalled one %q, "+
			"so this test cannot tell `installed DESC` from `recipe ASC`",
			indexfixture.RecipeRankedInstalled, indexfixture.RecipeRankedUninstalled)
	}
}

// TestSingleProviderCommand is the R18 regression shape: exactly one provider,
// so nothing about narrowing can change what is chosen.
func TestSingleProviderCommand(t *testing.T) {
	fx := indexfixture.New(t)

	ranked := fx.RecipesFor(t, indexfixture.CommandOneProvider)
	if !reflect.DeepEqual(ranked, []string{indexfixture.RecipeSolo}) {
		t.Fatalf("RecipesFor(%q) = %v, want [%q]",
			indexfixture.CommandOneProvider, ranked, indexfixture.RecipeSolo)
	}
}

// TestFixtureNamesAreNotInTheRecipeTree is R17's actual requirement: a fixture
// recipe name that also names a shipped recipe stops being a fixture the
// moment the shipped recipe changes.
func TestFixtureNamesAreNotInTheRecipeTree(t *testing.T) {
	root := repoRoot(t)
	names := []string{
		indexfixture.RecipeDupFirst,
		indexfixture.RecipeDupSecond,
		indexfixture.RecipeTrioFirst,
		indexfixture.RecipeTrioSecond,
		indexfixture.RecipeTrioThird,
		indexfixture.RecipeSolo,
		indexfixture.RecipeRankedInstalled,
		indexfixture.RecipeRankedUninstalled,
	}

	checked := 0
	for _, name := range names {
		if len(name) == 0 {
			t.Fatalf("empty fixture recipe name")
		}
		path := filepath.Join(root, "recipes", name[:1], name+".toml")
		if _, err := os.Stat(path); err == nil {
			t.Errorf("fixture recipe %q also exists in the recipe tree at %s", name, path)
		}
		checked++
	}
	if checked != len(names) {
		t.Fatalf("checked %d names, want %d", checked, len(names))
	}

	// The scan is only worth anything if the tree it looks in is populated.
	if entries, err := os.ReadDir(filepath.Join(root, "recipes")); err != nil || len(entries) == 0 {
		t.Fatalf("recipes/ directory is empty or unreadable (%v); this check is not looking at anything", err)
	}
}

// TestLatestResolvesOffline runs the production http_json provider against the
// fixture's own endpoint. "Offline" means no external network rather than no
// sockets, which is what makes `latest` resolvable here at all.
//
// The prefix half of AC18 is deliberately absent: ResolveWithinBoundary
// narrows a prefix only for providers implementing VersionLister, and
// HTTPJSONProvider implements none, so a prefix passes through verbatim. That
// needs a fixture version provider, which this issue does not build.
func TestLatestResolvesOffline(t *testing.T) {
	fx := indexfixture.New(t)

	res := version.New()
	provider, err := version.NewHTTPJSONProvider(res, fx.VersionURL(indexfixture.DeclaredRecipe), "version")
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider() error = %v", err)
	}
	info, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}
	if info.Version != indexfixture.SharedVersion {
		t.Errorf("ResolveLatest() = %q, want %q", info.Version, indexfixture.SharedVersion)
	}

	// Both providers of the two-provider command report the same version,
	// which the refusal's formatting needs.
	other, err := version.NewHTTPJSONProvider(res, fx.VersionURL(indexfixture.RecipeDupFirst), "version")
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider() error = %v", err)
	}
	otherInfo, err := other.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}
	if otherInfo.Version != info.Version {
		t.Errorf("providers of %q report versions %q and %q; they must share one",
			indexfixture.CommandTwoProviders, info.Version, otherInfo.Version)
	}
}

// TestLoaderReadsFixtureRecipes covers the property that makes the fixture
// reachable from the `tsuku install` path: main.go's highest-priority provider
// is a local provider over $TSUKU_HOME/recipes, and that is where the fixture
// writes.
func TestLoaderReadsFixtureRecipes(t *testing.T) {
	fx := indexfixture.New(t)

	r, err := fx.Loader().Get(indexfixture.DeclaredRecipe, recipe.LoaderOptions{})
	if err != nil {
		t.Fatalf("Get(%q) error = %v", indexfixture.DeclaredRecipe, err)
	}
	if r.Metadata.Name != indexfixture.DeclaredRecipe {
		t.Errorf("loaded recipe name = %q, want %q", r.Metadata.Name, indexfixture.DeclaredRecipe)
	}
	want := "bin/" + indexfixture.CommandTwoProviders
	if !reflect.DeepEqual(r.Metadata.Binaries, []string{want}) {
		t.Errorf("Metadata.Binaries = %v, want [%q]", r.Metadata.Binaries, want)
	}
	if len(r.Steps) == 0 {
		t.Fatal("loaded recipe has no steps")
	}
	cmd, _ := r.Steps[0].Params["command"].(string)
	if !strings.Contains(cmd, "{install_dir}/bin/"+indexfixture.CommandTwoProviders) {
		t.Errorf("install step does not write the command binary: %q", cmd)
	}
}

// TestWriteProjectConfig covers the declaration side of the fixture.
func TestWriteProjectConfig(t *testing.T) {
	fx := indexfixture.New(t)

	dir := t.TempDir()
	path := fx.WriteProjectConfig(t, dir, map[string]string{
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	})
	data, err := os.ReadFile(path) //nolint:gosec // path is the fixture's own temp dir
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	got := string(data)
	if !strings.Contains(got, indexfixture.DeclaredRecipe) || !strings.Contains(got, indexfixture.SharedVersion) {
		t.Errorf(".tsuku.toml does not declare the recipe at its version:\n%s", got)
	}
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test's working directory")
		}
		dir = parent
	}
}
