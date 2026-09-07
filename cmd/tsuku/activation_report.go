package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tsukumogami/tsuku/internal/activation"
	"github.com/tsukumogami/tsuku/internal/project"
)

// activationMessages renders the reasons a .tsuku.toml could not be fully
// honored, one message per line, in the order the declarations appear on PATH.
//
// It returns the lines rather than printing them so the wording can be asserted
// without capturing a stream, and so the only thing the caller does is choose a
// stream and respect --quiet.
//
// This is a switch with a whole sentence per arm and no default arm that
// formats an unrecognized reason. It is not five phrases substituted into a
// shared template, and the difference is visible in what the sentences contain
// rather than in how they are worded: missing-files names the version to
// reinstall, bad-form and channel name the declared string, no-match names
// neither, and unreadable is not per-declaration at all. Collapsing these into
// one format string would have to drop information from three of them, which is
// a regression in a diff rather than a tidy-up.
//
// The %q here is quoting a value from .tsuku.toml for a human reading a
// terminal, which is what %q is for -- it renders control characters visibly
// instead of letting them reach the terminal raw. It is not quoting for a
// shell: nothing on this path is eval'd, unlike the export statements
// FormatExports produces on stdout.
func activationMessages(result *activation.ActivationResult) []string {
	if result == nil {
		return nil
	}

	var lines []string

	for _, u := range result.Unhonorable {
		switch u.Reason {
		case activation.ReasonNoMatch:
			lines = append(lines, fmt.Sprintf(
				"tsuku: %s is declared in .tsuku.toml, but nothing installed matches it. Run 'tsuku install %s' to add it.",
				u.Tool, u.Tool))

		case activation.ReasonBadForm:
			// Two sentences, because the two malformed halves send the
			// developer to different parts of the same line. Blaming the
			// version for a bad name is actively misleading: a key of
			// "../../../etc" with a version of "latest" has a perfectly good
			// version, and telling someone to fix it wastes their time on the
			// half that is already correct.
			if u.BadName {
				lines = append(lines, fmt.Sprintf(
					"tsuku: %q in .tsuku.toml is not a usable tool name. A name must be a single path segment, without '/', '\\' or '..'.",
					u.Tool))
				break
			}
			lines = append(lines, fmt.Sprintf(
				"tsuku: %s is declared in .tsuku.toml as %q, which is not a valid version string. Fix the declaration to activate it.",
				u.Tool, u.Declared))

		case activation.ReasonChannel:
			lines = append(lines, fmt.Sprintf(
				"tsuku: %s is declared in .tsuku.toml as %q, which is a channel pin. Activation selects among installed versions and does not resolve channels, so declare a version instead.",
				u.Tool, u.Declared))

		case activation.ReasonMissingFiles:
			lines = append(lines, fmt.Sprintf(
				"tsuku: %s %s is recorded as installed but its files are missing. Run 'tsuku install %s@%s' to restore it.",
				u.Tool, u.Version, u.Tool, u.Version))
		}
	}

	if result.Unreadable != nil {
		// One message for the whole read, naming the declarations it would have
		// resolved. This is not a per-declaration entry, so a ten-tool file
		// still produces exactly one line here.
		lines = append(lines, fmt.Sprintf(
			"tsuku: could not read installation state, so %s in .tsuku.toml %s left unactivated.",
			strings.Join(result.Unreadable.Tools, ", "),
			plural(len(result.Unreadable.Tools), "was", "were")))
	}

	return lines
}

// parseDiagnostic renders the one line an unparseable .tsuku.toml produces, or
// returns "" when err is not a parse failure.
//
// It names the file rather than the directory, because that is what the
// developer opens, and it carries the underlying cause, which is where the line
// number lives.
func parseDiagnostic(err error) string {
	var parseErr *project.ParseError
	if !errors.As(err, &parseErr) {
		return ""
	}
	return fmt.Sprintf("tsuku: %s could not be parsed, so no tools were activated: %v",
		parseErr.Path, parseErr.Err)
}

// reportParseFailure writes the parse diagnostic through printWarning, so
// --quiet suppresses it exactly as it suppresses the five reasons.
//
// Like those, it is gated on Entered: a prompt hook in a project whose file will
// not parse says its piece on arrival, not on every prompt for as long as the
// developer stays there.
func reportParseFailure(result *activation.ActivationResult, err error) {
	line := parseDiagnostic(err)
	if line == "" {
		return
	}
	if result != nil && !result.Entered {
		return
	}
	printWarning(line)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// reportActivation writes the reason messages to stderr through printWarning,
// which is what makes --quiet apply to both entry points without either of them
// knowing about the flag.
//
// Reporting is gated on Entered so a prompt hook firing on every command in a
// project says its piece once, when the developer arrives, rather than on every
// prompt. The gate reads the flag activation computed rather than re-deriving
// it, so both commands agree.
func reportActivation(result *activation.ActivationResult) {
	if result == nil || !result.Entered {
		return
	}
	for _, line := range activationMessages(result) {
		printWarning(line)
	}
}
