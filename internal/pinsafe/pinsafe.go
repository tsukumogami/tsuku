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
// It exists as a leaf because internal/project cannot import internal/install:
// the cycle is project -> install -> shellenv -> project. That is structural
// rather than a filing accident -- install depends on project -- so no amount
// of moving packages around removes it.
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
	for _, r := range requested {
		if unicode.IsDigit(r) || r == '.' || r == '@' || unicode.IsLetter(r) || r == '-' {
			continue
		}
		return fmt.Errorf("invalid character %q in requested version %q", string(r), requested)
	}
	if strings.Contains(requested, "..") {
		return fmt.Errorf("path traversal pattern in requested version %q", requested)
	}
	if strings.ContainsAny(requested, "/\\") {
		return fmt.Errorf("path separator in requested version %q", requested)
	}
	return nil
}
