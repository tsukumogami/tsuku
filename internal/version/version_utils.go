package version

import (
	"fmt"
	"strings"
)

// normalizeVersion strips common version prefixes and formats
func normalizeVersion(version string) string {
	// Strip "v" prefix
	version = strings.TrimPrefix(version, "v")

	// Handle multi-part tags like "kustomize/v5.7.1" -> "5.7.1"
	if strings.Contains(version, "/") {
		parts := strings.Split(version, "/")
		version = strings.TrimPrefix(parts[len(parts)-1], "v")
	}

	// Handle Release_X_Y_Z format (e.g., "Release_1_15_0" -> "1.15.0")
	if strings.HasPrefix(version, "Release_") {
		version = strings.TrimPrefix(version, "Release_")
		version = strings.ReplaceAll(version, "_", ".")
	}

	// Handle golang-style tags (go1.21.5 -> 1.21.5)
	version = strings.TrimPrefix(version, "go")

	return version
}

// isValidVersion checks if a version string looks like a semantic version
func isValidVersion(v string) bool {
	if v == "" {
		return false
	}

	// Must contain at least one digit
	hasDigit := false
	for _, c := range v {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}

	return hasDigit
}

// compareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Simple lexicographic comparison works for most semver strings
	// This handles cases like "1.21.5" vs "1.20.1"
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int

		// Parse part1
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &p1)
		}

		// Parse part2
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &p2)
		}

		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}

	return 0
}
