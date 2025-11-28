package version

import (
	"errors"
	"testing"
)

func TestResolverError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ResolverError
		expected string
	}{
		{
			name: "with underlying error",
			err: &ResolverError{
				Type:    ErrTypeNetwork,
				Source:  "github",
				Message: "connection failed",
				Err:     errors.New("timeout"),
			},
			expected: "github resolver: connection failed: timeout",
		},
		{
			name: "without underlying error",
			err: &ResolverError{
				Type:    ErrTypeNotFound,
				Source:  "npm",
				Message: "package not found",
				Err:     nil,
			},
			expected: "npm resolver: package not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolverError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &ResolverError{
		Type:    ErrTypeNetwork,
		Source:  "test",
		Message: "test message",
		Err:     underlying,
	}

	if err.Unwrap() != underlying {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), underlying)
	}

	// Test with nil underlying error
	errNoUnderlying := &ResolverError{
		Type:    ErrTypeNotFound,
		Source:  "test",
		Message: "test message",
		Err:     nil,
	}

	if errNoUnderlying.Unwrap() != nil {
		t.Errorf("Unwrap() with nil underlying = %v, want nil", errNoUnderlying.Unwrap())
	}
}

func TestErrorType_Constants(t *testing.T) {
	// Verify error type constants are distinct
	types := []ErrorType{
		ErrTypeNetwork,
		ErrTypeNotFound,
		ErrTypeParsing,
		ErrTypeValidation,
		ErrTypeUnknownSource,
		ErrTypeNotSupported,
	}

	seen := make(map[ErrorType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("Duplicate ErrorType value: %d", et)
		}
		seen[et] = true
	}
}
