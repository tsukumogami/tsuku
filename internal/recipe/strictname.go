package recipe

import (
	"fmt"
	"regexp"
	"strings"
)

// strictNamePattern is the character set a recipe identifier may use:
// lowercase ASCII letters, digits, '.', '_' and '-'.
//
// It is an allowlist rather than a denylist of known-bad characters, and that
// choice is the control rather than a style preference. A denylist of '/',
// '\', '..' and NUL accepts x$(id)y, which is a reproduced attack value. A
// denylist extended to cover shell metacharacters still accepts a:b -- a colon
// is not a shell metacharacter, but it *is* the PATH separator, so
// <tools>/a:b-1.0/bin reaches the shell as two entries and the second,
// "b-1.0/bin", is relative and resolves against the working directory. Neither
// a quoter nor a containment assertion catches that one: quoting protects the
// value from the shell's word parser, not from PATH's own semantics, and the
// composed path genuinely is inside the tools tree. Only the allowlist refuses
// it.
var strictNamePattern = regexp.MustCompile(`^[a-z0-9._-]+$`)

// ValidateStrictName reports whether name is a well-formed recipe identifier
// safe to compose into a path, a URL, or shell-visible text.
//
// It returns an error rather than a bool so that per-rule messages survive:
// callers surface distinct text for a charset miss, a traversal segment and a
// leading dash, and collapsing those into one verdict would weaken exactly the
// diagnostics this rule exists to strengthen.
//
// It deliberately does NOT call IsValidRecipeName. That helper rejects ".." by
// substring, which would refuse a name like "foo..bar" -- an internal doubled
// dot is not traversal, and rejecting it is over-broad. The rule here treats
// ".." as a whole path segment instead. IsValidRecipeName is left untouched so
// the sink-level backstops that use it keep their stricter behavior, which is
// acceptable for a backstop.
func ValidateStrictName(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("name %q contains a null byte", name)
	}
	if isTraversalSegment(name) {
		return fmt.Errorf("name %q must not be a path traversal segment", name)
	}
	// Separators are checked before the charset so the message says what is
	// wrong rather than making the reader work it out from a regex. The
	// pattern below would reject these anyway; what it would not do is tell
	// you why, and these two are the most common malformed shapes.
	if strings.Contains(name, "/") {
		return fmt.Errorf("name %q must not contain '/'", name)
	}
	if strings.Contains(name, `\`) {
		return fmt.Errorf(`name %q must not contain '\'`, name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("name %q must not start with '-' (looks like a CLI flag)", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("name %q must not start with '.'", name)
	}
	if !strictNamePattern.MatchString(name) {
		return fmt.Errorf("name %q must match %s (lowercase letters, digits, '.', '_', '-')",
			name, strictNamePattern.String())
	}
	return nil
}

// isTraversalSegment reports whether name is "." or ".." as a whole path
// segment, rather than merely containing those characters.
//
// Written as a segment walk rather than a substring test because the substring
// form rejects "foo..bar", which is harmless: filepath.Clean only treats a
// complete ".." element as an ascent. Since the charset above already excludes
// separators, a valid name is a single segment and this reduces to an equality
// check in practice -- but it is written as the segment rule so it stays
// correct if the check is ever applied before separators are excluded.
func isTraversalSegment(name string) bool {
	for _, seg := range strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if seg == ".." || seg == "." {
			return true
		}
	}
	return name == ".." || name == "."
}
