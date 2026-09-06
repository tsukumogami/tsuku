package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateStrictName(t *testing.T) {
	// Each reject case names the wrong implementation it rules out. The set is
	// chosen so that no single wrong implementation passes all of them: in
	// particular, a denylist of shell metacharacters plus a case check --
	// which is this codebase's existing shape in three places, and therefore
	// what an implementer reaches for -- passes every entry except the colon.
	rejects := []struct {
		name   string
		value  string
		ruleOu string
	}{
		{"command_substitution", "x$(id)y", "denylist of / \\ .. NUL, which accepts this"},
		{"backtick", "a`id`b", "same"},
		{"space", "tool name", "same"},
		{"semicolon", "a;b", "same"},
		{"leading_dash", "-rf", "a rule that omits the flag-injection prefix check"},
		{"leading_dot", ".hidden", "the charset alone: '.' is a permitted character, so only the prefix rule catches this"},
		{"newline", "a\nb", "a denylist that enumerates printable metacharacters"},
		{"non_ascii", "café", "any ASCII-only denylist"},
		{"uppercase", "UPPER", "a case-insensitive pattern"},
		{"traversal_segment", "..", "a rule that only checks the charset"},
		{"slash", "a/b", "a rule that relies on the charset for the message"},
		{"backslash", `a\b`, "same"},
		{"empty", "", "a rule that forgets the empty case"},
		{"null_byte", "a\x00b", "a regex anchored with $ rather than \\z"},
		// The one that decides the rule's shape.
		{"colon", "a:b", "EVERY denylist. A colon is not a shell metacharacter, " +
			"but it is the PATH separator: <tools>/a:b-1.0/bin reaches the shell " +
			"as two entries and the second is relative to the working directory. " +
			"Neither a quoter nor a containment check reaches it."},
	}
	for _, tc := range rejects {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			if err := ValidateStrictName(tc.value); err == nil {
				t.Errorf("ValidateStrictName(%q) = nil, want an error.\nrules out: %s",
					tc.value, tc.ruleOu)
			}
		})
	}

	accepts := []struct {
		name   string
		value  string
		ruleOu string
	}{
		{"plain", "jq", ""},
		{"single_char", "n", "a rule with a minimum length"},
		{"dot", "llama.cpp", "a rule barring '.', or one allowing only [a-z0-9-]. " +
			"This is a real recipe and the only one of 1449 with a dot."},
		{"underscore", "hdrhistogram_c", "a rule allowing only [a-z0-9.-]. " +
			"The only registry name with an underscore."},
		{"leading_digit", "7zip", "^[a-z][a-z0-9._-]*$, which is the natural pattern " +
			"to write and which no registry name would catch, since none starts with a digit"},
		{"internal_double_dot", "foo..bar", "a substring test for '..' rather than " +
			"a path-segment rule. Both rules this consolidates used the substring form."},
		{"hyphen", "jpeg-turbo", ""},
	}
	for _, tc := range accepts {
		t.Run("accept_"+tc.name, func(t *testing.T) {
			if err := ValidateStrictName(tc.value); err != nil {
				msg := "ValidateStrictName(%q) = %v, want nil"
				if tc.ruleOu != "" {
					msg += "\nrules out: " + tc.ruleOu
				}
				t.Errorf(msg, tc.value, err)
			}
		})
	}
}

// TestValidateStrictName_MessagesAreSpecific pins the per-rule diagnostics.
//
// The predicate returns an error rather than a bool for this reason: a boolean
// collapses these into one verdict, and the consolidated caller's own tests
// assert on the substrings below. Losing them would weaken diagnostics in a
// change whose other half is strengthening them.
func TestValidateStrictName_MessagesAreSpecific(t *testing.T) {
	for _, tc := range []struct{ value, wantSub string }{
		{"", "must not be empty"},
		{"a\x00b", "null byte"},
		{"..", "path traversal"},
		{"a/b", "must not contain '/'"},
		{`a\b`, `must not contain '\'`},
		{"-rf", "must not start with '-'"},
		{".hidden", "must not start with '.'"},
		{"UPPER", "must match"},
	} {
		err := ValidateStrictName(tc.value)
		if err == nil {
			t.Errorf("ValidateStrictName(%q) = nil, want an error", tc.value)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("ValidateStrictName(%q) = %q, want it to mention %q",
				tc.value, err.Error(), tc.wantSub)
		}
	}
}

// TestValidateStrictName_AcceptsEveryRegistryName is the guard against the rule
// being too strict, which is the failure mode no attack fixture detects. An
// over-rejecting validator passes every "malicious input is refused" test.
func TestValidateStrictName_AcceptsEveryRegistryName(t *testing.T) {
	root := filepath.Join("..", "..", "recipes")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("recipes/ not present: %v", err)
	}

	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(path), ".toml")
		checked++
		if verr := ValidateStrictName(name); verr != nil {
			t.Errorf("registry recipe %q is rejected by the rule: %v", name, verr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking recipes/: %v", err)
	}
	if checked < 1000 {
		t.Fatalf("only checked %d recipe names; the sweep is not covering the registry", checked)
	}
	t.Logf("accepted all %d registry recipe names", checked)
}
