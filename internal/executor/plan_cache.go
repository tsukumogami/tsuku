package executor

import (
	"fmt"
	"strings"
)

// PlanCacheKey uniquely identifies a cached plan.
// The key is based on the OUTPUT of version resolution (the resolved version),
// not the user's input. This means "ripgrep" and "ripgrep@14.1.0" that resolve
// to the same version will share the same cache key.
type PlanCacheKey struct {
	Tool     string `json:"tool"`
	Version  string `json:"version"`  // RESOLVED version (e.g., "14.1.0")
	Platform string `json:"platform"` // e.g., "linux-amd64"
	// Note: RecipeHash was removed in v4. Cache validation now uses content-based hashing.
}

// CacheKeyFor generates a cache key from version resolution output.
// This should be called AFTER version resolution completes.
func CacheKeyFor(tool, resolvedVersion, os, arch string) PlanCacheKey {
	return PlanCacheKey{
		Tool:     tool,
		Version:  resolvedVersion,
		Platform: fmt.Sprintf("%s-%s", os, arch),
	}
}

// ValidateCachedPlan checks if a cached plan is still valid for the given cache key.
// Validation checks:
//   - Format version matches current PlanFormatVersion
//   - Platform (OS-Arch) matches the key
//
// Note: Recipe hash validation was removed in v4. Content-based cache validation
// will be implemented in a future change.
//
// Returns nil if valid, or an error describing why the plan is invalid.
func ValidateCachedPlan(plan *InstallationPlan, key PlanCacheKey) error {
	// Check format version
	if plan.FormatVersion != PlanFormatVersion {
		return fmt.Errorf("plan format version %d is outdated (current: %d)",
			plan.FormatVersion, PlanFormatVersion)
	}

	// Parse platform from key (format: "os-arch")
	keyOS, keyArch, found := strings.Cut(key.Platform, "-")
	if !found {
		return fmt.Errorf("invalid platform format in cache key: %q (expected \"os-arch\")", key.Platform)
	}

	// Check platform
	if plan.Platform.OS != keyOS || plan.Platform.Arch != keyArch {
		return fmt.Errorf("plan platform %s-%s does not match %s",
			plan.Platform.OS, plan.Platform.Arch, key.Platform)
	}

	return nil
}

// ChecksumMismatchError indicates that a downloaded asset's checksum doesn't match
// the expected checksum recorded in the installation plan. This could indicate:
//   - A legitimate release update (re-tagged release)
//   - A supply chain attack (malicious modification)
type ChecksumMismatchError struct {
	Tool             string // Tool being installed
	Version          string // Version being installed
	URL              string // URL of the mismatched download
	ExpectedChecksum string // Checksum from the installation plan
	ActualChecksum   string // Checksum of the downloaded file
}

// Error implements the error interface with a user-friendly message
// that explains the issue and provides a recovery path.
func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf(`checksum mismatch for %s

Expected: %s
Got:      %s

The upstream asset has changed since the installation plan was generated.
This could indicate:
- A legitimate release update (re-tagged release)
- A supply chain attack (malicious modification)

To proceed with the new asset, regenerate the plan:
    tsuku install %s@%s --fresh

To investigate, compare with upstream release notes or checksums.`,
		e.URL, e.ExpectedChecksum, e.ActualChecksum, e.Tool, e.Version)
}
