package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// isInteractive reports whether a prompt written to stderr can be answered.
//
// It is a variable rather than a function so a test can present both answers in
// process. Every consent criterion in this feature turns on it -- a missing
// terminal is never consent for a change that outlives the command -- and none
// of them can be exercised against a real tty in `go test -short`.
//
// It is the same idiom stdinIsTerminal already uses, which is deliberately not
// reused: that one belongs to `tsuku config set` and is checked at a different
// moment, and collapsing the two would make a test of one silently change the
// other.
var isInteractive = func() bool {
	return stdinIsTerminal()
}

// promptReader is the one reader every prompt reads through.
//
// Each prompt used to build its own bufio.Reader over stdin. A buffered reader
// consumes more than the line it returns, so the second prompt's reader found
// an empty stdin and the second answer was gone. That is invisible with one
// prompt and became a defect the moment a project install asked about a source
// and then asked "Proceed?": a script piping "y\nn\n" had its second answer
// swallowed, and the run proceeded on a default rather than on the answer.
//
// Sharing one reader is what makes a scripted two-answer sequence work, and it
// is what lets a test drive both prompts.
var promptReader = struct {
	r *bufio.Reader
}{}

// promptInput replaces the stream prompts read from, and returns a function
// restoring it. Tests use it; nothing in production calls it.
func promptInput(r io.Reader) func() {
	prev := promptReader.r
	promptReader.r = bufio.NewReader(r)
	return func() { promptReader.r = prev }
}

// readPromptLine reads one answer.
func readPromptLine() (string, error) {
	if promptReader.r == nil {
		promptReader.r = bufio.NewReader(os.Stdin)
	}
	return promptReader.r.ReadString('\n')
}

// askYesNo prints a prompt and reads one answer, defaulting to no.
//
// No is the default because every caller is asking about something that
// outlives the command: registering a source, overwriting a recipe. An empty
// answer, a closed stdin and a read error all mean no.
func askYesNo(prompt string) bool {
	if !isInteractive() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s (y/N) ", prompt)
	response, err := readPromptLine()
	if err != nil && response == "" {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}
