package version

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
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
