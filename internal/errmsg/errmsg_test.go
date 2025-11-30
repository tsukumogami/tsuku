package errmsg

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

// mockSuggesterError is a test error that implements Suggester
type mockSuggesterError struct {
	message    string
	suggestion string
}

func (e *mockSuggesterError) Error() string      { return e.message }
func (e *mockSuggesterError) Suggestion() string { return e.suggestion }

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "generic error without suggestion",
			err:      errors.New("something went wrong"),
			expected: "something went wrong",
		},
		{
			name: "error with suggestion",
			err: &mockSuggesterError{
				message:    "connection refused",
				suggestion: "Check your internet connection",
			},
			expected: "connection refused\n\nSuggestion: Check your internet connection",
		},
		{
			name: "error with empty suggestion",
			err: &mockSuggesterError{
				message:    "unknown error",
				suggestion: "",
			},
			expected: "unknown error",
		},
		{
			name: "wrapped error with suggestion",
			err: fmt.Errorf("failed to fetch: %w", &mockSuggesterError{
				message:    "rate limit exceeded",
				suggestion: "Wait a few minutes before trying again",
			}),
			expected: "failed to fetch: rate limit exceeded\n\nSuggestion: Wait a few minutes before trying again",
		},
		{
			name: "deeply wrapped error with suggestion",
			err: fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", &mockSuggesterError{
				message:    "DNS resolution failed",
				suggestion: "Check your DNS settings",
			})),
			expected: "outer: middle: DNS resolution failed\n\nSuggestion: Check your DNS settings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			if result != tt.expected {
				t.Errorf("FormatError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFprint(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: "Error: something went wrong\n",
		},
		{
			name: "error with suggestion",
			err: &mockSuggesterError{
				message:    "timeout",
				suggestion: "Try again later",
			},
			expected: "Error: timeout\n\nSuggestion: Try again later\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			Fprint(&buf, tt.err)
			if buf.String() != tt.expected {
				t.Errorf("Fprint() wrote %q, want %q", buf.String(), tt.expected)
			}
		})
	}
}

func TestExtractSuggestion(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "error without Suggester",
			err:      errors.New("plain error"),
			expected: "",
		},
		{
			name: "direct Suggester",
			err: &mockSuggesterError{
				message:    "error",
				suggestion: "fix it",
			},
			expected: "fix it",
		},
		{
			name: "wrapped Suggester",
			err: fmt.Errorf("wrap: %w", &mockSuggesterError{
				message:    "inner",
				suggestion: "inner suggestion",
			}),
			expected: "inner suggestion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSuggestion(tt.err)
			if result != tt.expected {
				t.Errorf("extractSuggestion() = %q, want %q", result, tt.expected)
			}
		})
	}
}
