package version

import (
	"errors"
	"strings"
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
		ErrTypeRateLimit,
		ErrTypeTimeout,
		ErrTypeDNS,
		ErrTypeConnection,
		ErrTypeTLS,
	}

	seen := make(map[ErrorType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("Duplicate ErrorType value: %d", et)
		}
		seen[et] = true
	}
}

func TestResolverError_Suggestion(t *testing.T) {
	tests := []struct {
		name       string
		errorType  ErrorType
		wantEmpty  bool
		wantSubstr string
	}{
		{
			name:       "rate limit has suggestion",
			errorType:  ErrTypeRateLimit,
			wantSubstr: "Wait a few minutes",
		},
		{
			name:       "timeout has suggestion",
			errorType:  ErrTypeTimeout,
			wantSubstr: "internet connection",
		},
		{
			name:       "DNS has suggestion",
			errorType:  ErrTypeDNS,
			wantSubstr: "DNS settings",
		},
		{
			name:       "connection has suggestion",
			errorType:  ErrTypeConnection,
			wantSubstr: "service may be down",
		},
		{
			name:       "TLS has suggestion",
			errorType:  ErrTypeTLS,
			wantSubstr: "certificate issue",
		},
		{
			name:       "not found has suggestion",
			errorType:  ErrTypeNotFound,
			wantSubstr: "Verify the tool/package name",
		},
		{
			name:       "generic network has suggestion",
			errorType:  ErrTypeNetwork,
			wantSubstr: "internet connection",
		},
		{
			name:      "parsing has no suggestion",
			errorType: ErrTypeParsing,
			wantEmpty: true,
		},
		{
			name:      "validation has no suggestion",
			errorType: ErrTypeValidation,
			wantEmpty: true,
		},
		{
			name:      "unknown source has no suggestion",
			errorType: ErrTypeUnknownSource,
			wantEmpty: true,
		},
		{
			name:      "not supported has no suggestion",
			errorType: ErrTypeNotSupported,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ResolverError{
				Type:    tt.errorType,
				Source:  "test",
				Message: "test message",
			}
			suggestion := err.Suggestion()

			if tt.wantEmpty {
				if suggestion != "" {
					t.Errorf("Suggestion() = %q, want empty", suggestion)
				}
			} else {
				if suggestion == "" {
					t.Errorf("Suggestion() is empty, want substring %q", tt.wantSubstr)
				} else if tt.wantSubstr != "" && !strings.Contains(suggestion, tt.wantSubstr) {
					t.Errorf("Suggestion() = %q, want substring %q", suggestion, tt.wantSubstr)
				}
			}
		})
	}
}
