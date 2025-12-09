package validate

import (
	"regexp"
	"strings"
)

// Sanitizer removes sensitive information from error messages before
// they are sent to external LLM APIs for repair analysis.
type Sanitizer struct {
	maxLength    int
	homePatterns []*regexp.Regexp
	ipPatterns   []*regexp.Regexp
	credPatterns []*regexp.Regexp
}

// SanitizerOption configures a Sanitizer.
type SanitizerOption func(*Sanitizer)

// WithMaxLength sets the maximum output length.
// Output exceeding this length will be truncated with "... [truncated]" suffix.
func WithMaxLength(n int) SanitizerOption {
	return func(s *Sanitizer) {
		s.maxLength = n
	}
}

// NewSanitizer creates a sanitizer with default patterns.
// Default max length is 2000 characters.
func NewSanitizer(opts ...SanitizerOption) *Sanitizer {
	s := &Sanitizer{
		maxLength: 2000,
		homePatterns: []*regexp.Regexp{
			// Windows paths must come first (before Unix /Users/ pattern)
			// Windows home directories (with escaped backslashes)
			regexp.MustCompile(`C:\\Users\\[^\\\s]+`),
			regexp.MustCompile(`C:/Users/[^/\s]+`),
			// Unix home directories
			regexp.MustCompile(`/home/[^/\s]+`),
			// macOS home directories
			regexp.MustCompile(`/Users/[^/\s]+`),
		},
		ipPatterns: []*regexp.Regexp{
			// IPv4 addresses
			regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
			// IPv6 addresses (common patterns)
			regexp.MustCompile(`\b[0-9a-fA-F]{1,4}(:[0-9a-fA-F]{1,4}){7}\b`),
			// IPv6 compressed (::)
			regexp.MustCompile(`\b([0-9a-fA-F]{1,4}:){1,7}:\b`),
			regexp.MustCompile(`\b:([0-9a-fA-F]{1,4}:){1,7}\b`),
			regexp.MustCompile(`\b([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}\b`),
			// ::1 localhost (no word boundary since : isn't a word char)
			regexp.MustCompile(`::1(?:\b|$|\s)`),
		},
		credPatterns: []*regexp.Regexp{
			// Common credential patterns (case insensitive matching done at runtime)
			regexp.MustCompile(`(?i)(api_key|apikey|api-key)[=:]\s*\S+`),
			regexp.MustCompile(`(?i)(token|auth_token|access_token)[=:]\s*\S+`),
			regexp.MustCompile(`(?i)(password|passwd|pwd)[=:]\s*\S+`),
			regexp.MustCompile(`(?i)(secret|secrets|secret_key)[=:]\s*\S+`),
			regexp.MustCompile(`(?i)(credential|credentials)[=:]\s*\S+`),
			// Bearer tokens in headers
			regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9._-]+`),
			// Basic auth
			regexp.MustCompile(`(?i)basic\s+[a-zA-Z0-9+/=]+`),
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Sanitize removes sensitive data from the input string.
// It applies all redaction patterns and truncates if necessary.
func (s *Sanitizer) Sanitize(input string) string {
	if input == "" {
		return ""
	}

	result := input

	// Redact home directories
	// Use ReplaceAllLiteralString because $HOME contains $ which is special in regex replacement
	for _, pattern := range s.homePatterns {
		if strings.Contains(pattern.String(), `C:`) {
			result = pattern.ReplaceAllLiteralString(result, "%USERPROFILE%")
		} else {
			result = pattern.ReplaceAllLiteralString(result, "$HOME")
		}
	}

	// Redact IP addresses
	for _, pattern := range s.ipPatterns {
		result = pattern.ReplaceAllLiteralString(result, "[IP]")
	}

	// Redact credentials
	for _, pattern := range s.credPatterns {
		result = pattern.ReplaceAllLiteralString(result, "[REDACTED]")
	}

	// Truncate if necessary
	if s.maxLength > 0 && len(result) > s.maxLength {
		suffix := "... [truncated]"
		result = result[:s.maxLength-len(suffix)] + suffix
	}

	return result
}

// MaxLength returns the configured maximum output length.
func (s *Sanitizer) MaxLength() int {
	return s.maxLength
}
