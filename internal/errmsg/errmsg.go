// Package errmsg provides enhanced error message formatting with actionable suggestions.
package errmsg

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/tsuku-dev/tsuku/internal/version"
)

// ErrorContext provides additional context for error formatting
type ErrorContext struct {
	ToolName string // The tool being operated on (for suggestions)
}

// Format returns a formatted error message with possible causes and suggestions.
// The context parameter is optional - pass nil for generic formatting.
func Format(err error, ctx *ErrorContext) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Check for ResolverError (structured errors from version package)
	var resolverErr *version.ResolverError
	if errors.As(err, &resolverErr) {
		return formatResolverError(resolverErr, ctx)
	}

	// Check for rate limit errors (string matching for unstructured errors)
	if isRateLimitError(errMsg) {
		return formatRateLimitError(errMsg, ctx)
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return formatNetworkError(netErr, ctx)
	}

	// Check for connection-related errors by message
	if isNetworkError(errMsg) {
		return formatGenericNetworkError(errMsg, ctx)
	}

	// Check for "not found" errors
	if isNotFoundError(errMsg) {
		return formatNotFoundError(errMsg, ctx)
	}

	// Check for permission errors
	if isPermissionError(errMsg) {
		return formatPermissionError(errMsg, ctx)
	}

	// Return original error for unrecognized types
	return errMsg
}

func formatResolverError(err *version.ResolverError, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(err.Error())
	sb.WriteString("\n")

	switch err.Type {
	case version.ErrTypeNetwork:
		sb.WriteString("\nPossible causes:\n")
		sb.WriteString("  - Network connectivity issue\n")
		sb.WriteString("  - Service temporarily unavailable\n")
		sb.WriteString("  - GitHub API rate limit exceeded\n")

		sb.WriteString("\nSuggestions:\n")
		sb.WriteString("  - Check your internet connection\n")
		sb.WriteString("  - Set GITHUB_TOKEN to increase rate limit\n")
		sb.WriteString("  - Try again in a few minutes\n")

	case version.ErrTypeNotFound:
		sb.WriteString("\nPossible causes:\n")
		sb.WriteString("  - The version does not exist\n")
		sb.WriteString("  - The source no longer provides this version\n")

		sb.WriteString("\nSuggestions:\n")
		if ctx != nil && ctx.ToolName != "" {
			sb.WriteString(fmt.Sprintf("  - Run 'tsuku versions %s' to see available versions\n", ctx.ToolName))
		} else {
			sb.WriteString("  - Run 'tsuku versions <tool>' to see available versions\n")
		}
		sb.WriteString("  - Use 'latest' to get the most recent version\n")

	case version.ErrTypeValidation:
		sb.WriteString("\nPossible causes:\n")
		sb.WriteString("  - Invalid version format\n")
		sb.WriteString("  - Unexpected data from version source\n")

		sb.WriteString("\nSuggestions:\n")
		if ctx != nil && ctx.ToolName != "" {
			sb.WriteString(fmt.Sprintf("  - Run 'tsuku versions %s' to see available versions\n", ctx.ToolName))
		} else {
			sb.WriteString("  - Run 'tsuku versions <tool>' to see available versions\n")
		}

	case version.ErrTypeUnknownSource:
		sb.WriteString("\nPossible causes:\n")
		sb.WriteString("  - Recipe uses an unsupported version source\n")
		sb.WriteString("  - Recipe configuration error\n")

		sb.WriteString("\nSuggestions:\n")
		sb.WriteString("  - Check the recipe configuration\n")
		sb.WriteString("  - Report the issue at github.com/tsuku-dev/tsuku/issues\n")

	default:
		// Generic suggestions for other resolver errors
		sb.WriteString("\nSuggestions:\n")
		sb.WriteString("  - Try again in a few minutes\n")
		sb.WriteString("  - Check tsuku's issue tracker for known problems\n")
	}

	return sb.String()
}

