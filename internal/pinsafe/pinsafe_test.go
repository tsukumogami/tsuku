package pinsafe

import "testing"

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
		// harmonising this rule with the lowercase tool-name rule would be a
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

// TestValidateRequested_ColonSplitsPath records why the colon case matters,
// since "invalid character" reads like a style rule rather than a control.
func TestValidateRequested_ColonSplitsPath(t *testing.T) {
	// <tools>/jq-1.0:evil/bin is one PATH entry to Go and two to a shell. The
	// second, "evil/bin", is relative and resolves against the working
	// directory -- which for a cloned repository is a directory the attacker
	// wrote. Neither a quoter nor a containment assertion catches it: the
	// composed path really is inside the tools tree.
	if err := ValidateRequested("1.0:evil"); err == nil {
		t.Fatal("a colon in a version must be refused: it is the PATH separator, " +
			"so the composed entry splits in two and the second half is relative")
	}
}
