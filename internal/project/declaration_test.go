package project

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// orgKeyA and orgKeyB declare the same bare recipe from two different sources.
// They are the pair R2a is about: the index reports one bare name for both, so
// a dedup keyed on that name collapses them.
var (
	orgKeyA = "org-a/" + indexfixture.DeclaredRecipe
	orgKeyB = "org-b/" + indexfixture.DeclaredRecipe
)

// declaringResolver builds a resolver over tools, without touching the
// filesystem. configPath is what the declarations should carry.
func declaringResolver(tools map[string]string, configPath string) *Resolver {
	reqs := make(map[string]ToolRequirement, len(tools))
	for key, version := range tools {
		reqs[key] = ToolRequirement{Version: version}
	}
	cfg := &ConfigResult{
		Config: &ProjectConfig{Tools: reqs},
		Path:   configPath,
		Dir:    filepath.Dir(configPath),
	}
	return NewResolver(cfg, nil).(*Resolver)
}

// declarationsFor runs the resolver over the fixture's ranking of command.
// Going through the fixture rather than a hand-written match slice is what
// keeps the ordering property in play: for CommandTwoProviders the declared
// recipe ranks second, so a narrowing that never matches is distinguishable
// from one that works.
func declarationsFor(t *testing.T, r *Resolver, fx *indexfixture.Fixture, command string) []ProjectDeclaration {
	t.Helper()
	ctx := context.Background()
	matches, err := fx.Lookup(ctx, command)
	if err != nil {
		t.Fatalf("Lookup(%q) error = %v", command, err)
	}
	declared, err := r.DeclarationsFor(ctx, matches)
	if err != nil {
		t.Fatalf("DeclarationsFor error = %v", err)
	}
	return declared
}

// configKeys reports the key each declaration came from, in order.
func configKeys(declared []ProjectDeclaration) []string {
	keys := make([]string, 0, len(declared))
	for _, d := range declared {
		keys = append(keys, d.ConfigKey)
	}
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// AC11 and AC12. A bare key and an org-scoped key reducing to it denote one
// recipe, so they are one declaration, and the version is the bare key's.
func TestDeclarationsFor_BareAndOrgKeyForOneRecipeAreOneDeclaration(t *testing.T) {
	fx := indexfixture.New(t)
	r := declaringResolver(map[string]string{
		indexfixture.DeclaredRecipe: "3.0.0",
		orgKeyA:                     "1.0.0",
	}, "/project/.tsuku.toml")

	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)

	if len(declared) != 1 {
		t.Fatalf("declarations = %v, want exactly one: a bare key and an org key "+
			"reducing to it denote one recipe", configKeys(declared))
	}
	if declared[0].Recipe != indexfixture.DeclaredRecipe {
		t.Errorf("Recipe = %q, want %q", declared[0].Recipe, indexfixture.DeclaredRecipe)
	}
	if declared[0].Version != "3.0.0" {
		t.Errorf("Version = %q, want %q: the bare key's version wins", declared[0].Version, "3.0.0")
	}
	if declared[0].ConfigKey != indexfixture.DeclaredRecipe {
		t.Errorf("ConfigKey = %q, want the bare key %q", declared[0].ConfigKey, indexfixture.DeclaredRecipe)
	}
	if declared[0].ConfigPath != "/project/.tsuku.toml" {
		t.Errorf("ConfigPath = %q, want %q", declared[0].ConfigPath, "/project/.tsuku.toml")
	}
}

// AC11a, first half. Two org-scoped keys whose org components differ are two
// declarations even though both reduce to one bare name, so the caller reaches
// the ambiguity refusal instead of one being picked.
//
// This is the case that separates a dedup keyed on the recipe a key denotes
// from one keyed on the bare name. The bare-name version returns a single
// declaration here and passes AC11 and AC12 above unchanged.
func TestDeclarationsFor_TwoOrgSourcesAreTwoDeclarations(t *testing.T) {
	fx := indexfixture.New(t)
	r := declaringResolver(map[string]string{
		orgKeyA: "1.0.0",
		orgKeyB: "2.0.0",
	}, "/project/.tsuku.toml")

	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)

	want := []string{orgKeyA, orgKeyB}
	if !equalStrings(configKeys(declared), want) {
		t.Fatalf("declarations came from %v, want %v: two org-scoped keys whose sources "+
			"differ denote different recipes and must not collapse to one declaration",
			configKeys(declared), want)
	}
	// The refusal names a version per key, so the versions have to survive
	// separately too -- a collapse that kept both keys but one version would
	// still have lost the association this exists to preserve.
	if declared[0].Version != "1.0.0" || declared[1].Version != "2.0.0" {
		t.Errorf("versions = %q, %q, want %q, %q",
			declared[0].Version, declared[1].Version, "1.0.0", "2.0.0")
	}
	for i, d := range declared {
		if d.Recipe != indexfixture.DeclaredRecipe {
			t.Errorf("declaration %d Recipe = %q, want %q", i, d.Recipe, indexfixture.DeclaredRecipe)
		}
	}
}