func formatRateLimitError(errMsg string, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	sb.WriteString("\nPossible causes:\n")
	sb.WriteString("  - Too many requests to the API\n")
	sb.WriteString("  - Unauthenticated requests have lower limits\n")

	sb.WriteString("\nSuggestions:\n")
	sb.WriteString("  - Set GITHUB_TOKEN environment variable to increase rate limit\n")
	sb.WriteString("  - Wait a few minutes before retrying\n")
	if ctx != nil && ctx.ToolName != "" {
		sb.WriteString(fmt.Sprintf("  - Use 'tsuku install %s@<version>' to specify a version directly\n", ctx.ToolName))
	}

	return sb.String()
}

func formatNetworkError(err net.Error, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(err.Error())
	sb.WriteString("\n")

	sb.WriteString("\nPossible causes:\n")
	if err.Timeout() {
		sb.WriteString("  - Request timed out\n")
		sb.WriteString("  - Slow or unstable network connection\n")
	} else {
		sb.WriteString("  - Network connectivity issue\n")
		sb.WriteString("  - DNS resolution failure\n")
	}
	sb.WriteString("  - Firewall or proxy blocking the connection\n")

	sb.WriteString("\nSuggestions:\n")
	sb.WriteString("  - Check your internet connection\n")
	sb.WriteString("  - Try again in a few minutes\n")
	if err.Timeout() {
		sb.WriteString("  - Check if you're behind a slow proxy\n")
	}

	return sb.String()
}

func formatGenericNetworkError(errMsg string, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	sb.WriteString("\nPossible causes:\n")
	sb.WriteString("  - Network connectivity issue\n")
	sb.WriteString("  - DNS resolution failure\n")
	sb.WriteString("  - Service temporarily unavailable\n")

	sb.WriteString("\nSuggestions:\n")
	sb.WriteString("  - Check your internet connection\n")
	sb.WriteString("  - Try again in a few minutes\n")

	return sb.String()
}

func formatNotFoundError(errMsg string, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	sb.WriteString("\nPossible causes:\n")
	sb.WriteString("  - Recipe does not exist in the registry\n")
	sb.WriteString("  - Typo in the tool name\n")

	sb.WriteString("\nSuggestions:\n")
	sb.WriteString("  - Check the spelling of the tool name\n")
	sb.WriteString("  - Run 'tsuku recipes' to see available recipes\n")
	if ctx != nil && ctx.ToolName != "" {
		sb.WriteString(fmt.Sprintf("  - Run 'tsuku create %s --from <ecosystem>' to create a recipe\n", ctx.ToolName))
		sb.WriteString("    Available ecosystems: crates.io, rubygems, pypi, npm\n")
	}

	return sb.String()
}

func formatPermissionError(errMsg string, ctx *ErrorContext) string {
	var sb strings.Builder
	sb.WriteString(errMsg)
	sb.WriteString("\n")

	sb.WriteString("\nPossible causes:\n")
	sb.WriteString("  - Insufficient permissions on $TSUKU_HOME directory\n")
	sb.WriteString("  - File or directory owned by different user\n")

	sb.WriteString("\nSuggestions:\n")
	sb.WriteString("  - Check permissions on ~/.tsuku directory\n")
	sb.WriteString("  - Ensure you own the tsuku directories: ls -la ~/.tsuku\n")

	return sb.String()
}

// isRateLimitError checks if the error message indicates a rate limit
func isRateLimitError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "rate-limit") ||
		strings.Contains(lower, "too many requests")
}

// isNetworkError checks if the error message indicates a network issue
func isNetworkError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "i/o timeout")
}

// isNotFoundError checks if the error message indicates something not found
func isNotFoundError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "404") ||
		strings.Contains(lower, "does not exist")
}

// isPermissionError checks if the error message indicates a permission issue
func isPermissionError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "operation not permitted")
}
