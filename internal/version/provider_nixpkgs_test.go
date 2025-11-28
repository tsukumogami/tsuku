package version

import "testing"

func TestIsValidNixpkgsVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		// Valid versions
		{"unstable", "unstable", true},
		{"YY.MM format", "24.05", true},
		{"YY.MM format older", "23.11", true},
		{"YY.MM format future", "25.05", true},
		{"longer version", "24.05.1", true},

		// Invalid versions - too short
		{"too short", "24", false},
		{"single char", "a", false},
		{"empty", "", false},

		// Invalid versions - too long
		{"too long", "12345678901", false},

		// Invalid versions - invalid characters
		{"letters in version", "24.ab", false},
		{"special chars", "24.05!", false},
		{"spaces", "24 05", false},
		{"command injection", "24.05; rm -rf /", false},
		{"path traversal", "../etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNixpkgsVersion(tt.version)
			if result != tt.expected {
				t.Errorf("isValidNixpkgsVersion(%q) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}