// AC11a, second half. Adding a bare key alongside the two org-scoped ones does
// not resolve the ambiguity.
//
// This is the case that separates the stated rule from "the bare key wins the
// tie", which is the more dangerous wrong implementation because it looks
// principled: it passes AC11, AC12 and the first half above, and only here
// does it return one declaration where the configuration is still ambiguous
// between two different registries.
func TestDeclarationsFor_BareKeyDoesNotBreakTheTieBetweenTwoOrgSources(t *testing.T) {
	fx := indexfixture.New(t)
	r := declaringResolver(map[string]string{
		indexfixture.DeclaredRecipe: "3.0.0",
		orgKeyA:                     "1.0.0",
		orgKeyB:                     "2.0.0",
	}, "/project/.tsuku.toml")

	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)

	if len(declared) < 2 {
		t.Fatalf("declarations = %v, want more than one: a bare key does not select "+
			"between two org-scoped sources, so this configuration is still ambiguous",
			configKeys(declared))
	}
	// Every key the user wrote stands on its own, so the refusal can name all
	// of them. Collapsing the bare key into one of the org keys would have to
	// choose which, and there is no basis for the choice.
	want := []string{indexfixture.DeclaredRecipe, orgKeyA, orgKeyB}
	if !equalStrings(configKeys(declared), want) {
		t.Errorf("declarations came from %v, want %v", configKeys(declared), want)
	}
}

// Two keys denoting the same recipe from the same source are one declaration.
// SplitOrgKey strips the version suffix, so these two keys reduce to the same
// org-scoped recipe rather than to two.
func TestDeclarationsFor_SameSourceTwiceIsOneDeclaration(t *testing.T) {
	fx := indexfixture.New(t)
	r := declaringResolver(map[string]string{
		orgKeyA:            "1.0.0",
		orgKeyA + "@2.0.0": "2.0.0",
	}, "/project/.tsuku.toml")

	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)

	if len(declared) != 1 {
		t.Fatalf("declarations = %v, want exactly one: both keys denote %q from the "+
			"same source", configKeys(declared), indexfixture.DeclaredRecipe)
	}
	if declared[0].ConfigKey != orgKeyA {
		t.Errorf("ConfigKey = %q, want %q: keys denoting one recipe resolve to the "+
			"sorted-first key", declared[0].ConfigKey, orgKeyA)
	}
}

// AC17. A config naming a recipe that is in no index declares nothing, for the
// command whose name resembles that recipe and for every other command. The
// comparison is against a resolver with no config at all, which is the state
// AC17 requires the run to be indistinguishable from.
func TestDeclarationsFor_RecipeInNoIndexDeclaresNothing(t *testing.T) {
	fx := indexfixture.New(t)
	const unknownRecipe = "fixture-absent-from-every-index"

	declaring := declaringResolver(map[string]string{
		unknownRecipe: "1.0.0",
	}, "/project/.tsuku.toml")
	none := NewResolver(nil, nil).(*Resolver)

	for _, command := range []string{
		unknownRecipe, // the command whose name resembles the unknown recipe
		indexfixture.CommandTwoProviders,
		indexfixture.CommandOneProvider,
	} {
		t.Run(command, func(t *testing.T) {
			got := declarationsFor(t, declaring, fx, command)
			want := declarationsFor(t, none, fx, command)
			if len(got) != len(want) {
				t.Errorf("with the config: %v; with no config at all: %v; AC17 requires these to agree",
					configKeys(got), configKeys(want))
			}
		})
	}
}

