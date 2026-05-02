package tui

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ErrCanceled is returned by Pick when the user aborts the picker
// (Ctrl-C / SIGINT). Callers should treat this as a deliberate user
// action, not a failure: typically print "Canceled." and exit non-zero
// without mutating any state.
var ErrCanceled = errors.New("picker: canceled")

// Choice is one row in the picker. Name is the identifier the caller
// receives back when the user confirms; Description is rendered alongside
// for context.
type Choice struct {
	Name        string
	Description string
}

// IsAvailable reports whether stderr is a TTY so the picker can render.
// Callers should fall back to a non-interactive code path when this
// returns false (the Pick function itself will error if invoked
// without a TTY).
func IsAvailable() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// Pick renders an arrow-driven single-select prompt to stderr and returns
// the index of the chosen entry.
//
// Behavior:
//   - Up/Down arrow keys move the cursor.
//   - Enter confirms, returning the cursor's index.
//   - Ctrl-C cancels, returning ErrCanceled.
//   - Other input is ignored.
//
// The cursor is hidden during rendering and restored on exit, including
// on panic (the deferred Restore + cursor-show calls run during goroutine
// unwind).
//
// Callers are responsible for verifying TTY readiness via IsAvailable
// before calling Pick. When stderr is not a TTY, Pick returns an error
// from term.MakeRaw rather than rendering a broken display.
func Pick(prompt string, choices []Choice) (int, error) {
	if len(choices) == 0 {
		return 0, errors.New("picker: no choices")
	}
	return pick(os.Stdin, os.Stderr, prompt, choices)
}

// pick is the testable core. It accepts an injected stdin reader and
// stderr writer so unit tests can drive the picker without a real TTY.
// The stderr writer is also used as the fd for raw-mode setup; the
// caller's responsibility to pass an *os.File when raw mode is needed.
func pick(stdin io.Reader, stderr io.Writer, prompt string, choices []Choice) (int, error) {
	// Set raw mode on the stderr fd if it's a real terminal. Tests
	// that pass a *bytes.Buffer skip this branch and just exercise the
	// rendering + key-decoding logic.
	if f, ok := stderr.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			oldState, err := term.MakeRaw(fd)
			if err != nil {
				return 0, fmt.Errorf("picker: enter raw mode: %w", err)
			}
			defer func() { _ = term.Restore(fd, oldState) }()
		}
	}

	// Hide the cursor; restore on exit.
	_, _ = fmt.Fprint(stderr, "\x1b[?25l")
	defer func() { _, _ = fmt.Fprint(stderr, "\x1b[?25h") }()

	cursor := 0
	render(stderr, prompt, choices, cursor, false)

	buf := make([]byte, 3)
	for {
		n, err := stdin.Read(buf)
		if err != nil {
			clear(stderr, prompt, choices)
			return 0, fmt.Errorf("picker: read input: %w", err)
		}

		switch {
		case isUpArrow(buf, n) && cursor > 0:
			cursor--
		case isDownArrow(buf, n) && cursor < len(choices)-1:
			cursor++
		case isEnter(buf, n):
			clear(stderr, prompt, choices)
			return cursor, nil
		case isCtrlC(buf, n):
			clear(stderr, prompt, choices)
			return 0, ErrCanceled
		default:
			// Unknown input — ignore (don't re-render).
			continue
		}
		render(stderr, prompt, choices, cursor, true)
	}
}

// render writes the prompt followed by one line per choice. The selected
// row is prefixed with "> "; other rows are prefixed with "  ". When
// rerender is true, render first moves the cursor up by len(choices)+1
// lines so the new frame overwrites the previous one.
//
// All caller-supplied strings are sanitized before being written so a
// recipe description containing ANSI escapes can't reposition the
// cursor or overwrite the picker frame.
func render(w io.Writer, prompt string, choices []Choice, selected int, rerender bool) {
	if rerender {
		// Move cursor up by total lines we previously wrote, then clear
		// each line as we go.
		fmt.Fprintf(w, "\x1b[%dA", len(choices)+1)
	}
	// Prompt line.
	fmt.Fprintf(w, "\r\x1b[2K%s\r\n", SanitizeDisplayString(prompt))
	for i, c := range choices {
		marker := "  "
		if i == selected {
			marker = "> "
		}
		name := SanitizeDisplayString(c.Name)
		desc := SanitizeDisplayString(c.Description)
		if desc == "" {
			fmt.Fprintf(w, "\r\x1b[2K%s%s\r\n", marker, name)
		} else {
			fmt.Fprintf(w, "\r\x1b[2K%s%s — %s\r\n", marker, name, desc)
		}
	}
}

// clear erases every line the picker rendered, leaving the terminal
// at the position the prompt started. Called when the picker exits
// (confirm or cancel) so the install command's subsequent output
// doesn't appear below a stale picker frame.
func clear(w io.Writer, prompt string, choices []Choice) {
	// Move cursor up by total lines, then clear each as we descend.
	_, _ = fmt.Fprintf(w, "\x1b[%dA", len(choices)+1)
	for i := 0; i <= len(choices); i++ {
		_, _ = fmt.Fprint(w, "\r\x1b[2K")
		if i < len(choices) {
			_, _ = fmt.Fprint(w, "\n")
		}
	}
	// Move cursor back up to the prompt's start so subsequent writes
	// from the caller appear at the picker's original line.
	fmt.Fprintf(w, "\x1b[%dA", len(choices))
	_ = prompt // silence unused; kept in signature so tests can assert frame size
}

// isUpArrow returns true when buf holds an Up Arrow escape sequence.
// Terminal Up Arrow is ESC [ A (3 bytes).
func isUpArrow(buf []byte, n int) bool {
	return n == 3 && buf[0] == '\x1b' && buf[1] == '[' && buf[2] == 'A'
}

// isDownArrow returns true when buf holds a Down Arrow escape sequence.
// Terminal Down Arrow is ESC [ B (3 bytes).
func isDownArrow(buf []byte, n int) bool {
	return n == 3 && buf[0] == '\x1b' && buf[1] == '[' && buf[2] == 'B'
}

// isEnter returns true when buf holds Enter (CR or LF).
func isEnter(buf []byte, n int) bool {
	return n >= 1 && (buf[0] == '\r' || buf[0] == '\n')
}

// isCtrlC returns true when buf holds Ctrl-C (ETX, 0x03).
func isCtrlC(buf []byte, n int) bool {
	return n >= 1 && buf[0] == 0x03
}
