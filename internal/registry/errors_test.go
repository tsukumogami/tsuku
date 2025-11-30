package registry

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestRegistryError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *RegistryError
		expected string
	}{
		{
			name: "with underlying error",
			err: &RegistryError{
				Type:    ErrTypeNetwork,
				Recipe:  "gh",
				Message: "failed to fetch recipe",
				Err:     errors.New("connection refused"),
			},
			expected: "registry: failed to fetch recipe: connection refused",
		},
		{
			name: "without underlying error",
			err: &RegistryError{
				Type:    ErrTypeNotFound,
				Recipe:  "unknown-tool",
				Message: "recipe unknown-tool not found in registry",
				Err:     nil,
			},
			expected: "registry: recipe unknown-tool not found in registry",
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

func TestRegistryError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &RegistryError{
		Type:    ErrTypeNetwork,
		Recipe:  "test",
		Message: "test message",
		Err:     underlying,
	}

	if err.Unwrap() != underlying {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), underlying)
	}

	// Test with nil underlying error
	errNoUnderlying := &RegistryError{
		Type:    ErrTypeNotFound,
		Recipe:  "test",
		Message: "test message",
		Err:     nil,
	}

	if errNoUnderlying.Unwrap() != nil {
		t.Errorf("Unwrap() with nil underlying = %v, want nil", errNoUnderlying.Unwrap())
	}
}

func TestRegistryError_Suggestion(t *testing.T) {
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
			wantSubstr: "GitHub",
		},
		{
			name:       "TLS has suggestion",
			errorType:  ErrTypeTLS,
			wantSubstr: "certificate",
		},
		{
			name:       "not found has suggestion",
			errorType:  ErrTypeNotFound,
			wantSubstr: "tsuku recipes",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &RegistryError{
				Type:    tt.errorType,
				Recipe:  "test",
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

func TestWrapNetworkError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		recipe     string
		message    string
		wantType   ErrorType
		wantRecipe string
	}{
		{
			name:       "wraps DNS error",
			err:        &net.DNSError{Err: "no such host", Name: "example.com"},
			recipe:     "gh",
			message:    "failed to fetch",
			wantType:   ErrTypeDNS,
			wantRecipe: "gh",
		},
		{
			name:       "wraps timeout error",
			err:        context.DeadlineExceeded,
			recipe:     "serve",
			message:    "request timed out",
			wantType:   ErrTypeTimeout,
			wantRecipe: "serve",
		},
		{
			name:       "wraps generic error",
			err:        errors.New("unknown error"),
			recipe:     "btop",
			message:    "failed to connect",
			wantType:   ErrTypeNetwork,
			wantRecipe: "btop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapNetworkError(tt.err, tt.recipe, tt.message)

			if result.Type != tt.wantType {
				t.Errorf("WrapNetworkError().Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Recipe != tt.wantRecipe {
				t.Errorf("WrapNetworkError().Recipe = %v, want %v", result.Recipe, tt.wantRecipe)
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
			name: "net.OpError with nested DNS error",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: &net.DNSError{Err: "no such host", Name: "example.com"},
			},
			wantType: ErrTypeDNS,
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
			name: "url.Error with tls error",
			err: &url.Error{
				Op:  "Get",
				URL: "https://example.com",
				Err: errors.New("tls: handshake failure"),
			},
			wantType: ErrTypeTLS,
		},
		{
			name: "url.Error with generic error",
			err: &url.Error{
				Op:  "Get",
				URL: "https://example.com",
				Err: errors.New("connection reset"),
			},
			wantType: ErrTypeNetwork,
		},
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			wantType: ErrTypeNetwork,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if got != tt.wantType {
				t.Errorf("classifyError() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

// timeoutError is a helper for testing timeout detection
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
