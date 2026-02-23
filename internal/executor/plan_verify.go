package executor

import "strings"

// CheckPlanVerification evaluates verify results against expectations.
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
// Used by both the sandbox and validate packages to ensure consistent
// verification behavior.
func CheckPlanVerification(verifyExitCode int, output string, expectedExitCode int, pattern string) bool {
	if verifyExitCode != expectedExitCode {
		return false
	}

	if pattern == "" {
		return true
	}

	return strings.Contains(output, pattern)
}
