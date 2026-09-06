package pinsafe

import (
	"strings"
	"testing"
)

func TestValidateRequested(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		wantErr   bool
	}{
		// Accepts. Every documented pin form has to keep working -- this
		// extraction must not narrow the rule, and R12 turns on that.
		{"empty means no pin", "", false},
		{"exact", "1.2.3", false},
		{"prefix", "1.2", false},
		{"latest", "latest", false},
		{"channel", "@lts", false},
		{"formula channel", "shellcheck@0.9", false},
		// Named because an over-strict rule loses exactly these, and because
		// harmonizing this rule with the lowercase tool-name rule would be a
		// plausible mistake in a change whose theme is one definition reused.
		{"prerelease lowercase", "1.2.3-rc1", false},
		{"prerelease uppercase", "1.2.3-RC1", false},

		// Rejects.
		{"traversal", "../../evil", true},
		{"traversal embedded", "1.0/../../evil", true},
		{"forward slash", "1.0/evil", true},
		{"backslash", `1.0\evil`, true},
		// The colon attack has a twin on the version component:
		// <tools>/jq-1.0:evil/bin splits into two PATH entries exactly as a
		// colon in the name does. The rule is an allowlist, so it already
		// refuses this -- the test pins that property rather than adding one,
		// because a delegation needs a test proving the delegate covers the case.
		{"colon", "1.0:evil", true},
		{"space", "1.0 evil", true},
		{"dollar", "1.0$(id)", true},
		{"semicolon", "1.0;id", true},
		{"null byte", "1.0\x00evil", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequested(tc.requested)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateRequested(%q) = nil, want an error", tc.requested)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateRequested(%q) = %v, want nil", tc.requested, err)
			}
		})
	}
}

// TestValidateRequestedNamesTheActualProblem pins the order the checks run in.
//
// Every rejection above is satisfied by the charset loop alone, so the table
// passes whether or not the separator and traversal branches are ever reached --
// and they were not: when this rule moved out of internal/install the loop ran
// first, and since '/' and '\' are not in the permitted set, nothing downstream
// could see one. The branch was dead and the table could not tell.
//
// What distinguishes the orders is the message. A user who typed a path gets
// told they typed a path, rather than being told character 4 was invalid and
// left to work out which character and why it matters.
func TestValidateRequestedNamesTheActualProblem(t *testing.T) {
	for _, tc := range []struct{ requested, want string }{
		{"1.0/evil", "path separator"},
		{`1.0\evil`, "path separator"},
		{"../../evil", "path separator"},
		{"1.0..evil", "path traversal"},
		// The colon has no dedicated branch and should not get one: it is
		// refused by the charset loop, and "invalid character" is the honest
		// message for it. Present so that a future edit adding a colon branch
		// has to change a test that says why there isn't one.
		{"1.0:evil", "invalid character"},
	} {
		t.Run(tc.requested, func(t *testing.T) {
			err := ValidateRequested(tc.requested)
			if err == nil {
				t.Fatalf("ValidateRequested(%q) = nil, want an error", tc.requested)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ValidateRequested(%q) said %q, want a message containing %q.\n\n"+
					"If this is reporting an invalid character where a path was expected, "+
					"the charset loop has been moved back ahead of the separator check and "+
					"that branch is unreachable again.", tc.requested, err, tc.want)
			}
		})
	}
}
