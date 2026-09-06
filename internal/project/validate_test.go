package project

import (
	"strings"
	"testing"
)

// TestValidateKey_Refusals. Each case names the wrong implementation it rules
// out, because a fixture that only demonstrates the mechanism is passed by the
// wrong implementation as readily as the right one.
func TestValidateKey_Refusals(t *testing.T) {
	cases := []struct{ name, key, rulesOut string }{
		{"traversal", "../../../../tmp/evil",
			"the baseline: nothing rejects this today"},
		{"traversal mid-key", "a/../../b",
			"an implementation that only rejects the literal string \"..\""},
		{"traversal alone", "..",
			"filepath.Clean(name) == name, which holds for \"..\""},
		{"sibling prefix", "../tools-evil/x",
			"a containment check using a string prefix: tools-evil prefixes tools"},
		{"command substitution", "x$(id)y",
			"the existing blocklist of / \\ .. NUL, which accepts this verbatim"},
		{"backtick", "a`id`b", "the same blocklist"},
		{"colon", "a:b",
			"EVERY denylist. The colon is the PATH separator, so <tools>/a:b-1.0/bin " +
				"reaches the shell as two entries and the second is relative"},
		{"leading dash", "-rf", "a rule with no flag-injection prefix check"},
		{"uppercase", "UPPER", "a case-insensitive rule"},
		{"null byte", "go\x00evil", "a regex anchored with $ rather than \\z"},
		{"version in a bare key", "jq@2.0.0",
			"nothing today; recorded as a deliberate asymmetry, since the org form " +
				"strips @version and the bare form does not"},
		// The org-split cases. These never yield a bare name at all, so an
		// implementation that refuses only on "the derived name was invalid"
		// lets every one of them through.
		{"org split fails", "../../../../example-nonexistent-owner/reg/main/tool",
			"an implementation that discards SplitOrgKey's error -- which is " +
				"exactly what the splitter's only current caller does"},
		{"org source traversal", "evil/../../../tmp/x:tool",
			"validating only the bare name; the source half reaches path " +
				"construction downstream"},
		{"org source metacharacter", "ow$(id)ner/repo:jq",
			"validating only the bare name. This one's bare name (jq) is " +
				"impeccable and its source contains no '..', so it survives both " +
				"the split and any bare-name check"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateKey(tc.key); err == nil {
				t.Errorf("validateKey(%q) = nil, want an error.\nrules out: %s", tc.key, tc.rulesOut)
			}
		})
	}
}

// TestValidateKey_Accepts is the guard against over-rejection, which no attack
// fixture detects: a validator that refuses everything passes every refusal
// test above.
func TestValidateKey_Accepts(t *testing.T) {
	cases := []struct{ name, key, rulesOut string }{
		{"plain", "jq", ""},
		{"dot", "llama.cpp", "a rule barring '.'; a real recipe, and the only " +
			"one of 1449 with a dot"},
		{"underscore", "hdrhistogram_c", "a rule allowing only [a-z0-9.-]"},
		{"leading digit", "7zip", "^[a-z][a-z0-9._-]*$, which no registry name would catch"},
		{"internal double dot", "foo..bar", "a substring test for '..'"},
		{"org scoped", "tsukumogami/koto",
			"validating the raw key, which would refuse every org-scoped declaration"},
		{"org scoped with tool", "tsukumogami/registry:mytool",
			"a rule that does not handle the :tool suffix"},
		{"uppercase org owner", "BurntSushi/toml",
			"applying the lowercase bare-name rule to the source half -- which " +
				"the one-definition principle actively invites, and which would " +
				"refuse the TOML library this repository depends on"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateKey(tc.key); err != nil {
				msg := "validateKey(%q) = %v, want nil"
				if tc.rulesOut != "" {
					msg += "\nrules out: " + tc.rulesOut
				}
				t.Errorf(msg, tc.key, err)
			}
		})
	}
}

func TestValidateDeclarations_VersionIsCheckedToo(t *testing.T) {
	// A perfectly ordinary name with a traversal in the version. No name rule
	// reaches this, which is why a name-only fix leaves the identical hole
	// through the other half of the same composition.
	tools := map[string]ToolRequirement{
		"jq": {Version: "../../../../../../../tmp/evil"},
	}
	kept, diags := validateDeclarations(tools)
	if len(kept) != 0 {
		t.Errorf("a traversal in the version survived: %v", kept)
	}
	if len(diags) != 1 || !strings.Contains(diags[0], "jq") {
		t.Errorf("expected one diagnostic naming jq, got %v", diags)
	}
}

func TestValidateDeclarations_VersionAccepts(t *testing.T) {
	// The documented pin forms all have to keep working. The prerelease and
	// channel forms are named because an over-strict version rule loses
	// exactly those, and because harmonizing this rule with the lowercase
	// name rule is a plausible mistake in a change about sharing definitions.
	for _, v := range []string{"", "latest", "1.2.3", "1.2", "1.2.3-rc1", "1.2.3-RC1", "@lts", "v1.2.3"} {
		tools := map[string]ToolRequirement{"jq": {Version: v}}
		kept, diags := validateDeclarations(tools)
		if len(kept) != 1 {
			t.Errorf("version %q was refused: %v", v, diags)
			continue
		}
		if kept["jq"].Version != v {
			t.Errorf("version %q was rewritten to %q; validation must reject, never repair",
				v, kept["jq"].Version)
		}
	}
}

// TestValidateDeclarations_RefusalIsPerEntry pins the blast radius. A shared
// config with one bad declaration must keep working for everything else --
// otherwise one mistyped entry disables every tool in the repository, and an
// attacker gets a denial of service for the price of one line.
func TestValidateDeclarations_RefusalIsPerEntry(t *testing.T) {
	tools := map[string]ToolRequirement{
		"jq":     {Version: "1.7.1"},
		"x$(id)": {Version: "1.0"},
		"node":   {Version: "20.16.0"},
	}
	kept, diags := validateDeclarations(tools)

	if len(kept) != 2 {
		t.Errorf("expected the two good declarations to survive, got %v", kept)
	}
	if _, ok := kept["jq"]; !ok {
		t.Error("jq was dropped because a sibling was malformed")
	}
	if _, ok := kept["x$(id)"]; ok {
		t.Error("the malformed declaration survived")
	}
	if len(diags) != 1 {
		t.Fatalf("expected exactly one diagnostic, got %v", diags)
	}
	// Naming the key is what makes this actionable for the common case, which
	// is a typo rather than an attack.
	if !strings.Contains(diags[0], "x$(id)") {
		t.Errorf("the diagnostic did not name the offending key: %q", diags[0])
	}
}

func TestValidateDeclarations_ErrorNamesTheExpectedShape(t *testing.T) {
	for _, tc := range []struct{ key, wantSub string }{
		{"UPPER", "lowercase"},
		{"-rf", "must not start with '-'"},
		// Not "a/b" -- that is a legitimate org-scoped reference meaning owner
		// "a", repo "b". A trailing slash leaves an empty repo half, which is
		// what the owner/repo shape check is for.
		{"a/", "owner/repo"},
	} {
		_, diags := validateDeclarations(map[string]ToolRequirement{tc.key: {Version: "1.0"}})
		if len(diags) != 1 {
			t.Fatalf("key %q: expected one diagnostic, got %v", tc.key, diags)
		}
		if !strings.Contains(diags[0], tc.wantSub) {
			t.Errorf("key %q: diagnostic %q should say %q -- naming the key alone "+
				"tells a user something is wrong but not what to change",
				tc.key, diags[0], tc.wantSub)
		}
	}
}
