package version

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ErrorType classifies resolver errors for better handling
type ErrorType int

const (
	// ErrTypeNetwork indicates a generic network-related error (fallback when specific type is unknown)
	ErrTypeNetwork ErrorType = iota
	// ErrTypeNotFound indicates the requested resource was not found (HTTP 404, etc.)
	ErrTypeNotFound
	// ErrTypeParsing indicates an error parsing response data (TOML, JSON, etc.)
	ErrTypeParsing
	// ErrTypeValidation indicates data validation failure (invalid version format, etc.)
	ErrTypeValidation
	// ErrTypeUnknownSource indicates an unknown/unregistered version source
	ErrTypeUnknownSource
	// ErrTypeNotSupported indicates the operation is not supported for this source
	ErrTypeNotSupported
	// ErrTypeRateLimit indicates API rate limit exceeded (HTTP 429, or 403 with rate limit headers)
	ErrTypeRateLimit
	// ErrTypeTimeout indicates a request timeout
	ErrTypeTimeout
	// ErrTypeDNS indicates DNS resolution failure
	ErrTypeDNS
	// ErrTypeConnection indicates connection refused or reset
	ErrTypeConnection
	// ErrTypeTLS indicates TLS/SSL certificate errors
	ErrTypeTLS
)

// ResolverError provides structured error information for version resolution failures
type ResolverError struct {
	Type    ErrorType
	Source  string // Version source name (e.g., "rust_dist", "nodejs_dist")
	Message string // Human-readable error message
	Err     error  // Underlying error (if any)
}

// Error implements the error interface
func (e *ResolverError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s resolver: %s: %v", e.Source, e.Message, e.Err)
	}
	return fmt.Sprintf("%s resolver: %s", e.Source, e.Message)
}

// Unwrap returns the underlying error for error chain support
func (e *ResolverError) Unwrap() error {
	return e.Err
}

// Suggestion returns an actionable suggestion for the user based on the error type.
// Returns an empty string if no specific suggestion is available.
func (e *ResolverError) Suggestion() string {
	switch e.Type {
	case ErrTypeRateLimit:
		return "Wait a few minutes before trying again, or check if you need to authenticate"
	case ErrTypeTimeout:
		return "Check your internet connection and try again"
	case ErrTypeDNS:
		return "Check your DNS settings and internet connection"
	case ErrTypeConnection:
		return "The service may be down or blocked. Check if you can access it in a browser"
	case ErrTypeTLS:
		return "There may be a certificate issue. Check your system time is correct"
	case ErrTypeNotFound:
		return "Verify the tool/package name is correct"
	case ErrTypeNetwork:
		return "Check your internet connection and try again"
	default:
		return ""
	}
}

// ClassifyError examines an error and returns the most specific ErrorType.
// This function uses Go's error unwrapping to detect specific network error types.
func ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrTypeNetwork
	}

	// Check for context timeout/deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTypeTimeout
	}

	// Check for context canceled (user interrupt)
	if errors.Is(err, context.Canceled) {
		return ErrTypeNetwork
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsTimeout {
			return ErrTypeTimeout
		}
		return ErrTypeDNS
	}

	// Check for TLS certificate errors
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return ErrTypeTLS
	}

	// Check for net.OpError (connection errors)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return ErrTypeTimeout
		}
		// Check if underlying error is a DNS error
		var innerDNS *net.DNSError
		if errors.As(opErr.Err, &innerDNS) {
			return ErrTypeDNS
		}
		// Connection refused, reset, etc.
		return ErrTypeConnection
	}

	// Check for url.Error (wraps transport errors)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return ErrTypeTimeout
		}
		// Check error message for TLS hints
		if strings.Contains(urlErr.Err.Error(), "certificate") ||
			strings.Contains(urlErr.Err.Error(), "tls") ||
			strings.Contains(urlErr.Err.Error(), "x509") {
			return ErrTypeTLS
		}
		// Recurse into the wrapped error
		return ClassifyError(urlErr.Err)
	}

	// Default to generic network error
	return ErrTypeNetwork
}

// WrapNetworkError wraps an error with the appropriate error type based on classification.
// This is a convenience function for providers to create properly typed ResolverErrors.
func WrapNetworkError(err error, source, message string) *ResolverError {
	return &ResolverError{
		Type:    ClassifyError(err),
		Source:  source,
		Message: message,
		Err:     err,
	}
}

// GitHubRateLimitContext describes what operation was being performed when rate limited
type GitHubRateLimitContext string

const (
	// GitHubContextVersionResolution indicates rate limit hit while resolving tool versions
	GitHubContextVersionResolution GitHubRateLimitContext = "version_resolution"
)

// GitHubRateLimitError provides detailed information about GitHub API rate limit errors.
// It includes the rate limit details, time until reset, and context about what operation
// was being performed.
type GitHubRateLimitError struct {
	Limit         int                    // Requests per hour
	Remaining     int                    // Remaining requests
	ResetTime     time.Time              // When the rate limit resets
	Authenticated bool                   // Whether the request was authenticated
	Context       GitHubRateLimitContext // What operation was being performed
	Err           error                  // Underlying error
}

// Error implements the error interface
func (e *GitHubRateLimitError) Error() string {
	authStatus := "unauthenticated"
	if e.Authenticated {
		authStatus = "authenticated"
	}

	var contextMsg string
	switch e.Context {
	case GitHubContextVersionResolution:
		contextMsg = "while resolving tool versions from GitHub"
	default:
		contextMsg = "while accessing GitHub API"
	}

	return fmt.Sprintf("GitHub API rate limit exceeded %s: %d/%d requests used (%s), resets at %s",
		contextMsg, e.Limit-e.Remaining, e.Limit, authStatus, e.ResetTime.Format(time.Kitchen))
}

// Unwrap returns the underlying error for error chain support
func (e *GitHubRateLimitError) Unwrap() error {
	return e.Err
}

// Suggestion returns actionable suggestions for the user.
// The suggestions are context-aware based on the operation context and authentication status.
func (e *GitHubRateLimitError) Suggestion() string {
	var sb strings.Builder

	// Explain why tsuku uses GitHub based on context
	switch e.Context {
	case GitHubContextVersionResolution:
		sb.WriteString("Tsuku uses the GitHub API to discover available versions for tools hosted on GitHub. ")
	default:
		sb.WriteString("Tsuku uses the GitHub API to access tool information. ")
	}

	// Calculate time until reset
	timeUntilReset := time.Until(e.ResetTime)
	if timeUntilReset < 0 {
		timeUntilReset = 0
	}

	// Format the duration in a human-readable way
	minutes := int(timeUntilReset.Minutes())
	if minutes > 0 {
		sb.WriteString(fmt.Sprintf("Rate limit resets in %d minute(s). ", minutes))
	} else {
		sb.WriteString("Rate limit should reset soon. ")
	}

	if !e.Authenticated {
		sb.WriteString("Set GITHUB_TOKEN environment variable to increase limit from 60 to 5000 requests/hour. ")
	}

	sb.WriteString("You can also specify a version directly: tsuku install <tool>@<version>")

	return sb.String()
}
