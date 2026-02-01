package discover

import (
	"fmt"
	"strings"
	"unicode"
)

// NormalizeName lowercases and trims a tool name, rejecting names with
// non-ASCII characters that could be Unicode homoglyph attacks.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty tool name")
	}

	// Reject non-ASCII characters. Real homoglyph detection (confusable
	// character tables) is deferred to its own issue.
	for _, r := range name {
		if r > unicode.MaxASCII {
			return "", fmt.Errorf("tool name %q contains non-ASCII character %q — possible homoglyph attack. Use ASCII characters only", name, string(r))
		}
	}

	return strings.ToLower(name), nil
}
