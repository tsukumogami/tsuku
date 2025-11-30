// Package errmsg provides error formatting with actionable suggestions.
package errmsg

import (
	"errors"
	"fmt"
	"io"
)

// Suggester is an interface for errors that can provide actionable suggestions.
// Both version.ResolverError and registry.RegistryError implement this interface.
type Suggester interface {
	error
	Suggestion() string
}

// FormatError returns a formatted error message with suggestion if available.
// If the error implements Suggester and has a non-empty suggestion, it is appended.
func FormatError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()

	// Check if any error in the chain provides a suggestion
	suggestion := extractSuggestion(err)
	if suggestion != "" {
		return fmt.Sprintf("%s\n\nSuggestion: %s", msg, suggestion)
	}

	return msg
}

// extractSuggestion walks the error chain looking for a Suggester.
// Returns the first non-empty suggestion found.
func extractSuggestion(err error) string {
	for err != nil {
		if s, ok := err.(Suggester); ok {
			if suggestion := s.Suggestion(); suggestion != "" {
				return suggestion
			}
		}
		err = errors.Unwrap(err)
	}
	return ""
}

// Fprint writes a formatted error message to w.
// It adds "Error: " prefix and includes suggestion if available.
func Fprint(w io.Writer, err error) {
	if err == nil {
		return
	}

	msg := FormatError(err)
	fmt.Fprintf(w, "Error: %s\n", msg)
}
