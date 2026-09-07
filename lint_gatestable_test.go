package main_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
)

// AC52a. The mode-lowering gates registered in the code and the gates recorded
// in the design are the same set, checked mechanically.
//
// This is the comparison that discriminates, and AC52 is not. AC52 compares
// the recorded derivation against the table recorded beside it -- two
// documents, one author, one sitting -- which R21 already concedes catches a
// transcription slip and nothing else. A gate deleted from the code is a
// change both documents would happily agree about, because neither of them is
// the code.
//
// It matters most because of what the derivation does elsewhere: three gates
// collapse to one *site*, since a gate decides whether the mode is lowered and
// lowerMode is what lowers it. That collapse is right, and it is also exactly
// what would hide a gate disappearing. So the gates get a comparison of their
// own, against the one artifact that cannot be wrong about them.

const designRecordPath = "docs/designs/DESIGN-autoinstall-mode-resolution.md"

// gatesRecordHeading opens the recorded table. It is matched as a line prefix,
// which is specific enough that a section renamed without this check being
// updated fails loudly rather than matching some other table further down.
const gatesRecordHeading = "**The registered gates, by the identifier each announces itself with**,"

func TestGatesTableMatchesTheRecord(t *testing.T) {
	recorded, err := recordedGateIdentifiers()
	if err != nil {
		t.Fatalf("reading the recorded gates table: %v\n\n"+
			"This check compares %s against autoinstall.GateIdentifiers(). It cannot pass by "+
			"finding nothing: a record it cannot locate is a record nobody is checking.",
			err, designRecordPath)
	}

	registered := append([]string(nil), autoinstall.GateIdentifiers()...)
	sort.Strings(registered)
	sort.Strings(recorded)

	if strings.Join(registered, ",") == strings.Join(recorded, ",") {
		return
	}

	// Both directions, named, because they are different mistakes with
	// different fixes.
	missing := setDifference(registered, recorded)
	extra := setDifference(recorded, registered)
	t.Errorf("the gates registered in internal/autoinstall and the gates recorded in %s are not the "+
		"same set.\nregistered: %v\nrecorded:   %v\n\n"+
		"registered but not recorded: %v -- a gate was added to modeGates and the design was not "+
		"told, so the derivation now describes a smaller set of gates than the code has.\n"+
		"recorded but not registered: %v -- a gate left modeGates while its row stayed, which is the "+
		"case the site list cannot catch: three gates share one site, so deleting one moves no site "+
		"at all.",
		designRecordPath, registered, recorded, missing, extra)
}

// recordedGateIdentifiers reads the identifiers out of the markdown table that
// follows the recorded heading.
//
// It shares markdownTableAfter with the derivation check next door: one reader
// for two tables in one section, and columns taken by their headings rather
// than by position, so reordering them is an edit rather than a silent change
// of meaning.
//
// It returns an error rather than an empty list for every way of finding
// nothing, because a silent empty result would make the comparison above
// vacuously true -- which is the failure mode a check like this actually has.
func recordedGateIdentifiers() ([]string, error) {
	section, err := derivationSection()
	if err != nil {
		return nil, err
	}
	rows, err := markdownTableAfter(section, gatesRecordHeading)
	if err != nil {
		return nil, err
	}

	var found []string
	for _, row := range rows {
		identifier := backticked(row["identifier"])
		if identifier == "" {
			return nil, fmt.Errorf("the row %q records no identifier; a gate that does not announce "+
				"itself under a name cannot be compared with one that does", row["gate"])
		}
		found = append(found, identifier)
	}
	return found, nil
}

// setDifference returns the members of a that are not in b.
func setDifference(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	out := []string{}
	for _, s := range a {
		if !inB[s] {
			out = append(out, s)
		}
	}
	return out
}
