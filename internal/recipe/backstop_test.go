package recipe

import (
	"context"
	"strings"
	"testing"
)

// TestRecipePath_RefusesUnsafeNames is a unit test at the sink's own level,
// deliberately, and the reason is worth stating.
//
// Once the config boundary refuses these names, there may be no config-driven
// route that reaches this function with one -- so an end-to-end test would be
// green whether or not the guard exists, which is a coverage lie rather than
// coverage. A sibling change found exactly that in its own containment guard:
// deleting the call site left its whole suite passing, because upstream checks
// meant no input arrived.
//
// This test is therefore the thing holding the guard up. Deleting the check in
// recipePath must turn it red. If it ever stops doing so, the guard is
// unreachable-in-production defence in depth and should be labelled as such,
// not quietly kept.
func TestRecipePath_RefusesUnsafeNames(t *testing.T) {
	for _, layout := range []string{"grouped", "flat"} {
		p := &RegistryProvider{manifest: Manifest{Layout: layout}}

		for _, name := range []string{
			"../../evil",
			"a/b",
			`a\b`,
			"..",
			"",
			"go\x00evil",
		} {
			if got := p.recipePath(name); got != "" {
				t.Errorf("layout %s: recipePath(%q) = %q, want \"\" -- this string "+
					"becomes both an HTTP path and a disk-cache write key",
					layout, name, got)
			}
		}

		// The other direction. A backstop that refuses everything is not a
		// backstop, and no reject fixture above detects that.
		if got := p.recipePath("jq"); got == "" {
			t.Errorf("layout %s: a valid name was refused", layout)
		}
	}
}

func TestRegistryProvider_GetRefusesUnsafeName(t *testing.T) {
	p := &RegistryProvider{manifest: Manifest{Layout: "grouped"}}
	_, err := p.Get(context.Background(), "../../evil")
	if err == nil {
		t.Fatal("Get accepted a traversing name")
	}
	if !strings.Contains(err.Error(), "invalid recipe name") {
		t.Errorf("error should say what was wrong, got: %v", err)
	}
	if p.Has(context.Background(), "../../evil") {
		t.Error("Has accepted a traversing name")
	}
}
