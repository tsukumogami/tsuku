package shellenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryFishSiteFailsClosed asserts that a test file which looks up fish
// also honours TSUKU_REQUIRE_FISH.
//
// This exists because the same defect happened three times on one branch. The
// gate was written in internal/shellquote, and then evalAndRead in this package
// skipped fish silently anyway, and then the hostile-values test did too --
// each a correct control wired to one caller while another went without. It is
// the same shape as the validators this package's change is about, which is
// what makes it worth a structural check rather than a fourth manual sweep.
//
// A skip is not a neutral outcome here. On a surface whose whole purpose is to
// prove emitted text is safe under fish, "fish was missing so we did not look"
// and "we looked and it was fine" produce the same green tick.
//
// Scope note: this covers this package only. internal/shellquote has its own
// requireFish helper, which is where its sites are gated.
func TestEveryFishSiteFailsClosed(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package dir: %v", err)
	}

	var checked int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") || name == "fish_gate_test.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		text := string(src)

		// Only files that actually reach for fish are in scope. A bash-only
		// LookPath is fine: bash is everywhere, and a skip there is honest.
		if !strings.Contains(text, "LookPath") || !strings.Contains(text, "fish") {
			continue
		}
		checked++
		if !strings.Contains(text, "TSUKU_REQUIRE_FISH") {
			t.Errorf("%s looks up fish but never mentions TSUKU_REQUIRE_FISH.\n\n"+
				"Under the CI gate a missing fish must fail rather than skip. A silent "+
				"skip on this surface is indistinguishable from a pass, which is the "+
				"exact thing the variable exists to prevent -- and it has already been "+
				"missed twice in this package.", name)
		}
	}

	// The walk has to reach something, or a wrong predicate makes this pass by
	// examining nothing -- which is the failure mode it is written against.
	if checked == 0 {
		t.Fatal("no fish-using test file was found in this package; the predicate above " +
			"is wrong and this check is asserting nothing")
	}
	t.Logf("checked %d fish-using test file(s) in %s", checked, mustAbs(t))
}

func mustAbs(t *testing.T) string {
	t.Helper()
	d, err := filepath.Abs(".")
	if err != nil {
		return "."
	}
	return filepath.Base(d)
}
