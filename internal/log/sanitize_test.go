package log

import (
	"testing"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Empty and simple cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple URL no sensitive data",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "URL with non-sensitive query params preserves values",
			input:    "https://example.com/path?foo=bar",
			expected: "https://example.com/path?foo=bar",
		},

		// Basic Auth redaction
		{
			name:     "Basic Auth with password",
			input:    "https://user:pass@example.com/path",
			expected: "https://REDACTED@example.com/path",
		},
		{
			name:     "Basic Auth with complex password",
			input:    "https://user:p@ss:word@example.com/path",
			expected: "https://REDACTED@example.com/path",
		},
		{
			name:     "Basic Auth user only no password",
			input:    "https://user@example.com/path",
			expected: "https://user@example.com/path",
		},

		// Query parameter redaction - token variations
		{
			name:     "token query param",
			input:    "https://example.com/api?token=secret123",
			expected: "https://example.com/api?token=REDACTED",
		},
		{
			name:     "access_token query param",
			input:    "https://example.com/api?access_token=abc123",
			expected: "https://example.com/api?access_token=REDACTED",
		},
		{
			name:     "refresh_token query param",
			input:    "https://example.com/api?refresh_token=xyz",
			expected: "https://example.com/api?refresh_token=REDACTED",
		},

		// Query parameter redaction - key variations
		{
			name:     "key query param",
			input:    "https://example.com/api?key=abc123",
			expected: "https://example.com/api?key=REDACTED",
		},
		{
			name:     "api_key query param",
			input:    "https://example.com/api?api_key=abc123",
			expected: "https://example.com/api?api_key=REDACTED",
		},
		{
			name:     "apikey query param",
			input:    "https://example.com/api?apikey=abc123",
			expected: "https://example.com/api?apikey=REDACTED",
		},

		// Query parameter redaction - other sensitive patterns
		{
			name:     "secret query param",
			input:    "https://example.com/api?secret=xyz",
			expected: "https://example.com/api?secret=REDACTED",
		},
		{
			name:     "client_secret query param",
			input:    "https://example.com/api?client_secret=xyz",
			expected: "https://example.com/api?client_secret=REDACTED",
		},
		{
			name:     "password query param",
			input:    "https://example.com/api?password=xyz",
			expected: "https://example.com/api?password=REDACTED",
		},
		{
			name:     "auth query param",
			input:    "https://example.com/api?auth=bearer_token",
			expected: "https://example.com/api?auth=REDACTED",
		},
		{
			name:     "authorization query param",
			input:    "https://example.com/api?authorization=Bearer%20xyz",
			expected: "https://example.com/api?authorization=REDACTED",
		},
		{
			name:     "credential query param",
			input:    "https://example.com/api?credential=xyz",
			expected: "https://example.com/api?credential=REDACTED",
		},

		// Case insensitivity
		{
			name:     "TOKEN uppercase",
			input:    "https://example.com/api?TOKEN=abc",
			expected: "https://example.com/api?TOKEN=REDACTED",
		},
		{
			name:     "ApiKey mixed case",
			input:    "https://example.com/api?ApiKey=abc",
			expected: "https://example.com/api?ApiKey=REDACTED",
		},

		// Multiple parameters - mix of sensitive and non-sensitive
		{
			name:     "multiple params some sensitive",
			input:    "https://example.com/api?version=v1&token=secret&format=json",
			expected: "https://example.com/api?format=json&token=REDACTED&version=v1",
		},
		{
			name:     "multiple sensitive params",
			input:    "https://example.com/api?token=a&key=b&secret=c",
			expected: "https://example.com/api?key=REDACTED&secret=REDACTED&token=REDACTED",
		},

		// Combined Basic Auth and query params
		{
			name:     "Basic Auth and sensitive query param",
			input:    "https://user:pass@example.com/api?token=xyz",
			expected: "https://REDACTED@example.com/api?token=REDACTED",
		},

		// Edge cases
		{
			name:     "URL with fragment",
			input:    "https://example.com/path?token=abc#section",
			expected: "https://example.com/path?token=REDACTED#section",
		},
		{
			name:     "URL with port",
			input:    "https://example.com:8080/path?key=abc",
			expected: "https://example.com:8080/path?key=REDACTED",
		},
		{
			name:     "file URL",
			input:    "file:///path/to/file?token=abc",
			expected: "file:///path/to/file?token=REDACTED",
		},
		{
			name:     "relative URL with query",
			input:    "/api/v1?token=abc",
			expected: "/api/v1?token=REDACTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeURL_InvalidURL(t *testing.T) {
	// Invalid URLs should be returned unchanged
	tests := []string{
		"://missing-scheme",
		"http://[::1:bad",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			result := SanitizeURL(input)
			if result != input {
				t.Errorf("SanitizeURL(%q) = %q, want unchanged input", input, result)
			}
		})
	}
}

func TestIsSensitiveParam(t *testing.T) {
	tests := []struct {
		param    string
		expected bool
	}{
		// Positive cases
		{"token", true},
		{"Token", true},
		{"TOKEN", true},
		{"access_token", true},
		{"refresh_token", true},
		{"key", true},
		{"api_key", true},
		{"apikey", true},
		{"secret", true},
		{"client_secret", true},
		{"password", true},
		{"auth", true},
		{"authorization", true},
		{"credential", true},
		{"credentials", true},

		// Negative cases
		{"version", false},
		{"format", false},
		{"page", false},
		{"limit", false},
		{"id", false},
		{"name", false},
		{"callback", false},
	}

	for _, tt := range tests {
		t.Run(tt.param, func(t *testing.T) {
			result := isSensitiveParam(tt.param)
			if result != tt.expected {
				t.Errorf("isSensitiveParam(%q) = %v, want %v", tt.param, result, tt.expected)
			}
		})
	}
}
