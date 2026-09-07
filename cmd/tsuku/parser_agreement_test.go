package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

// TestBoundaryAndSinkParsersAgree holds the two parsers for a declaration key
// to the same answer.
//
// Boundary validation is only as strong as the agreement between the parser
// that validates and the parser that consumes. `project.SplitOrgKey` decides
// what the boundary checks; `parseDistributedName` decides what the install
// path actually uses. They are separate implementations of the same format,
// and nothing but this test holds them together.
//
// The failure this prevents is specific and quiet: if the sink parser ever
// derives a different source or name from a key the boundary approved, then
// the boundary validated one string and the installer used another, and every
// check in internal/project is being applied to a value that is not the one
// reaching the sink. No existing test would notice, because each parser is
// correct on its own terms.
//
// The corpus is deliberately the hostile one rather than the documented forms.
// Two parsers written from the same spec agree on the happy path by
// construction; they diverge, if they diverge, on the shapes nobody specified
// -- an empty half, a trailing separator, several '@' or ':' characters, a
// colon before the slash. Those are the inputs worth spending a test on.
func TestBoundaryAndSinkParsersAgree(t *testing.T) {
	keys := []string{
		// Documented forms.
		"owner/repo",
		"owner/repo:tool",
		"owner/repo@1.2.3",
		"owner/repo:tool@1.2.3",
		"BurntSushi/toml",
		// Version shapes that exercise the LastIndex('@') rule.
		"owner/repo:tool@@lts",
		"owner/repo@1.2.3@4",
		"owner/repo:to@ol@1.0",
		// Separator placement.
		"owner/repo:",
		"owner/:tool",
		"/repo:tool",
		"owner/repo::tool",
		"owner:tool/repo",
		"a/b/c:tool",
		"owner/repo:tool:extra",
		// Empty and degenerate halves.
		"/",
		":",
		"@",
		"owner/repo@",
		"owner/repo:@1.0",
		// The shapes the boundary refuses; the parsers should still agree on
		// what they mean, because agreement is what makes the refusal apply to
		// the value the sink would have used.
		"owner/repo@1.0:evil",
		"owner/repo@x$(id):jq",
		"ow$(id)ner/repo:jq",
		"owner/repo:x$(id)y",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			source, bare, isOrg, err := project.SplitOrgKey(key)
			args := parseDistributedName(key)

			// Reachability first. A key the boundary rejects outright never
			// reaches the sink, so a disagreement there is not observable.
			if err != nil {
				return
			}
			// A key with no "/" is not an org-scoped reference to either
			// parser; the sink returns nil and the boundary says isOrg=false.
			if !isOrg {
				if args != nil {
					t.Errorf("boundary says not org-scoped (bare=%q) but the sink parsed it as "+
						"source=%q name=%q version=%q.\n\nThe boundary validated a bare name "+
						"while the installer will use an owner/repo coordinate.",
						bare, args.Source, args.RecipeName, args.Version)
				}
				return
			}
			if args == nil {
				t.Errorf("boundary parsed this as org-scoped (source=%q bare=%q) but the sink "+
					"parser refused it entirely.\n\nNot exploitable by itself, but the two "+
					"disagree about what this key is, which is the property this test exists "+
					"to keep true.", source, bare)
				return
			}
			if args.Source != source {
				t.Errorf("source disagreement: boundary validated %q, sink uses %q.\n\n"+
					"Every check applied to the boundary's value is being applied to a "+
					"different string than the installer composes.", source, args.Source)
			}
			if args.RecipeName != bare {
				t.Errorf("name disagreement: boundary validated %q, sink uses %q.\n\n"+
					"The name rule ran against a value the install path does not use.",
					bare, args.RecipeName)
			}
		})
	}
}
