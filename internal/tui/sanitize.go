// Package tui provides terminal UI primitives that other packages
// reuse: an arrow-key driven single-select picker (Pick), TTY
// detection (IsAvailable), and ANSI/control-byte sanitization for
// any string we render to the terminal.
//
// The package is built on golang.org/x/term plus hand-rolled ANSI
// escape sequences. It's intentionally small (no external TUI
// framework) and mirrors the pattern in internal/progress for
// terminal handling.
package tui

import "regexp"

// ansiPattern matches ANSI/VT100 escape sequences that should be
// stripped from strings before display. Mirrors
// internal/progress/sanitize.go so the picker treats recipe-sourced
// strings (descriptions, names) the same way the spinner does.
var ansiPattern = regexp.MustCompile(
	`\x1b\[[\x30-\x3F]*[\x20-\x2F]*[A-Za-z]` +
		`|\x1b\][^\x07]*?(?:\x07|\x1b\\)` +
		`|\x1b`,
)

// SanitizeDisplayString strips all ANSI/VT100 escape sequences from s.
// Use this before rendering any externally-sourced string (recipe name,
// description) to the terminal — prevents a malicious recipe from
// repositioning the cursor or overwriting the picker frame.
func SanitizeDisplayString(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
