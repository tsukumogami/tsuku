package main

import "os"

// Exit codes for different error types.
// These enable scripts to distinguish between failure modes.
const (
	// ExitSuccess indicates successful execution
	ExitSuccess = 0

	// ExitGeneral indicates a general error
	ExitGeneral = 1

	// ExitUsage indicates invalid arguments or usage error
	ExitUsage = 2

	// ExitRecipeNotFound indicates the recipe was not found
	ExitRecipeNotFound = 3

	// ExitVersionNotFound indicates the version was not found
	ExitVersionNotFound = 4

	// ExitNetwork indicates a network error
	ExitNetwork = 5

	// ExitInstallFailed indicates installation failed
	ExitInstallFailed = 6

	// ExitVerifyFailed indicates verification failed
	ExitVerifyFailed = 7

	// ExitDependencyFailed indicates dependency resolution failed
	ExitDependencyFailed = 8
)

// exitWithCode exits with the specified exit code
func exitWithCode(code int) {
	os.Exit(code)
}
