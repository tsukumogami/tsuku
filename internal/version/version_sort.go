package version

import (
	"sort"
)

// SortVersionsDescending sorts versions in descending order (latest first).
// Handles semver, calver, and custom formats with graceful fallback.
// The input slice is not modified; a new sorted slice is returned.
func SortVersionsDescending(versions []string) []string {
	if len(versions) == 0 {
		return versions
	}

	result := make([]string, len(versions))
	copy(result, versions)

	sort.Slice(result, func(i, j int) bool {
		return CompareVersions(result[i], result[j]) > 0
	})

	return result
}

// IsSortedDescending checks if versions are sorted in descending order.
// Returns true if versions are properly sorted (latest first).
func IsSortedDescending(versions []string) bool {
	for i := 1; i < len(versions); i++ {
		if CompareVersions(versions[i-1], versions[i]) < 0 {
			return false
		}
	}
	return true
}
