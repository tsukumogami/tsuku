package validate

import (
	"regexp"
	"strconv"
	"strings"
)

// ErrorCategory classifies validation errors for targeted repair.
type ErrorCategory string

const (
	// ErrorBinaryNotFound indicates the expected binary was not found after installation.
	ErrorBinaryNotFound ErrorCategory = "binary_not_found"

	// ErrorExtractionFailed indicates the archive could not be extracted.
	ErrorExtractionFailed ErrorCategory = "extraction_failed"

	// ErrorVerifyFailed indicates the verification command failed.
	ErrorVerifyFailed ErrorCategory = "verify_failed"

	// ErrorPermissionDenied indicates a file permission issue.
	ErrorPermissionDenied ErrorCategory = "permission_denied"

	// ErrorDownloadFailed indicates the asset download failed.
	ErrorDownloadFailed ErrorCategory = "download_failed"

	// ErrorUnknown is used when the error doesn't match any known pattern.
	ErrorUnknown ErrorCategory = "unknown"
)

// ParsedError contains structured information about a validation failure.
type ParsedError struct {
	// Category classifies the type of error for targeted repair.
	Category ErrorCategory

	// Message is the sanitized error message.
	Message string

	// Details contains extracted information about the error.
	// Keys may include: "command", "file", "expected", "actual".
	Details map[string]string

	// Suggestions contains potential fixes for this error category.
	Suggestions []string
}

// Error patterns for each category
var (
	binaryNotFoundPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)command not found`),
		regexp.MustCompile(`(?im)(\S+):\s+not found\s*$`), // Must be at end of line (multiline)
		regexp.MustCompile(`(?i)no such file or directory`),
		regexp.MustCompile(`(?i)cannot find[:\s]+(\S+)`),
		regexp.MustCompile(`(?i)executable file not found`),
		regexp.MustCompile(`(?i)exec[:\s]+"[^"]+"\s*:\s*not found`), // exec: "cmd": not found
	}

	extractionFailedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)tar:\s+.*error`),
		regexp.MustCompile(`(?i)tar:\s+cannot`),
		regexp.MustCompile(`(?i)unzip[:\s]+.*error`),
		regexp.MustCompile(`(?i)gzip[:\s]+.*error`),
		regexp.MustCompile(`(?i)error opening archive`),
		regexp.MustCompile(`(?i)not in gzip format`),
		regexp.MustCompile(`(?i)invalid tar header`),
		regexp.MustCompile(`(?i)unexpected EOF`),
		regexp.MustCompile(`(?i)archive.*corrupt`),
	}

	verifyFailedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)verification failed`),
		regexp.MustCompile(`(?i)checksum.*mismatch`),
		regexp.MustCompile(`(?i)sha256.*mismatch`),
		regexp.MustCompile(`(?i)signature.*invalid`),
		regexp.MustCompile(`(?i)gpg[:\s]+.*failed`),
		regexp.MustCompile(`(?i)expected.*got`),
	}

	permissionDeniedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)permission denied`),
		regexp.MustCompile(`(?i)operation not permitted`),
		regexp.MustCompile(`(?i)access denied`),
		regexp.MustCompile(`(?i)EACCES`),
		regexp.MustCompile(`(?i)cannot write`),
		regexp.MustCompile(`(?i)read-only file system`),
	}

	downloadFailedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)download failed`),
		regexp.MustCompile(`(?i)connection refused`),
		regexp.MustCompile(`(?i)connection timed out`),
		regexp.MustCompile(`(?i)could not resolve host`),
		regexp.MustCompile(`(?i)404.*not found`),
		regexp.MustCompile(`(?i)curl[:\s]+.*error`),
		regexp.MustCompile(`(?i)wget[:\s]+.*error`),
		regexp.MustCompile(`(?i)HTTP.*[45]\d\d`),
	}
)

