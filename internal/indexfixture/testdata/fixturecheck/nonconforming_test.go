// This file is the negative control for TestMultiProviderCasesUseTheFixture.
// It breaks both of that check's rules on purpose, so the check has something
// it is known to reject.
//
// It lives under testdata/ because the Go tool never builds anything there:
// the whole point is a file that would fail the check if it were real code,
// and a real file that fails the check would fail CI. The check's scanner
// descends into testdata/ deliberately, and asserts that at least one
// violation it reports came from under testdata/, so this control cannot go
// silently unread.
//
// It sits under internal/indexfixture rather than under the repository's own
// testdata/ so the scan that reaches it can start one level above a directory
// named testdata. A scan rooted inside testdata never meets a directory named
// testdata, and so could not tell a scanner that skips them from one that does.
//
// Nothing imports this package. Do not fix the violations below.
package fixturecheck

import "github.com/tsukumogami/tsuku/internal/index"

// twoRealRecipes is the construct R17 exists to forbid: two providers of one
// command, named out of the published registry rather than the fixture. The
// day the registry stops shipping this pair, a test built on it exercises
// nothing and passes while doing so.
func twoRealRecipes() []index.BinaryMatch {
	return []index.BinaryMatch{
		{Recipe: "neovim", Command: "vi"},
		{Recipe: "vim", Command: "vi"},
	}
}

// twoRealRecipesViaRebuild is the same case assembled for Rebuild instead:
// two recipe names whose declared binaries reduce to one command.
func twoRealRecipesViaRebuild() map[string][]byte {
	return map[string][]byte{
		"neovim": recipeTOML("bin/vi"),
		"vim":    recipeTOML("bin/vi"),
	}
}

// oneRecipe is here so the control also shows what the check must not flag.
func oneRecipe() []index.BinaryMatch {
	return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}
}

func recipeTOML(binaryPath string) []byte {
	return []byte("[metadata]\nname = \"test\"\nbinaries = [\"" + binaryPath + "\"]\n")
}
