package validate

import (
	"regexp"
	"strings"
)

// VerifyFailureAnalysis contains results from analyzing a verification failure.
type VerifyFailureAnalysis struct {
	// Repairable indicates whether this failure can be deterministically repaired.
	Repairable bool

	// ToolName is the tool name extracted from output, if found.
	ToolName string

	// ExitCode is the process exit code.
	ExitCode int

	// HasUsageText indicates the output contains a "usage:" pattern.
	HasUsageText bool

	// HasToolName indicates the output contains the tool name.
	HasToolName bool

	// OutputLength is the total length of stdout + stderr.
	OutputLength int

	// SuggestedMode is the suggested verify mode ("output" for help text).
	SuggestedMode string

	// SuggestedPattern is the suggested pattern to match in output.
	SuggestedPattern string

	// SuggestedReason is the suggested reason field for the verify section.
	SuggestedReason string
}

// Help text detection patterns.
var (
	// usagePattern matches common help text indicators.
	usagePattern = regexp.MustCompile(`(?im)^\s*usage[:\s]`)

	// optionsPattern matches options section headers.
	optionsPattern = regexp.MustCompile(`(?im)^\s*(options|flags|arguments)[:\s]`)

	// commandsPattern matches commands section headers.
	commandsPattern = regexp.MustCompile(`(?im)^\s*(commands|subcommands)[:\s]`)

	// helpSuggestionPattern matches suggestions to use help.
	helpSuggestionPattern = regexp.MustCompile(`(?i)(--help|-h|help)\b`)
)

// AnalyzeVerifyFailure examines sandbox output to determine if a verification
// failure is deterministically repairable.
//
// A failure is considered repairable when:
//   - Exit code is 1 or 2 (typical "invalid argument" codes)
//   - Output contains help-text indicators (usage:, options:, etc.)
//   - Output is long enough to be meaningful help text (>200 bytes)
//
// Exit code 127 (command not found) is NOT repairable - the binary is missing.
// Exit code 0 should not reach this function (validation passed).
func AnalyzeVerifyFailure(stdout, stderr string, exitCode int, toolName string) *VerifyFailureAnalysis {
	combined := stdout + "\n" + stderr
	outputLen := len(combined)

	analysis := &VerifyFailureAnalysis{
		ExitCode:     exitCode,
		OutputLength: outputLen,
		ToolName:     toolName,
	}

	// Exit code 127 means command not found - not repairable
	if exitCode == 127 {
		return analysis
	}

	// Exit code 0 means success - shouldn't be here but not repairable
	if exitCode == 0 {
		return analysis
	}

	// Only consider exit codes 1-2 as potentially repairable
	// These are common "invalid argument" or "usage error" codes
	if exitCode < 1 || exitCode > 2 {
		// Some tools use higher exit codes for invalid args, but be conservative
		// Still check for help text patterns
	}

	// Check for help text patterns
	analysis.HasUsageText = usagePattern.MatchString(combined)
	hasOptions := optionsPattern.MatchString(combined)
	hasCommands := commandsPattern.MatchString(combined)
	hasHelpSuggestion := helpSuggestionPattern.MatchString(combined)

	// Check if tool name appears in output (case-insensitive)
	if toolName != "" {
		analysis.HasToolName = strings.Contains(strings.ToLower(combined), strings.ToLower(toolName))
	}

	// Determine if this is repairable
	// Primary signal: has usage text pattern
	// Secondary signals: has options/commands sections, output is long enough
	helpIndicators := 0
	if analysis.HasUsageText {
		helpIndicators += 2 // Strong signal
	}
	if hasOptions || hasCommands {
		helpIndicators += 1
	}
	if hasHelpSuggestion {
		helpIndicators += 1
	}
	if analysis.HasToolName {
		helpIndicators += 1
	}
	if outputLen > 200 {
		helpIndicators += 1
	}

	// Consider repairable if we have strong signals
	// (usage text alone, or multiple weaker signals)
	if helpIndicators >= 2 && (exitCode == 1 || exitCode == 2) {
		analysis.Repairable = true
		analysis.SuggestedMode = "output"

		// Choose the best pattern to match
		if analysis.HasUsageText {
			analysis.SuggestedPattern = "usage"
		} else if analysis.HasToolName && toolName != "" {
			analysis.SuggestedPattern = escapeForPattern(toolName)
		} else {
			// Fallback to a generic pattern that should match help text
			analysis.SuggestedPattern = "usage"
		}

		analysis.SuggestedReason = "verification repaired: tool does not support --version"
	}

	return analysis
}

// escapeForPattern escapes special regex characters for use in a pattern.
// For simple tool name matching, we escape characters that could be regex.
func escapeForPattern(s string) string {
	// Escape common regex metacharacters
	// Note: backslash must be processed first so we don't double-escape
	metacharacters := []string{"\\", ".", "+", "*", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|"}
	result := s
	for _, c := range metacharacters {
		result = strings.ReplaceAll(result, c, "\\"+c)
	}
	return result
}

// IsHelpExitCode returns true if the exit code is commonly used for
// "invalid argument" or help display scenarios.
func IsHelpExitCode(exitCode int) bool {
	// 1: general error (often used for invalid args)
	// 2: misuse of shell command (POSIX, Python convention for arg errors)
	return exitCode == 1 || exitCode == 2
}

// IsNotFoundExitCode returns true if the exit code indicates the command
// was not found.
func IsNotFoundExitCode(exitCode int) bool {
	// 127: command not found (shell convention)
	return exitCode == 127
}