// Suggestions for each error category
var categorySuggestions = map[ErrorCategory][]string{
	ErrorBinaryNotFound: {
		"Check the binary name in the recipe matches the actual binary in the archive",
		"Verify the install_binaries action has the correct source path",
		"Check if the binary needs to be extracted from a subdirectory",
	},
	ErrorExtractionFailed: {
		"Verify the archive format matches the extraction action (tar.gz, zip, etc.)",
		"Check if the archive requires a different extraction command",
		"Ensure the download URL points to a valid archive file",
	},
	ErrorVerifyFailed: {
		"Check that the verification command is correct for this tool",
		"Verify the checksum or signature file is accessible",
		"Try running without verification to isolate the issue",
	},
	ErrorPermissionDenied: {
		"Check file permissions in the recipe actions",
		"Ensure the chmod action is applied before execution",
		"Verify the installation directory is writable",
	},
	ErrorDownloadFailed: {
		"Verify the download URL is correct and accessible",
		"Check if the URL requires authentication",
		"Ensure the asset name pattern matches the actual release asset",
	},
	ErrorUnknown: {
		"Review the full error output for clues",
		"Check if all recipe actions are in the correct order",
		"Verify the recipe syntax is correct",
	},
}

// ParseValidationError analyzes validation output to categorize the failure.
// It examines stdout, stderr, and the exit code to determine the error category
// and provide targeted suggestions for repair.
func ParseValidationError(stdout, stderr string, exitCode int) *ParsedError {
	// Combine output for pattern matching
	combined := stderr + "\n" + stdout

	// Check patterns in order of specificity (most specific first)
	// Extraction errors checked first because they may contain "not found" within tar/unzip context
	if category, details := matchCategory(combined, extractionFailedPatterns, ErrorExtractionFailed); category != ErrorUnknown {
		return &ParsedError{
			Category:    category,
			Message:     extractRelevantMessage(combined, 500),
			Details:     details,
			Suggestions: categorySuggestions[category],
		}
	}

	if category, details := matchCategory(combined, binaryNotFoundPatterns, ErrorBinaryNotFound); category != ErrorUnknown {
		return &ParsedError{
			Category:    category,
			Message:     extractRelevantMessage(combined, 500),
			Details:     details,
			Suggestions: categorySuggestions[category],
		}
	}

	if category, details := matchCategory(combined, verifyFailedPatterns, ErrorVerifyFailed); category != ErrorUnknown {
		return &ParsedError{
			Category:    category,
			Message:     extractRelevantMessage(combined, 500),
			Details:     details,
			Suggestions: categorySuggestions[category],
		}
	}

	if category, details := matchCategory(combined, permissionDeniedPatterns, ErrorPermissionDenied); category != ErrorUnknown {
		return &ParsedError{
			Category:    category,
			Message:     extractRelevantMessage(combined, 500),
			Details:     details,
			Suggestions: categorySuggestions[category],
		}
	}

	if category, details := matchCategory(combined, downloadFailedPatterns, ErrorDownloadFailed); category != ErrorUnknown {
		return &ParsedError{
			Category:    category,
			Message:     extractRelevantMessage(combined, 500),
			Details:     details,
			Suggestions: categorySuggestions[category],
		}
	}

	// No specific pattern matched - return unknown with exit code info
	details := make(map[string]string)
	if exitCode != 0 {
		details["exit_code"] = strconv.Itoa(exitCode)
	}

	return &ParsedError{
		Category:    ErrorUnknown,
		Message:     extractRelevantMessage(combined, 500),
		Details:     details,
		Suggestions: categorySuggestions[ErrorUnknown],
	}
}

// matchCategory checks if any pattern in the list matches the output.
// Returns the category if matched, or ErrorUnknown if not.
func matchCategory(output string, patterns []*regexp.Regexp, category ErrorCategory) (ErrorCategory, map[string]string) {
	details := make(map[string]string)

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(output)
		if matches != nil {
			// Extract any captured groups as details
			if len(matches) > 1 {
				details["matched"] = matches[1]
			}
			return category, details
		}
	}

	return ErrorUnknown, nil
}

// extractRelevantMessage extracts a relevant portion of the error output.
// It prioritizes stderr content and truncates to maxLength.
func extractRelevantMessage(output string, maxLength int) string {
	// Clean up the output
	output = strings.TrimSpace(output)

	if output == "" {
		return "No error output captured"
	}

	// Take the last portion if too long (usually most relevant)
	if len(output) > maxLength {
		// Find a good break point
		truncated := output[len(output)-maxLength:]
		// Try to start at a newline
		if idx := strings.Index(truncated, "\n"); idx > 0 && idx < 50 {
			truncated = truncated[idx+1:]
		}
		return "..." + truncated
	}

	return output
}
