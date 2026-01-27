package registry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrorType classifies registry errors for better handling
type ErrorType int

const (
	// ErrTypeNetwork indicates a generic network-related error
	ErrTypeNetwork ErrorType = iota
	// ErrTypeNotFound indicates the recipe was not found (HTTP 404)
	ErrTypeNotFound
	// ErrTypeParsing indicates an error parsing response data
	ErrTypeParsing
	// ErrTypeValidation indicates data validation failure
	ErrTypeValidation
	// ErrTypeRateLimit indicates API rate limit exceeded (HTTP 429)
	ErrTypeRateLimit
	// ErrTypeTimeout indicates a request timeout
	ErrTypeTimeout
	// ErrTypeDNS indicates DNS resolution failure
	ErrTypeDNS
	// ErrTypeConnection indicates connection refused or reset
	ErrTypeConnection
	// ErrTypeTLS indicates TLS/SSL certificate errors
	ErrTypeTLS
	// ErrTypeCacheRead indicates a cache read operation failed
	ErrTypeCacheRead
	// ErrTypeCacheWrite indicates a cache write operation failed
	ErrTypeCacheWrite
	// ErrTypeCacheTooStale indicates cache exists but exceeds max staleness
	ErrTypeCacheTooStale
	// ErrTypeCacheStaleUsed indicates stale cache was used (warning context)
	ErrTypeCacheStaleUsed
)

// RegistryError provides structured error information for registry operations
type RegistryError struct {
	Type    ErrorType
	Recipe  string // Recipe name that caused the error
	Message string // Human-readable error message
	Err     error  // Underlying error (if any)
}

// Error implements the error interface
func (e *RegistryError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("registry: %s: %v", e.Message, e.Err)
	}
	return fmt.Sprintf("registry: %s", e.Message)
}

// Unwrap returns the underlying error for error chain support
func (e *RegistryError) Unwrap() error {
	return e.Err
}

// Suggestion returns an actionable suggestion for the user based on the error type.
// Returns an empty string if no specific suggestion is available.
func (e *RegistryError) Suggestion() string {
	switch e.Type {
	case ErrTypeRateLimit:
		return "Wait a few minutes before trying again"
	case ErrTypeTimeout:
		return "Check your internet connection and try again"
	case ErrTypeDNS:
		return "Check your DNS settings and internet connection"
	case ErrTypeConnection:
		return "The registry may be down or blocked. Check if you can access GitHub"
	case ErrTypeTLS:
		return "There may be a certificate issue. Check your system time is correct"
	case ErrTypeNotFound:
		return "Verify the recipe name is correct. Run 'tsuku recipes' to list available recipes"
	case ErrTypeNetwork:
		return "Check your internet connection and try again"
	case ErrTypeCacheTooStale:
		return "Run 'tsuku update-registry' when you have internet connectivity to refresh the cache"
	default:
		return ""
	}
}

// classifyError examines an error and returns the most specific ErrorType.
// This function uses Go's error unwrapping to detect specific network error types.
func classifyError(err error) ErrorType {
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
		return classifyError(urlErr.Err)
	}

	// Default to generic network error
	return ErrTypeNetwork
}

// WrapNetworkError wraps a network error with the appropriate error type based on classification.
// This is a convenience function for creating properly typed RegistryErrors from network operations.
func WrapNetworkError(err error, recipe, message string) *RegistryError {
	return &RegistryError{
		Type:    classifyError(err),
		Recipe:  recipe,
		Message: message,
		Err:     err,
	}
}