// AC18. A version that is not an exact pin reaches the caller verbatim, and
// the recipe named is the declared one rather than the index's top-ranked
// provider.
//
// Both halves are observable here without resolving anything, which is what
// R20 requires: resolution of a non-exact version stays the installer's job,
// unchanged, and what this layer owes is that carrying the recipe identity
// does not quietly change what is carried alongside it.
func TestDeclarationsFor_NonExactVersionsAreCarriedVerbatim(t *testing.T) {
	fx := indexfixture.New(t)

	for _, version := range []string{indexfixture.LatestVersionKeyword, "1.", ""} {
		t.Run("version="+version, func(t *testing.T) {
			r := declaringResolver(map[string]string{
				indexfixture.DeclaredRecipe: version,
			}, "/project/.tsuku.toml")

			declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)
			if len(declared) != 1 {
				t.Fatalf("declarations = %v, want exactly one", configKeys(declared))
			}
			if declared[0].Version != version {
				t.Errorf("Version = %q, want %q verbatim", declared[0].Version, version)
			}
			if declared[0].Recipe != indexfixture.DeclaredRecipe {
				t.Errorf("Recipe = %q, want the declared recipe %q and not the index's "+
					"first-ranked provider", declared[0].Recipe, indexfixture.DeclaredRecipe)
			}
		})
	}
}

// Declaration order follows the index's ranking of the matches, not config map
// iteration. Both providers of CommandTwoProviders are declared here, so the
// two recipes have an order to get wrong.
func TestDeclarationsFor_FollowsIndexRanking(t *testing.T) {
	fx := indexfixture.New(t)
	ranked := fx.RecipesFor(t, indexfixture.CommandTwoProviders)

	r := declaringResolver(map[string]string{
		ranked[0]: "1.0.0",
		ranked[1]: "2.0.0",
	}, "/project/.tsuku.toml")

	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)

	got := make([]string, 0, len(declared))
	for _, d := range declared {
		got = append(got, d.Recipe)
	}
	if !equalStrings(got, ranked) {
		t.Errorf("declaration order = %v, want the index's ranking %v", got, ranked)
	}
}

// A key SplitOrgKey rejects declares nothing. Reading it as a bare name would
// let a traversal attempt become a declaration of a recipe by that name.
func TestDeclarationsFor_MalformedKeyDeclaresNothing(t *testing.T) {
	r := declaringResolver(map[string]string{
		"../../etc/passwd": "1.0.0",
	}, "/project/.tsuku.toml")

	declared, err := r.DeclarationsFor(context.Background(), []index.BinaryMatch{
		{Recipe: "passwd", Command: "passwd"},
	})
	if err != nil {
		t.Fatalf("DeclarationsFor error = %v", err)
	}
	if len(declared) != 0 {
		t.Errorf("declarations = %v, want none", configKeys(declared))
	}
}

func TestDeclarationsFor_NoConfig(t *testing.T) {
	r := NewResolver(nil, nil).(*Resolver)
	declared, err := r.DeclarationsFor(context.Background(), []index.BinaryMatch{
		{Recipe: "jq", Command: "jq"},
	})
	if err != nil {
		t.Fatalf("DeclarationsFor error = %v", err)
	}
	if len(declared) != 0 {
		t.Errorf("declarations = %v, want none for a nil config", configKeys(declared))
	}
}

// ConfigPath carries the path of the file that actually declared the tool, so
// the disclosure emitted in internal/autoinstall can name it. This is the one
// case that loads a real .tsuku.toml rather than building ConfigResult inline.
func TestDeclarationsFor_CarriesTheDeclaringFilePath(t *testing.T) {
	fx := indexfixture.New(t)

	dir := t.TempDir()
	t.Setenv(EnvCeilingPaths, filepath.Dir(dir))
	path := fx.WriteProjectConfig(t, dir, map[string]string{
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	})

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig(%q) error = %v", dir, err)
	}
	if cfg == nil {
		t.Fatalf("LoadProjectConfig(%q) found no config; it wrote one at %q", dir, path)
	}

	r := NewResolver(cfg, nil).(*Resolver)
	declared := declarationsFor(t, r, fx, indexfixture.CommandTwoProviders)
	if len(declared) != 1 {
		t.Fatalf("declarations = %v, want exactly one", configKeys(declared))
	}
	if declared[0].ConfigPath != cfg.Path {
		t.Errorf("ConfigPath = %q, want %q", declared[0].ConfigPath, cfg.Path)
	}
}
