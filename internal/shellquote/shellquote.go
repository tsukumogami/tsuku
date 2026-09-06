// Package shellquote renders arbitrary strings as shell literals that carry no
// meaning beyond their own bytes.
//
// It exists because Go's %q is not a shell quoter. %q emits a Go string
// literal: it escapes " and \ and non-printables, and leaves $ and the
// backtick alone. Inside the double quotes %q produces, both of those are
// live, so a value reaching an eval'd emitter through %q can run commands.
//
// The two dialects need different functions and one function covering both
// would be wrong for one of them. POSIX single quotes are fully literal --
// nothing inside them is special, and a single quote cannot appear at all.
// Fish's single quotes recognise two escapes, \' and \\, and treat everything
// else literally. So a value containing *consecutive* backslashes, quoted the
// POSIX way and handed to fish, comes back with one of them consumed --
// verified against fish 3.7.1: POSIX-quoting `a\\b` yields `'a\\b'`, which fish
// reads as `a\b`. A single backslash round-trips under either quoter, which is
// why a fixture using one would show no divergence and report the dialects as
// interchangeable.
package shellquote

import "strings"

// POSIX quotes s for sh, bash and zsh.
//
// The result is single-quoted, with each embedded single quote rendered as the
// close-reopen idiom '\”. Everything else -- $, backticks, backslashes,
// newlines, glob characters, whitespace -- is literal inside single quotes and
// needs no escaping. The empty string yields ”, which is required rather than
// cosmetic: an unquoted empty value disappears from the command line.
func POSIX(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Fish quotes s for fish.
//
// Fish's single quotes are not fully literal: they recognise \' and \\. Both
// therefore need escaping, and the order matters. Backslashes are escaped
// first, so that the backslash introduced when escaping a quote is not itself
// escaped a second time. Reversing these two lines produces output that is
// wrong only for values containing both characters, which is the kind of bug
// that survives a test suite whose fixtures contain neither.
func Fish(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}
