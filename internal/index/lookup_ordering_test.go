package index_test

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// Lookup's ordering contract is `installed DESC, recipe ASC`. Both halves are
// exercised here rather than in lookup_test.go for two reasons: the cases are
// multi-provider, so R17 requires them to come from the shared fixture, and
// the fixture imports this package, so only an external test file can use it.

// TestLookupOrdering_InstalledOutranksName pins the first half. The fixture's
// installed provider sorts *after* the uninstalled one by name, so it can only
// come back first if the installed flag is being honored. The pair this test
// replaced could not tell the two halves apart -- its installed recipe also
// sorted first alphabetically.
func TestLookupOrdering_InstalledOutranksName(t *testing.T) {
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
}

// TestLookupOrdering_LexicographicTiebreaker pins the second half against the
// three-provider command, where no provider is installed.
func TestLookupOrdering_LexicographicTiebreaker(t *testing.T) {
	fx := indexfixture.New(t)

	got := fx.RecipesFor(t, indexfixture.CommandThreeProviders)
	want := []string{
		indexfixture.RecipeTrioFirst,
		indexfixture.RecipeTrioSecond,
		indexfixture.RecipeTrioThird,
	}
	if len(got) != len(want) {
		t.Fatalf("RecipesFor(%q) = %v, want %v", indexfixture.CommandThreeProviders, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("matches[%d].Recipe = %q, want %q", i, got[i], want[i])
		}
	}
}
