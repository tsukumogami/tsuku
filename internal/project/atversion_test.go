package project

import (
	"strings"
	"testing"
)

// TestValidateKey_VersionInsideOrgScopedKey covers the third component of a
// declaration key, which was reaching a path sink unvalidated.
//
// A key can carry a version: "owner/repo:tool@1.2.3". SplitOrgKey strips
// everything after the last '@' before it splits the source from the tool name,
// so that component is discarded by the splitter and was seen by neither the
// source check nor the name check. It is then promoted to the effective version
// at install time and composed into a path.
//
// This is the shape the boundary exists to make impossible: a value from the
// config file reaching a path sink because nothing was looking at that
// particular half of it. It is not a case the name rule or the source rule got
// wrong -- it is one neither was ever handed. Both of them passed
// "owner/repo@x$(id):jq" before this check existed.
func TestValidateKey_VersionInsideOrgScopedKey(t *testing.T) {
	refuse := []struct{ key, wantSub string }{
		// The PATH-separator case, spelled inside the key instead of the value.
		{"owner/repo@1.0:evil", "PATH separator"},
		{"owner/repo:jq@1.0:evil", "PATH separator"},
		// A command substitution in the version half.
		{"owner/repo@x$(id):jq", "version in key"},
		{"owner/repo:jq@x$(id)", "invalid character"},
		// A separator, which would leave the tools tree outright.
		{"owner/repo@1.0/evil", "path separator"},
	}
	for _, tc := range refuse {
		t.Run("refuse/"+tc.key, func(t *testing.T) {
			err := validateKey(tc.key)
			if err == nil {
				t.Fatalf("validateKey(%q) = nil; the version half reaches a path "+
					"sink, so it has to be checked like any other version", tc.key)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("validateKey(%q) said %q, want a message containing %q",
					tc.key, err, tc.wantSub)
			}
		})
	}

	// The accepts matter as much: a rule that refused every versioned key would
	// satisfy every assertion above while breaking a documented form.
	accept := []string{
		"owner/repo:tool@2.0.0",
		"tsukumogami/koto@1.2.3-rc1",
		"BurntSushi/toml@latest",
		"tsukumogami/koto",
		"owner/repo:tool",
	}
	for _, key := range accept {
		t.Run("accept/"+key, func(t *testing.T) {
			if err := validateKey(key); err != nil {
				t.Errorf("validateKey(%q) = %v, want nil -- this is a documented key form",
					key, err)
			}
		})
	}
}
