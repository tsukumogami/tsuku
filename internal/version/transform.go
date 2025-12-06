package version

import (
	"errors"
	"regexp"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// MaxVersionLength is the maximum allowed length for version strings
const MaxVersionLength = 128

// ErrInvalidVersionString indicates the version string contains invalid characters or is too long
var ErrInvalidVersionString = errors.New("invalid version string")

// validVersionChars matches the allowed character set for version strings: [a-zA-Z0-9._+\-@/]
// Includes @ and / to support scoped package names like @biomejs/biome@2.3.8
var validVersionChars = regexp.MustCompile(`^[a-zA-Z0-9._+\-@/]+$`)

// semverRegex extracts X.Y.Z from any format (ignores prerelease/build)
var semverRegex = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// semverFullRegex extracts X.Y.Z[-prerelease][+build] from any format
var semverFullRegex = regexp.MustCompile(`(\d+\.\d+\.\d+(?:-[a-zA-Z0-9._-]+)?(?:\+[a-zA-Z0-9._-]+)?)`)

// ValidateVersionString checks that a version string contains only allowed characters
// and is within the maximum length. Returns nil if valid, ErrInvalidVersionString otherwise.
func ValidateVersionString(version string) error {
	if version == "" {
		return ErrInvalidVersionString
	}
	if len(version) > MaxVersionLength {
		return ErrInvalidVersionString
	}
	if !validVersionChars.MatchString(version) {
		return ErrInvalidVersionString
	}
	return nil
}

// TransformVersion applies a format transformation to a version string.
// Returns the transformed version and any error encountered.
//
// Supported formats:
//   - raw: Returns the version unchanged
//   - semver: Extracts X.Y.Z (e.g., "biome@2.3.8" → "2.3.8", "v1.2.3-rc.1" → "1.2.3")
//   - semver_full: Extracts X.Y.Z[-pre][+build] (e.g., "v1.2.3-rc.1+build" → "1.2.3-rc.1+build")
//   - strip_v: Removes leading "v" (e.g., "v1.2.3" → "1.2.3")
//   - unknown: Treated as raw with no error (forward compatibility)
//
// If the transform cannot extract a version, the original is returned with an error.
func TransformVersion(version, format string) (string, error) {
	// Validate input first
	if err := ValidateVersionString(version); err != nil {
		return version, err
	}

	switch format {
	case "", recipe.VersionFormatRaw:
		return version, nil

	case recipe.VersionFormatSemver:
		return transformSemver(version)

	case recipe.VersionFormatSemverFull:
		return transformSemverFull(version)

	case recipe.VersionFormatStripV:
		return transformStripV(version)

	default:
		// Unknown format: treat as raw for forward compatibility
		return version, nil
	}
}

// transformSemver extracts X.Y.Z from any version string format.
// Examples:
//   - "biome@2.3.8" → "2.3.8"
//   - "v1.2.3-rc.1" → "1.2.3"
//   - "go1.21.0" → "1.21.0"
func transformSemver(version string) (string, error) {
	match := semverRegex.FindString(version)
	if match == "" {
		return version, errors.New("no semver pattern found in version string")
	}
	return match, nil
}

// transformSemverFull extracts X.Y.Z[-prerelease][+build] from any version string.
// Preserves prerelease and build metadata.
// Examples:
//   - "v1.2.3-rc.1+build" → "1.2.3-rc.1+build"
//   - "biome@2.3.8-beta" → "2.3.8-beta"
func transformSemverFull(version string) (string, error) {
	match := semverFullRegex.FindString(version)
	if match == "" {
		return version, errors.New("no semver pattern found in version string")
	}
	return match, nil
}

// transformStripV removes a leading "v" or "V" from the version string.
// If no leading v exists, returns the original unchanged.
// Examples:
//   - "v1.2.3" → "1.2.3"
//   - "V1.0.0" → "1.0.0"
//   - "1.2.3" → "1.2.3"
func transformStripV(version string) (string, error) {
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		return version[1:], nil
	}
	return version, nil
}
