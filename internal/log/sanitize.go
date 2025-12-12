package log

import (
	"net/url"
	"strings"
)

// sensitiveParamPatterns contains substrings that indicate a query parameter
// may contain sensitive data. These are checked case-insensitively.
var sensitiveParamPatterns = []string{
	"token",
	"key",
	"secret",
	"password",
	"auth",
	"credential",
	"api_key",
	"apikey",
}

// redactedValue is the replacement for sensitive data.
const redactedValue = "REDACTED"

// SanitizeURL removes credentials from URLs for safe logging.
// It handles Basic Auth credentials and sensitive query parameters.
//
// Sanitization patterns:
//   - Basic Auth: "https://user:pass@host" → "https://REDACTED@host"
//   - Query params with sensitive names: "?token=abc" → "?token=REDACTED"
//
// If the URL cannot be parsed, it is returned unchanged.
func SanitizeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Redact Basic Auth password
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.User(redactedValue)
		}
	}

	// Redact sensitive query parameters
	if parsed.RawQuery != "" {
		query := parsed.Query()
		modified := false
		for key := range query {
			if isSensitiveParam(key) {
				query.Set(key, redactedValue)
				modified = true
			}
		}
		if modified {
			parsed.RawQuery = query.Encode()
		}
	}

	return parsed.String()
}

// isSensitiveParam checks if a query parameter name indicates sensitive data.
func isSensitiveParam(param string) bool {
	lower := strings.ToLower(param)
	for _, pattern := range sensitiveParamPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
