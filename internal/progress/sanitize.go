package progress

import "regexp"

// ansiPattern matches ANSI/VT100 escape sequences that should be stripped from
// strings before display or logging. Three classes are matched:
//
//  1. CSI sequences: \x1b[ followed by parameter bytes (\x30-\x3F), then
//     intermediate bytes (\x20-\x2F), then a final byte (A-Za-z).
//     Examples: cursor movement (\x1b[1A), erase line (\x1b[2K), hide
//     cursor (\x1b[?25l).
//
//  2. OSC sequences: \x1b] followed by any characters up to the string
//     terminator (BEL \x07 or ST \x1b\\). Used by terminal title-setting.
//
//  3. Bare ESC bytes (\x1b) not already consumed by the above two patterns.
var ansiPattern = regexp.MustCompile(
	`\x1b\[[\x30-\x3F]*[\x20-\x2F]*[A-Za-z]` + // CSI sequences
		`|\x1b\][^\x07]*?(?:\x07|\x1b\\)` + // OSC sequences
		`|\x1b`, // bare ESC
)

// SanitizeDisplayString strips all ANSI/VT100 escape sequences from s. Use
// this before passing any recipe-sourced string to a terminal output function
// to prevent ANSI injection attacks.
func SanitizeDisplayString(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
