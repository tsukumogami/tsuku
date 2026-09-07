package autoinstall

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// refuseRun drives Run against a configuration that declares more than one
// provider and returns what the user was shown, with the fixture it ran
// against -- a path asserted on has to be built from the same $TSUKU_HOME the
// message was built from. A run that does not refuse is a failure here rather
// than an empty string, so every assertion below is on a message that was
// actually printed.
func refuseRun(t *testing.T, command string, declared map[string]string) (*indexfixture.Fixture, string) {
	t.Helper()

	fx := indexfixture.New(t)
	r, installer, execRec, stdout, stderr := newFixtureRunner(t, fx)

	err := r.Run(context.Background(), command, nil, ModeAuto, declaring(declared))

	var ambiguous *AmbiguousDeclarationError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Run() error = %v, want an AmbiguousDeclarationError", err)
	}
	if installer.called {
		t.Errorf("installed %q, want nothing", installer.recipe)
	}
	if execRec.called {
		t.Errorf("executed %q, want nothing", execRec.binary)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want the refusal on stderr alone", stdout.String())
	}
	return fx, stderr.String()
}

// AC13. Two of a command's three providers are declared. The refusal names
// both, with the version declared for each, and does not name the third.
//
// The versions differ so that a message printing one declaration's version
// against every recipe fails. The third provider is what separates a message
// built from the declaration set from one built from the index's own list of
// providers: only the declared two made this ambiguous, and only they can be
// removed from the file to resolve it.
func TestRun_AC13_TheRefusalNamesTheDeclaredProvidersAndNoOthers(t *testing.T) {
	const (
		firstVersion = "1.0.0"
		thirdVersion = "2.3.4"
	)
	_, message := refuseRun(t, indexfixture.CommandThreeProviders, map[string]string{
		indexfixture.RecipeTrioFirst: firstVersion,
		indexfixture.RecipeTrioThird: thirdVersion,
	})

	for _, want := range []string{
		indexfixture.RecipeTrioFirst + " " + firstVersion,
		indexfixture.RecipeTrioThird + " " + thirdVersion,
	} {
		if !strings.Contains(message, want) {
			t.Errorf("refusal does not name %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, indexfixture.RecipeTrioSecond) {
		t.Errorf("refusal names %q, which the project did not declare; only the "+
			"declarations can be removed to resolve this:\n%s",
			indexfixture.RecipeTrioSecond, message)
	}
}

// AC14. Two registries declaring one recipe name: the refusal names the
// configuration key each declaration came from, and gives a complete
// invocation reaching a specific one of them.
//
// Both declarations carry the same Recipe here, so a message assembled from
// recipe names reads "fixture-dup-omega, fixture-dup-omega" and tells the
// reader nothing about which line to remove. The key is the only thing that
// separates them.
func TestRun_AC14_TheRefusalNamesTheConfigurationKeyAndAnInvocation(t *testing.T) {
	keyA := "org-a/" + indexfixture.DeclaredRecipe
	keyB := "org-b/" + indexfixture.DeclaredRecipe

	fx, message := refuseRun(t, indexfixture.CommandTwoProviders, map[string]string{
		keyA: indexfixture.SharedVersion,
		keyB: indexfixture.SharedVersion,
	})

	for _, key := range []string{keyA, keyB} {
		if !strings.Contains(message, key) {
			t.Errorf("refusal does not name the configuration key %q; both declarations "+
				"carry the recipe name %q and nothing else tells them apart:\n%s",
				key, indexfixture.DeclaredRecipe, message)
		}
		if want := "tsuku install " + key + "@" + indexfixture.SharedVersion; !strings.Contains(message, want) {
			t.Errorf("refusal does not carry the invocation %q:\n%s", want, message)
		}
	}

	binary := filepath.Join(
		fx.Cfg.ToolBinDir(indexfixture.DeclaredRecipe, indexfixture.SharedVersion),
		indexfixture.CommandTwoProviders,
	)
	if !strings.Contains(message, binary) {
		t.Errorf("refusal does not name the version-specific path %q, so nothing in it "+
			"reaches one declared recipe rather than the other:\n%s", binary, message)
	}
}

// A declaration that is not an exact pin has no version directory to name:
// which one the install lands in is decided by a resolution that has not
// happened. The invocation ends at the link the install writes instead, rather
// than at a path built from the word "latest" that no install ever creates.
func TestRun_RefusalOfAnUnpinnedDeclarationNamesNoVersionDirectory(t *testing.T) {
	fx, message := refuseRun(t, indexfixture.CommandTwoProviders, map[string]string{
		indexfixture.RecipeDupFirst: "latest",
		indexfixture.DeclaredRecipe: "latest",
	})

	for _, recipe := range []string{indexfixture.RecipeDupFirst, indexfixture.DeclaredRecipe} {
		if fabricated := recipe + "-latest"; strings.Contains(message, fabricated) {
			t.Errorf("refusal names the directory %q, which no install creates:\n%s",
				fabricated, message)
		}
		if want := "tsuku install " + recipe + "@latest"; !strings.Contains(message, want) {
			t.Errorf("refusal does not carry the invocation %q:\n%s", want, message)
		}
	}
	linked := filepath.Join(fx.Cfg.CurrentDir, indexfixture.CommandTwoProviders)
	if !strings.Contains(message, linked) {
		t.Errorf("refusal does not name %q, where the install links the command:\n%s",
			linked, message)
	}
}
