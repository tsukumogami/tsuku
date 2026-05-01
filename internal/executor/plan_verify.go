package executor

import "strings"

// CheckPlanVerification evaluates verify results against expectations.
// It checks the verify command's exit code against the expected exit code,
// then checks whether every entry in patterns appears as a substring in
// the combined output (AND-semantics, matching VerifySection's two-field
// schema where pattern and patterns are mutually exclusive but both feed
// the same matcher here).
//
// Returns true when:
//   - Exit code matches expectedExitCode AND patterns is empty
//   - Exit code matches expectedExitCode AND every entry in patterns
//     is found in output
//
// Returns false when:
//   - Exit code does not match expectedExitCode
//   - Exit code matches but at least one pattern is not found in output
//
// Used by both the sandbox and validate packages to ensure consistent
// verification behavior.
func CheckPlanVerification(verifyExitCode int, output string, expectedExitCode int, patterns []string) bool {
	if verifyExitCode != expectedExitCode {
		return false
	}

	for _, p := range patterns {
		if p == "" {
			continue
		}
		if !strings.Contains(output, p) {
			return false
		}
	}
	return true
}
