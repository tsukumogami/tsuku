// Package verify provides library verification functionality for tsuku.
package verify

import "fmt"

// HeaderInfo contains validated header information for a shared library.
type HeaderInfo struct {
	// Format identifies the binary format ("ELF" or "Mach-O")
	Format string

	// Type describes the file type ("shared object", "dynamic library", etc.)
	Type string

	// Architecture is the target architecture ("x86_64", "arm64", etc.)
	Architecture string

	// Dependencies lists required libraries (DT_NEEDED or LC_LOAD_DYLIB)
	Dependencies []string

	// SymbolCount is the number of exported dynamic symbols.
	// -1 indicates symbol counting was skipped for performance.
	SymbolCount int

	// SourceArch is set for fat binaries to indicate which slice was used.
	// Empty for single-architecture files.
	SourceArch string
}

// ErrorCategory classifies validation failures for user-friendly reporting.
type ErrorCategory int

const (
	// ErrUnreadable indicates the file could not be read (permission, not found, etc.)
	ErrUnreadable ErrorCategory = iota

	// ErrInvalidFormat indicates the file is not a recognized binary format
	ErrInvalidFormat

	// ErrNotSharedLib indicates the file is a valid binary but not a shared library
	// (e.g., executable, object file, static library)
	ErrNotSharedLib

	// ErrWrongArch indicates the library is for a different architecture
	ErrWrongArch

	// ErrTruncated indicates the file appears truncated (unexpected EOF)
	ErrTruncated

	// ErrCorrupted indicates the file has invalid internal structure
	ErrCorrupted
)

// String returns a human-readable name for the error category.
func (c ErrorCategory) String() string {
	switch c {
	case ErrUnreadable:
		return "unreadable"
	case ErrInvalidFormat:
		return "invalid format"
	case ErrNotSharedLib:
		return "not a shared library"
	case ErrWrongArch:
		return "wrong architecture"
	case ErrTruncated:
		return "truncated"
	case ErrCorrupted:
		return "corrupted"
	default:
		return fmt.Sprintf("unknown(%d)", c)
	}
}

// ValidationError categorizes validation failures for user-friendly reporting.
type ValidationError struct {
	Category ErrorCategory
	Path     string
	Message  string
	Err      error // Underlying error (may be nil)
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Category, e.Err)
	}
	return e.Category.String()
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *ValidationError) Unwrap() error {
	return e.Err
}
