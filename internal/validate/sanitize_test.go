package validate

import (
	"strings"
	"testing"
)

func TestNewSanitizer(t *testing.T) {
	s := NewSanitizer()
	if s.MaxLength() != 2000 {
		t.Errorf("expected default maxLength 2000, got %d", s.MaxLength())
	}
}

func TestNewSanitizerWithMaxLength(t *testing.T) {
	s := NewSanitizer(WithMaxLength(500))
	if s.MaxLength() != 500 {
		t.Errorf("expected maxLength 500, got %d", s.MaxLength())
	}
}

func TestSanitizeEmpty(t *testing.T) {
	s := NewSanitizer()
	result := s.Sanitize("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestSanitizeUnixHomePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "linux home path",
			input:    "/home/johndoe/.config/app",
			expected: "$HOME/.config/app",
		},
		{
			name:     "linux home in error message",
			input:    "Error: file not found at /home/alice/project/src/main.go",
			expected: "Error: file not found at $HOME/project/src/main.go",
		},
		{
			name:     "multiple linux home paths",
			input:    "Copying /home/user1/file to /home/user2/dest",
			expected: "Copying $HOME/file to $HOME/dest",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeMacOSHomePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "macos home path",
			input:    "/Users/johndoe/Library/Caches",
			expected: "$HOME/Library/Caches",
		},
		{
			name:     "macos home in stack trace",
			input:    "at /Users/developer/go/src/myapp/main.go:42",
			expected: "at $HOME/go/src/myapp/main.go:42",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeWindowsHomePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "windows backslash path",
			input:    `C:\Users\JohnDoe\Documents\project`,
			expected: `%USERPROFILE%\Documents\project`,
		},
		{
			name:     "windows forward slash path",
			input:    "C:/Users/JohnDoe/Documents/project",
			expected: "%USERPROFILE%/Documents/project",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeIPv4Addresses(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple ipv4",
			input:    "Connection to 192.168.1.1 failed",
			expected: "Connection to [IP] failed",
		},
		{
			name:     "multiple ipv4",
			input:    "Route: 10.0.0.1 -> 172.16.0.1 -> 8.8.8.8",
			expected: "Route: [IP] -> [IP] -> [IP]",
		},
		{
			name:     "localhost ipv4",
			input:    "Listening on 127.0.0.1:8080",
			expected: "Listening on [IP]:8080",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeIPv6Addresses(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "localhost ipv6",
			input:    "Listening on ::1",
			expected: "Listening on [IP]",
		},
		{
			name:     "full ipv6",
			input:    "Connected to 2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: "Connected to [IP]",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeCredentials(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "api_key equals",
			input:    "api_key=sk-1234567890abcdef",
			expected: "[REDACTED]",
		},
		{
			name:     "api_key colon",
			input:    "api_key: sk-1234567890abcdef",
			expected: "[REDACTED]",
		},
		{
			name:     "apikey no underscore",
			input:    "apikey=secret123",
			expected: "[REDACTED]",
		},
		{
			name:     "api-key hyphen",
			input:    "api-key=mykey123",
			expected: "[REDACTED]",
		},
		{
			name:     "token",
			input:    "token=abc123xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "access_token",
			input:    "access_token=ghp_xxxxxxxxxxxx",
			expected: "[REDACTED]",
		},
		{
			name:     "password",
			input:    "password=s3cr3t!",
			expected: "[REDACTED]",
		},
		{
			name:     "secret",
			input:    "secret=very-secret-value",
			expected: "[REDACTED]",
		},
		{
			name:     "credentials",
			input:    "credentials=user:pass",
			expected: "[REDACTED]",
		},
		{
			name:     "bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Authorization: [REDACTED]",
		},
		{
			name:     "basic auth",
			input:    "Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ=",
			expected: "Authorization: [REDACTED]",
		},
		{
			name:     "case insensitive API_KEY",
			input:    "API_KEY=uppercase",
			expected: "[REDACTED]",
		},
		{
			name:     "case insensitive Password",
			input:    "Password=MixedCase",
			expected: "[REDACTED]",
		},
	}

	s := NewSanitizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeTruncation(t *testing.T) {
	s := NewSanitizer(WithMaxLength(50))

	// Create input longer than 50 chars
	input := strings.Repeat("x", 100)
	result := s.Sanitize(input)

	if len(result) != 50 {
		t.Errorf("expected length 50, got %d", len(result))
	}

	if !strings.HasSuffix(result, "... [truncated]") {
		t.Errorf("expected truncated suffix, got %q", result)
	}
}

func TestSanitizeNoTruncationWhenUnderLimit(t *testing.T) {
	s := NewSanitizer(WithMaxLength(100))

	input := "Short message"
	result := s.Sanitize(input)

	if result != input {
		t.Errorf("expected %q, got %q", input, result)
	}
}

func TestSanitizeCombinedPatterns(t *testing.T) {
	s := NewSanitizer()

	input := `Error connecting to database at 192.168.1.100:5432
Config file: /home/developer/.config/db.conf
Connection string: password=s3cr3t host=10.0.0.1
Stack trace:
  at /Users/developer/app/db.go:42
Authorization: Bearer eyJtoken123`

	result := s.Sanitize(input)

	// Check all patterns were applied
	if strings.Contains(result, "192.168.1.100") {
		t.Error("IPv4 address not redacted")
	}
	if strings.Contains(result, "/home/developer") {
		t.Error("Linux home path not redacted")
	}
	if strings.Contains(result, "/Users/developer") {
		t.Error("macOS home path not redacted")
	}
	if strings.Contains(result, "s3cr3t") {
		t.Error("Password not redacted")
	}
	if strings.Contains(result, "10.0.0.1") {
		t.Error("Second IPv4 not redacted")
	}
	if strings.Contains(result, "eyJtoken123") {
		t.Error("Bearer token not redacted")
	}

	// Check replacements are present
	if !strings.Contains(result, "$HOME") {
		t.Error("Expected $HOME replacement")
	}
	if !strings.Contains(result, "[IP]") {
		t.Error("Expected [IP] replacement")
	}
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("Expected [REDACTED] replacement")
	}
}

func TestSanitizePreservesNonSensitiveContent(t *testing.T) {
	s := NewSanitizer()

	input := `Error: command not found: mytool
Expected binary at /usr/local/bin/mytool
Exit code: 127`

	result := s.Sanitize(input)

	// Non-sensitive paths should be preserved
	if !strings.Contains(result, "/usr/local/bin/mytool") {
		t.Errorf("Expected /usr/local/bin path to be preserved, got %q", result)
	}

	// Error messages should be preserved
	if !strings.Contains(result, "command not found") {
		t.Error("Expected error message to be preserved")
	}
	if !strings.Contains(result, "Exit code: 127") {
		t.Error("Expected exit code to be preserved")
	}
}
