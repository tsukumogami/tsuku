package autoinstall

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the stub Runner.Run.
// It will be replaced by the full implementation in Issue 3.
var ErrNotImplemented = errors.New("autoinstall: not implemented")

// Run executes the install-then-exec flow for a command.
//
// It looks up the command in the binary index, applies the consent mode,
// installs if needed, and hands off execution via syscall.Exec.
//
// The resolver parameter provides project-pinned versions. Pass nil to
// use the latest version from the registry.
func (r *Runner) Run(ctx context.Context, command string, args []string, mode Mode, resolver ProjectVersionResolver) error {
	return ErrNotImplemented
}
