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

// CompareVersions compares two version strings.
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
//
// Supported formats:
//   - Semver: "1.2.3", "v1.2.3", "1.2.3-rc.1"
//   - Calver: "2024.01.15", "24.05"
//   - Go toolchain: "go1.21.5"
//   - Custom: "Release_1_15_0", "kustomize/v5.7.1"
//
// Prerelease handling:
//   - Stable versions sort higher than prereleases: 1.0.0 > 1.0.0-rc.1
//   - Prereleases sort lexicographically: alpha < beta < rc
func CompareVersions(v1, v2 string) int {
	// Normalize versions to strip prefixes (v, go, Release_, etc.)
	n1 := normalizeVersion(v1)
	n2 := normalizeVersion(v2)

	// Split into core version and prerelease parts
	core1, pre1 := splitPrerelease(n1)
	core2, pre2 := splitPrerelease(n2)

	// Compare core version parts
	coreResult := compareCoreParts(core1, core2)
	if coreResult != 0 {
		return coreResult
	}

	// Core versions are equal, compare prerelease
	return comparePrereleases(pre1, pre2)
}

// splitPrerelease splits a version into core and prerelease parts.
// "1.0.0-rc.1" -> ("1.0.0", "rc.1")
// "1.0.0" -> ("1.0.0", "")
// "1.0.0+build.123" -> ("1.0.0", "") -- build metadata is ignored
func splitPrerelease(version string) (core, prerelease string) {
	// Strip build metadata first (everything after +)
	if idx := strings.Index(version, "+"); idx != -1 {
		version = version[:idx]
	}

	// Split on first hyphen for prerelease
	if idx := strings.Index(version, "-"); idx != -1 {
		return version[:idx], version[idx+1:]
	}
	return version, ""
}

// compareCoreParts compares the numeric core parts of two versions.
func compareCoreParts(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int

		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &p1)
		}

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

// comparePrereleases compares two prerelease strings.
// Empty prerelease (stable) > any prerelease.
// Non-empty prereleases are compared with prerelease-aware ordering.
func comparePrereleases(pre1, pre2 string) int {
	// Stable versions (no prerelease) are greater than prereleases
	if pre1 == "" && pre2 == "" {
		return 0
	}
	if pre1 == "" {
		return 1 // v1 is stable, v2 is prerelease
	}
	if pre2 == "" {
		return -1 // v1 is prerelease, v2 is stable
	}

	// Both have prereleases - use prerelease-aware comparison
	return comparePrereleaseStrings(pre1, pre2)
}

// comparePrereleaseStrings compares two non-empty prerelease strings.
// Handles common prerelease identifiers: alpha < beta < rc
// Falls back to lexicographic comparison for unknown identifiers.
func comparePrereleaseStrings(pre1, pre2 string) int {
	// Split prereleases into parts (e.g., "rc.1" -> ["rc", "1"])
	parts1 := strings.Split(pre1, ".")
	parts2 := strings.Split(pre2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		// Empty part is less than non-empty
		if p1 == "" && p2 != "" {
			return -1
		}
		if p1 != "" && p2 == "" {
			return 1
		}

		result := comparePrereleaseIdentifiers(p1, p2)
		if result != 0 {
			return result
		}
	}

	return 0
}

// comparePrereleaseIdentifiers compares two prerelease identifiers.
// Numeric identifiers are compared numerically.
// Known identifiers (alpha, beta, rc) have defined ordering.
// Unknown identifiers are compared lexicographically.
func comparePrereleaseIdentifiers(id1, id2 string) int {
	// Try numeric comparison first
	var n1, n2 int
	parsed1, _ := fmt.Sscanf(id1, "%d", &n1)
	parsed2, _ := fmt.Sscanf(id2, "%d", &n2)
	isNum1 := parsed1 == 1
	isNum2 := parsed2 == 1

	if isNum1 && isNum2 {
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
		return 0
	}

	// Numeric identifiers have lower precedence than non-numeric
	if isNum1 && !isNum2 {
		return -1
	}
	if !isNum1 && isNum2 {
		return 1
	}

	// Both non-numeric: use prerelease ordering
	order1 := prereleaseOrder(id1)
	order2 := prereleaseOrder(id2)

	if order1 != order2 {
		if order1 > order2 {
			return 1
		}
		return -1
	}

	// Same order or both unknown - lexicographic comparison
	if id1 > id2 {
		return 1
	}
	if id1 < id2 {
		return -1
	}
	return 0
}

// prereleaseOrder returns a numeric order for common prerelease identifiers.
// Lower numbers sort earlier. Unknown identifiers return a high value.
func prereleaseOrder(id string) int {
	// Normalize to lowercase for comparison
	lower := strings.ToLower(id)
	switch lower {
	case "alpha", "a":
		return 1
	case "beta", "b":
		return 2
	case "rc", "cr":
		return 3
	case "pre", "preview":
		return 0 // Before alpha
	default:
		return 100 // Unknown identifiers sort after known ones
	}
}
