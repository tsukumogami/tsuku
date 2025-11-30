package version

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantType ErrorType
	}{
		{
			name:     "nil error",
			err:      nil,
			wantType: ErrTypeNetwork,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			wantType: ErrTypeTimeout,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			wantType: ErrTypeNetwork,
		},
		{
			name: "DNS error",
			err: &net.DNSError{
				Err:  "no such host",
				Name: "example.com",
			},
			wantType: ErrTypeDNS,
		},
		{
			name: "DNS timeout error",
			err: &net.DNSError{
				Err:       "timeout",
				Name:      "example.com",
				IsTimeout: true,
			},
			wantType: ErrTypeTimeout,
		},
		{
			name: "net.OpError timeout",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: &timeoutError{},
			},
			wantType: ErrTypeTimeout,
		},
		{
			name: "net.OpError connection refused",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errors.New("connection refused"),
			},
			wantType: ErrTypeConnection,
		},
		{
			name: "url.Error with timeout",
			err: &url.Error{
				Op:  "Get",
				URL: "https://example.com",
				Err: &timeoutError{},
			},
			wantType: ErrTypeTimeout,
		},
		{
			name: "url.Error with certificate error",
			err: &url.Error{
				Op:  "Get",
				URL: "https://example.com",
				Err: errors.New("x509: certificate has expired"),
			},
			wantType: ErrTypeTLS,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			wantType: ErrTypeNetwork,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.wantType {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

// timeoutError is a helper for testing timeout detection
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

func TestWrapNetworkError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		source     string
		message    string
		wantType   ErrorType
		wantSource string
	}{
		{
			name:       "wraps DNS error",
			err:        &net.DNSError{Err: "no such host", Name: "example.com"},
			source:     "github",
			message:    "failed to fetch",
			wantType:   ErrTypeDNS,
			wantSource: "github",
		},
		{
			name:       "wraps timeout error",
			err:        context.DeadlineExceeded,
			source:     "npm",
			message:    "request timed out",
			wantType:   ErrTypeTimeout,
			wantSource: "npm",
		},
		{
			name:       "wraps generic error",
			err:        errors.New("unknown error"),
			source:     "pypi",
			message:    "failed to connect",
			wantType:   ErrTypeNetwork,
			wantSource: "pypi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapNetworkError(tt.err, tt.source, tt.message)

			if result.Type != tt.wantType {
				t.Errorf("WrapNetworkError().Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Source != tt.wantSource {
				t.Errorf("WrapNetworkError().Source = %v, want %v", result.Source, tt.wantSource)
			}
			if result.Message != tt.message {
				t.Errorf("WrapNetworkError().Message = %v, want %v", result.Message, tt.message)
			}
			if result.Err != tt.err {
				t.Errorf("WrapNetworkError().Err = %v, want %v", result.Err, tt.err)
			}
		})
	}
}

func TestGitHubRateLimitError_Error(t *testing.T) {
	tests := []struct {
		name          string
		limit         int
		remaining     int
		resetTime     time.Time
		authenticated bool
		context       GitHubRateLimitContext
		wantSubstr    []string
	}{
		{
			name:          "unauthenticated version resolution",
			limit:         60,
			remaining:     0,
			resetTime:     time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			authenticated: false,
			context:       GitHubContextVersionResolution,
			wantSubstr:    []string{"60/60", "unauthenticated", "2:30PM", "resolving tool versions"},
		},
		{
			name:          "authenticated version resolution",
			limit:         5000,
			remaining:     100,
			resetTime:     time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
			authenticated: true,
			context:       GitHubContextVersionResolution,
			wantSubstr:    []string{"4900/5000", "authenticated", "3:00PM", "resolving tool versions"},
		},
		{
			name:          "unknown context (default message)",
			limit:         60,
			remaining:     0,
			resetTime:     time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			authenticated: false,
			context:       "", // Empty context
			wantSubstr:    []string{"60/60", "accessing GitHub API"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &GitHubRateLimitError{
				Limit:         tt.limit,
				Remaining:     tt.remaining,
				ResetTime:     tt.resetTime,
				Authenticated: tt.authenticated,
				Context:       tt.context,
			}
			errStr := err.Error()
			for _, substr := range tt.wantSubstr {
				if !strings.Contains(errStr, substr) {
					t.Errorf("Error() = %q, want substring %q", errStr, substr)
				}
			}
		})
	}
}

func TestGitHubRateLimitError_Unwrap(t *testing.T) {
	underlying := errors.New("rate limit exceeded")
	err := &GitHubRateLimitError{
		Limit:         60,
		Remaining:     0,
		ResetTime:     time.Now().Add(30 * time.Minute),
		Authenticated: false,
		Err:           underlying,
	}

	if err.Unwrap() != underlying {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), underlying)
	}

	// Test with nil underlying error
	errNoUnderlying := &GitHubRateLimitError{
		Limit:         60,
		Remaining:     0,
		ResetTime:     time.Now().Add(30 * time.Minute),
		Authenticated: false,
		Err:           nil,
	}

	if errNoUnderlying.Unwrap() != nil {
		t.Errorf("Unwrap() with nil underlying = %v, want nil", errNoUnderlying.Unwrap())
	}
}

func TestGitHubRateLimitError_Suggestion(t *testing.T) {
	tests := []struct {
		name          string
		resetTime     time.Time
		authenticated bool
		context       GitHubRateLimitContext
		wantSubstrs   []string
		dontWant      []string
	}{
		{
			name:          "version resolution context explains GitHub usage",
			resetTime:     time.Now().Add(15 * time.Minute),
			authenticated: false,
			context:       GitHubContextVersionResolution,
			wantSubstrs:   []string{"GitHub API to discover available versions", "resets in", "minute", "GITHUB_TOKEN", "5000 requests/hour", "specify a version"},
		},
		{
			name:          "unknown context has generic explanation",
			resetTime:     time.Now().Add(15 * time.Minute),
			authenticated: false,
			context:       "", // Empty context
			wantSubstrs:   []string{"GitHub API to access tool information", "resets in", "minute"},
		},
		{
			name:          "authenticated with time remaining",
			resetTime:     time.Now().Add(15 * time.Minute),
			authenticated: true,
			context:       GitHubContextVersionResolution,
			wantSubstrs:   []string{"resets in", "minute", "specify a version"},
			dontWant:      []string{"GITHUB_TOKEN"},
		},
		{
			name:          "reset time in past",
			resetTime:     time.Now().Add(-5 * time.Minute),
			authenticated: false,
			context:       GitHubContextVersionResolution,
			wantSubstrs:   []string{"reset soon", "GITHUB_TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &GitHubRateLimitError{
				Limit:         60,
				Remaining:     0,
				ResetTime:     tt.resetTime,
				Authenticated: tt.authenticated,
				Context:       tt.context,
			}
			suggestion := err.Suggestion()

			for _, substr := range tt.wantSubstrs {
				if !strings.Contains(suggestion, substr) {
					t.Errorf("Suggestion() = %q, want substring %q", suggestion, substr)
				}
			}
			for _, substr := range tt.dontWant {
				if strings.Contains(suggestion, substr) {
					t.Errorf("Suggestion() = %q, should not contain %q", suggestion, substr)
				}
			}
		})
	}
}
