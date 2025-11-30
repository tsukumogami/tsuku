package errmsg

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/version"
)

func TestFormat_NilError(t *testing.T) {
	result := Format(nil, nil)
	if result != "" {
		t.Errorf("expected empty string for nil error, got %q", result)
	}
}

func TestFormat_GenericError(t *testing.T) {
	err := errors.New("something went wrong")
	result := Format(err, nil)
	if result != "something went wrong" {
		t.Errorf("expected original error message, got %q", result)
	}
}

func TestFormat_ResolverError_Network(t *testing.T) {
	err := &version.ResolverError{
		Type:    version.ErrTypeNetwork,
		Source:  "github",
		Message: "connection failed",
	}

	ctx := &ErrorContext{ToolName: "mytool"}
	result := Format(err, ctx)

	// Check that result contains key elements
	checks := []string{
		"connection failed",
		"Possible causes:",
		"Network connectivity issue",
		"Suggestions:",
		"Check your internet connection",
		"GITHUB_TOKEN",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_ResolverError_NotFound(t *testing.T) {
	err := &version.ResolverError{
		Type:    version.ErrTypeNotFound,
		Source:  "nodejs",
		Message: "version v99.0.0 not found",
	}

	ctx := &ErrorContext{ToolName: "nodejs"}
	result := Format(err, ctx)

	checks := []string{
		"version v99.0.0 not found",
		"Possible causes:",
		"The version does not exist",
		"Suggestions:",
		"tsuku versions nodejs",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_RateLimitError(t *testing.T) {
	err := errors.New("GitHub API rate limit exceeded")
	ctx := &ErrorContext{ToolName: "kubectl"}
	result := Format(err, ctx)

	checks := []string{
		"rate limit",
		"Possible causes:",
		"Too many requests",
		"Suggestions:",
		"GITHUB_TOKEN",
		"tsuku install kubectl@<version>",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_NetworkError(t *testing.T) {
	err := errors.New("dial tcp: connection refused")
	result := Format(err, nil)

	checks := []string{
		"connection refused",
		"Possible causes:",
		"Network connectivity issue",
		"Suggestions:",
		"Check your internet connection",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_NotFoundError(t *testing.T) {
	err := errors.New("recipe not found in registry: nonexistent-tool")
	ctx := &ErrorContext{ToolName: "nonexistent-tool"}
	result := Format(err, ctx)

	checks := []string{
		"not found",
		"Possible causes:",
		"Recipe does not exist",
		"Typo",
		"Suggestions:",
		"tsuku recipes",
		"tsuku create nonexistent-tool",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_PermissionError(t *testing.T) {
	err := errors.New("open /home/user/.tsuku/tools: permission denied")
	result := Format(err, nil)

	checks := []string{
		"permission denied",
		"Possible causes:",
		"Insufficient permissions",
		"Suggestions:",
		"~/.tsuku",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	msg       string
	timeout   bool
	temporary bool
}

func (e mockNetError) Error() string   { return e.msg }
func (e mockNetError) Timeout() bool   { return e.timeout }
func (e mockNetError) Temporary() bool { return e.temporary }

// Ensure mockNetError implements net.Error
var _ net.Error = mockNetError{}

func TestFormat_NetError_Timeout(t *testing.T) {
	err := mockNetError{
		msg:     "i/o timeout",
		timeout: true,
	}
	result := Format(err, nil)

	checks := []string{
		"i/o timeout",
		"Possible causes:",
		"Request timed out",
		"Suggestions:",
		"slow proxy",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormat_WithoutContext(t *testing.T) {
	err := &version.ResolverError{
		Type:    version.ErrTypeNotFound,
		Source:  "test",
		Message: "not found",
	}

	result := Format(err, nil)

	// Should use generic suggestion without tool name
	if !strings.Contains(result, "tsuku versions <tool>") {
		t.Errorf("expected generic suggestion, got:\n%s", result)
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"GitHub API rate limit exceeded", true},
		{"rate-limit: too many requests", true},
		{"Too many requests to the server", true},
		{"connection failed", false},
		{"file not found", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := isRateLimitError(tt.msg); got != tt.expected {
				t.Errorf("isRateLimitError(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"dial tcp: connection refused", true},
		{"connection reset by peer", true},
		{"no such host", true},
		{"i/o timeout", true},
		{"file not found", false},
		{"permission denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := isNetworkError(tt.msg); got != tt.expected {
				t.Errorf("isNetworkError(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"recipe not found", true},
		{"returned 404", true},
		{"does not exist in registry", true},
		{"connection failed", false},
		{"rate limit exceeded", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := isNotFoundError(tt.msg); got != tt.expected {
				t.Errorf("isNotFoundError(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}

func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"permission denied", true},
		{"access denied", true},
		{"operation not permitted", true},
		{"file not found", false},
		{"connection refused", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := isPermissionError(tt.msg); got != tt.expected {
				t.Errorf("isPermissionError(%q) = %v, want %v", tt.msg, got, tt.expected)
			}
		})
	}
}
