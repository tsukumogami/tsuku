// Package pinsafe answers one question: is this version or pin string safe to
// compose into a filesystem path?
//
// The package is named for that question rather than for its subject matter,
// and the distinction is load-bearing. Two functions called
// ValidateVersionString exist in this tree and they answer different questions.
// The one in internal/version asks whether a string is a plausible version
// token; its character set permits '/' and it never treats ".." specially, so
// it accepts "../../evil". The one in internal/install asks whether a string is
// safe to compose into a path. Neither is globally stricter -- one is stricter
// on charset, the other on path safety -- so "use the stricter one" resolves to
// whichever axis the reader already had in mind, and "charset-stricter,
// therefore safer" picks the one that passes traversals.
//
// A package called "pin" holding a path-safety check would invite exactly that
// wrong import. Reach for this package when the value is about to become part
// of a path; reach for internal/version when you are parsing or transforming a
// version token.
//
// It is a leaf package rather than a function in either caller. internal/project
// cannot import internal/install -- install already imports project, so the edge
// would close a cycle immediately.
//
// That constraint alone does not force a third package: the rule could live in
// internal/project, with install calling it in the direction that is already
// legal. It is not filed there because internal/install validates pins that never
// came from a project file at all -- `tsuku install jq@1.7` supplies one on the
// command line -- and making the CLI path import internal/project to check a CLI
// argument would assert that pin safety is a project-config concern. It is not.
// A predicate both callers need and neither owns belongs beside neither.
package pinsafe

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateRequested checks that a requested version string contains only
// expected characters and cannot steer a path construction out of its intended
// directory.
//
// The empty string is valid: it means "no pin", and callers treat it as such.
func ValidateRequested(requested string) error {
	if requested == "" {
		return nil
	}
	// Separators come before the charset loop, which would otherwise reject them
	// first and report only "invalid character". The order was the other way when
	// this moved out of internal/install, which made this branch unreachable: the
	// loop rejects '/' and '\' because they are neither letters, digits, '.', '@'
	// nor '-', so nothing downstream of it could ever see one. Reordering makes
	// the branch live and the message say what is actually wrong, matching
	// recipe.ValidateStrictName, which orders its checks the same way for the
	// same reason.
	if strings.ContainsAny(requested, `/\`) {
		return fmt.Errorf("path separator in requested version %q", requested)
	}
	// A colon in a version splits PATH exactly as one in a name does:
	// <tools>/jq-1.0:evil/bin is one entry to Go and two to a shell, with the
	// second relative. Named rather than left to the charset loop, which would
	// report it as an invalid character and read like a style rule.
	if strings.Contains(requested, ":") {
		return fmt.Errorf("PATH separator ':' in requested version %q "+
			"(the composed directory would split in two)", requested)
	}
	if strings.Contains(requested, "..") {
		return fmt.Errorf("path traversal pattern in requested version %q", requested)
	}
	for _, r := range requested {
		if unicode.IsDigit(r) || r == '.' || r == '@' || unicode.IsLetter(r) || r == '-' {
			continue
		}
		return fmt.Errorf("invalid character %q in requested version %q", string(r), requested)
	}
	return nil
}
