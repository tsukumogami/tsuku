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

	// ExitDeterministicFailed indicates deterministic generation failed
	// and LLM fallback was suppressed (--deterministic-only flag).
	ExitDeterministicFailed = 9

	// ExitAmbiguous indicates multiple ecosystem sources were found and
	// the user must specify one with --from.
	ExitAmbiguous = 10

	// ExitIndexNotBuilt indicates the binary index has not been built yet.
	// Run 'tsuku update-registry' to build it.
	ExitIndexNotBuilt = 11

	// ExitNotInteractive indicates confirm mode was used without a TTY.
	// Set TSUKU_AUTO_INSTALL_MODE or use --mode to override.
	ExitNotInteractive = 12

	// ExitUserDeclined indicates the user declined an interactive prompt.
	ExitUserDeclined = 13

	// ExitForbidden indicates an operation was blocked for security reasons
	// (e.g., running as root).
	ExitForbidden = 14

	// ExitPartialFailure indicates some tools failed while others succeeded
	// during a batch install from project configuration.
	ExitPartialFailure = 15

	// ExitCancelled indicates the operation was canceled by the user (Ctrl+C)
	ExitCancelled = 130
)

// exitWithCode exits with the specified exit code
func exitWithCode(code int) {
	os.Exit(code)
}
