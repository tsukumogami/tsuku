package main

import (
	"os"

	"github.com/tsukumogami/tsuku/internal/project"
)

// loadProjectConfigReporting loads the project config and reports any refused
// declaration before handing the result back.
//
// It exists so that reporting is not a step a caller can forget. Validation at
// the config boundary refuses a malformed declaration per entry rather than
// refusing the whole file, and that choice is only safe while the refusal is
// actually seen -- an unreported refusal is the silent partial application the
// boundary exists to prevent. Binding the two calls together makes the safe
// thing the only convenient thing.
//
// Stderr rather than stdout is load-bearing: `tsuku hook-env` writes shell text
// to stdout for the shell to evaluate, and these messages quote a key that came
// from the config file.
func loadProjectConfigReporting(cwd string) (*project.ConfigResult, error) {
	result, err := project.LoadProjectConfig(cwd)
	result.FprintDiagnostics(os.Stderr)
	return result, err
}
