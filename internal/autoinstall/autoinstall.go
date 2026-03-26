// Package autoinstall provides the install-then-exec flow used by
// `tsuku run` and (future) `tsuku exec`. It owns consent mode resolution,
// binary index lookup, installation, and process handoff via syscall.Exec.
package autoinstall

import (
	"context"
	"io"

	"github.com/tsukumogami/tsuku/internal/config"
)

// Mode controls the consent behavior for auto-install.
type Mode int

const (
	// ModeConfirm prompts the user interactively before installing.
	// This is the default mode.
	ModeConfirm Mode = iota

	// ModeSuggest prints install instructions and exits without installing.
	ModeSuggest

	// ModeAuto installs silently with audit logging.
	// Requires explicit opt-in via config and flag/env corroboration.
	ModeAuto
)

// String returns the string representation of a Mode.
func (m Mode) String() string {
	switch m {
	case ModeConfirm:
		return "confirm"
	case ModeSuggest:
		return "suggest"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseMode converts a string to a Mode. Returns ok=false for invalid strings.
func ParseMode(s string) (Mode, bool) {
	switch s {
	case "confirm":
		return ModeConfirm, true
	case "suggest":
		return ModeSuggest, true
	case "auto":
		return ModeAuto, true
	default:
		return 0, false
	}
}

// ProjectVersionResolver resolves project-pinned versions for commands.
// Implementations come from the project config package (#1680).
// Pass nil to Runner.Run to use the latest version.
type ProjectVersionResolver interface {
	// ProjectVersionFor returns the project-pinned version for a command.
	// Returns ok=false if no pin exists (use latest).
	ProjectVersionFor(ctx context.Context, command string) (version string, ok bool, err error)
}

// Runner executes the install-then-exec flow.
type Runner struct {
	cfg    *config.Config
	stdout io.Writer
	stderr io.Writer

	// ConsentReader is the source for interactive consent input.
	// Inject a bytes.Buffer or strings.Reader in tests.
	ConsentReader io.Reader
}

// NewRunner creates a Runner with the given config and I/O writers.
func NewRunner(cfg *config.Config, stdout, stderr io.Writer) *Runner {
	return &Runner{
		cfg:    cfg,
		stdout: stdout,
		stderr: stderr,
	}
}
