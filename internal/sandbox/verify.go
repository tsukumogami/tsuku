package sandbox

import "github.com/tsukumogami/tsuku/internal/executor"

// CheckVerification evaluates verify results against expectations.
// It checks the verify command's exit code against the expected exit code,
// then checks whether the expected pattern appears in the combined output.
//
// Returns true when:
//   - Exit code matches expectedExitCode AND pattern is empty
//   - Exit code matches expectedExitCode AND pattern is found in output
//
// Returns false when:
//   - Exit code does not match expectedExitCode
//   - Exit code matches but pattern is not found in output
//
// Delegates to executor.CheckPlanVerification for the shared implementation.
// Used by the sandbox package for post-install verification.
func CheckVerification(verifyExitCode int, output string, expectedExitCode int, pattern string) bool {
	return executor.CheckPlanVerification(verifyExitCode, output, expectedExitCode, pattern)
}
