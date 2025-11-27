package version

import "fmt"

// ErrorType classifies resolver errors for better handling
type ErrorType int

const (
	// ErrTypeNetwork indicates a network-related error (timeout, connection failure, etc.)
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
