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

	// ExitNotInteractive indicates confirm mode was reached with no terminal
	// to prompt on. The run itself says what would get past it, which is not
	// always the same answer: --mode=auto works only where the mode-lowering
	// gates leave it at auto, and TSUKU_AUTO_INSTALL_MODE=auto does not work
	// on its own at all, because resolveMode ignores an env-supplied auto
	// unless config.toml already says auto.
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

// exitFunc is the seam tests replace to observe an exit instead of losing the
// whole test binary to os.Exit. It is never called directly -- go through
// exitWithCode, which enforces that an exit does not return.
var exitFunc = os.Exit

// exitWithCode exits with the specified exit code and never returns.
//
// The trailing panic is what keeps that guarantee true. Callers rely on it:
// several treat the statements after an exitWithCode as unreachable and would
// dereference a nil pointer if control ever continued. Making the seam a plain
// function variable would break that for real under a test override, and would
// also cost the static analysis that proves those dereferences unreachable.
// Here the seam is one level down, so exitWithCode still provably does not
// return, and a test override that tries to continue is stopped loudly rather
// than running code that was written on the assumption it could not.
func exitWithCode(code int) {
	exitFunc(code)
	panic(exitSentinel{code})
}

// exitSentinel is what a test override's return turns into. Tests recover it;
// in production exitFunc is os.Exit and this is never reached.
type exitSentinel struct{ code int }
