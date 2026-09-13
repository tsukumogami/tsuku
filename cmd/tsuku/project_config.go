package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/project"
)

// loadProjectConfigReporting loads the project config and reports any refused
// declaration, or a refused file, before handing the result back.
//
// It exists so that reporting is not a step a caller can forget. Validation at
// the config boundary refuses a malformed declaration per entry rather than
// refusing the whole file, and that choice is only safe while the refusal is
// actually seen -- an unreported refusal is the silent partial application the
// boundary exists to prevent. Binding the two calls together makes the safe
// thing the only convenient thing.
//
// A refused file is printed here rather than left to the caller because
// `tsuku run` discards the load error entirely. Printing at each call site
// would reverse exactly the property this helper exists for.
//
// The line goes out unconditionally rather than through the quiet-aware
// helper: `tsuku install`'s exit code on this path means nothing without it,
// and a command that fails silently is worse than a noisy one. The prompt hook
// is where --quiet applies, and it reports through activation's own path.
//
// Stderr rather than stdout is load-bearing: `tsuku hook-env` writes shell text
// to stdout for the shell to evaluate, and these messages quote a path and a
// key that came from the config file.
func loadProjectConfigReporting(cwd string) (*project.ConfigResult, error) {
	return loadProjectConfigReportingIn(discoveryEnv(), cwd)
}

// discoveryEnv is the filesystem and identity access every config load in this
// package goes through.
//
// It is a variable rather than a constructor call at each site for one reason:
// R26 asks that the layouts the discovery rule decides be exercisable through
// the commands themselves, in process, by a test running as an unprivileged
// user with no second account. Those layouts need another user's files and
// files owned by root, which such a test cannot create -- so the metadata has
// to be presented, and the presenting has to reach a command the test invokes
// by its own entry point.
//
// It follows the idiom already in this package for stdinIsTerminal and
// exitFunc. A test overriding it restores it; the seams in internal/project and
// internal/activation are what it routes through.
var discoveryEnv = project.OSDiscoveryEnv

// loadProjectConfigReportingIn is loadProjectConfigReporting with discovery's
// filesystem and identity access supplied explicitly, so an in-process test of
// `tsuku install` or `tsuku shim install` can drive a layout it cannot create.
func loadProjectConfigReportingIn(env project.DiscoveryEnv, cwd string) (*project.ConfigResult, error) {
	result, err := project.LoadProjectConfigIn(env, cwd)

	var refused *project.RefusedError
	if errors.As(err, &refused) {
		fmt.Fprintln(os.Stderr, refusalLine(refused))
	}

	result.FprintDiagnostics(os.Stderr)
	return result, err
}

// refusalLine renders one refusal as one line.
//
// The path is quoted by RefusedError.Error, which matters more here than
// elsewhere: it is a path from a directory the invoking user may not control,
// so a control character in it would otherwise reach the terminal raw.
func refusalLine(refused *project.RefusedError) string {
	return "tsuku: " + refused.Error()
}
