package recipe

import (
	"fmt"
	"regexp"
	"strings"
)

// HardcodedVersion represents a detected hardcoded version in a recipe field.
type HardcodedVersion struct {
	// Step is the 1-based step index where the hardcoded version was found
	Step int
	// Action is the action type of the step
	Action string
	// Field is the field name containing the hardcoded version
	Field string
	// Value is the detected version string
	Value string
	// FullValue is the complete field value for context
	FullValue string
}

// String returns a human-readable description of the hardcoded version.
func (h HardcodedVersion) String() string {
	suggestion := strings.Replace(h.FullValue, h.Value, "{version}", 1)
	return fmt.Sprintf("step %d (%s): field '%s' contains hardcoded version '%s'; use '%s' instead",
		h.Step, h.Action, h.Field, h.Value, suggestion)
}

// VersionFieldRule defines version detection behavior for a specific field.
type VersionFieldRule struct {
	// Field is the parameter name to check
	Field string
	// ExpectPlaceholder indicates this field should use {version} placeholder
	ExpectPlaceholder bool
}

// actionVersionRules maps action names to their version-sensitive fields.
// Actions not listed here have no version-related fields to check.
var actionVersionRules = map[string][]VersionFieldRule{
	// Download actions
	"download": {
		{Field: "url", ExpectPlaceholder: true},
		{Field: "checksum_url", ExpectPlaceholder: true},
	},
	"download_archive": {
		{Field: "url", ExpectPlaceholder: true},
		{Field: "checksum_url", ExpectPlaceholder: true},
	},
	// download_file: No ExpectPlaceholder fields - static URLs are expected

	// GitHub actions
	"github_archive": {
		{Field: "asset_pattern", ExpectPlaceholder: true},
	},
	"github_file": {
		{Field: "asset_pattern", ExpectPlaceholder: true},
	},

	// Extract action
	"extract": {
		{Field: "archive", ExpectPlaceholder: true},
	},

	// Build actions
	"configure_make": {
		{Field: "source_dir", ExpectPlaceholder: true},
	},
	"cmake_build": {
		{Field: "source_dir", ExpectPlaceholder: true},
	},
	"meson_build": {
		{Field: "source_dir", ExpectPlaceholder: true},
	},
	"cargo_build": {
		{Field: "source_dir", ExpectPlaceholder: true},
	},
	"go_build": {
		{Field: "source_dir", ExpectPlaceholder: true},
	},
}

// versionPatterns detects common version formats.
// Order matters: more specific patterns should come first.
// Each pattern uses a capturing group to extract just the version portion.
var versionPatterns = []*regexp.Regexp{
	// Semver with prerelease/build: 1.2.3-beta.1, 1.2.3+build
	// Captures: full match including prerelease/build metadata
	regexp.MustCompile(`\b([vV]?\d+\.\d+\.\d+(?:-[a-zA-Z0-9]+(?:\.[a-zA-Z0-9]+)*)?(?:\+[a-zA-Z0-9]+(?:\.[a-zA-Z0-9]+)*)?)(?:\.tar|\.zip|\.gz|\.tgz|\s|/|$)`),
	// Basic semver: 1.2.3, v1.2.3
	regexp.MustCompile(`\b([vV]?\d+\.\d+\.\d+)\b`),
	// Date-based: 2024.01, 2024.01.15
	regexp.MustCompile(`\b(20\d{2}\.\d{2}(?:\.\d{2})?)\b`),
	// Two-part version: 1.2, v1.2 (but NOT single digits or platform strings)
	regexp.MustCompile(`\b([vV]?\d+\.\d+)\b`),
}

// excludePatterns matches values that should NOT be flagged as hardcoded versions.
var excludePatterns = []*regexp.Regexp{
	// API versions in URLs
	regexp.MustCompile(`/api/v\d+/`),
	// Architecture strings that look like versions
	regexp.MustCompile(`x86_64|aarch64|arm64|i686`),
	// Common tool name patterns with version-like suffixes
	regexp.MustCompile(`python[23]|go1\.\d+|ncursesw?\d+-config`),
	// Library ABI versions (e.g., libfoo.so.1.2.3)
	regexp.MustCompile(`\.so\.\d+(\.\d+)*$`),
	regexp.MustCompile(`\.\d+\.dylib$`),
}

// hasVersionPlaceholder checks if a string contains {version} or {version_tag}.
func hasVersionPlaceholder(s string) bool {
	return strings.Contains(s, "{version}") || strings.Contains(s, "{version_tag}")
}

// isExcluded checks if a value matches any exclusion pattern.
func isExcluded(value string) bool {
	for _, pattern := range excludePatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

// findVersionPattern finds the first version-like pattern in a string.
// Returns the matched version string or empty if none found.
func findVersionPattern(value string) string {
	// Check exclusions first
	if isExcluded(value) {
		return ""
	}

	for _, pattern := range versionPatterns {
		matches := pattern.FindStringSubmatch(value)
		if len(matches) >= 2 && matches[1] != "" {
			match := matches[1]
			// Additional validation for two-part versions to reduce false positives
			// Skip if it looks like part of a larger non-version context
			if len(match) < 4 && !strings.Contains(value, "-"+match) && !strings.Contains(value, match+"-") {
				// Very short match (e.g., "1.0") and not surrounded by hyphens
				// This might be a false positive, skip it
				continue
			}
			return match
		}
	}
	return ""
}

// DownloadFileVersionMismatch represents an inconsistency where a recipe has
// a dynamic version source but uses download_file with hardcoded version URLs.
type DownloadFileVersionMismatch struct {
	// Step is the 1-based step index where the mismatch was found
	Step int
	// URL is the download_file URL containing a hardcoded version
	URL string
	// DetectedVersion is the version-like pattern found in the URL
	DetectedVersion string
}

// String returns a human-readable description of the mismatch.
func (m DownloadFileVersionMismatch) String() string {
	suggested := strings.Replace(m.URL, m.DetectedVersion, "{version}", 1)
	return fmt.Sprintf("step %d (download_file): URL contains hardcoded version '%s' but recipe has dynamic version source; consider using 'download' action with URL '%s'",
		m.Step, m.DetectedVersion, suggested)
}

// hasDynamicVersionSource checks if a recipe has a dynamic version source configured.
// Returns false if the recipe only has static/pinned versions (no source, github_repo, or fossil_repo).
func hasDynamicVersionSource(r *Recipe) bool {
	v := r.Version
	return v.Source != "" || v.GitHubRepo != "" || v.FossilRepo != ""
}

// DetectDownloadFileVersionMismatch finds download_file steps with hardcoded
// version URLs when the recipe has a dynamic version source configured.
//
// This is considered inconsistent because:
// - If a recipe has a version source (e.g., homebrew), it can resolve versions dynamically
// - Using download_file with hardcoded versions defeats this capability
// - The recipe should use download action with {version} placeholder instead
//
// Recipes without a dynamic version source (e.g., using pin = "X.Y.Z") are
// intentionally static and are not flagged.
func DetectDownloadFileVersionMismatch(r *Recipe) []DownloadFileVersionMismatch {
	// Skip if no dynamic version source configured
	if !hasDynamicVersionSource(r) {
		return nil
	}

	var mismatches []DownloadFileVersionMismatch

	for stepIdx, step := range r.Steps {
		if step.Action != "download_file" {
			continue
		}

		url, ok := step.Params["url"].(string)
		if !ok || url == "" {
			continue
		}

		// Skip if already has version placeholder
		if hasVersionPlaceholder(url) {
			continue
		}

		// Check for version pattern in URL
		if version := findVersionPattern(url); version != "" {
			mismatches = append(mismatches, DownloadFileVersionMismatch{
				Step:            stepIdx + 1, // 1-based for user display
				URL:             url,
				DetectedVersion: version,
			})
		}
	}

	return mismatches
}

// DetectHardcodedVersions scans a recipe for fields that contain hardcoded
// version strings where {version} placeholders should be used.
//
// Detection is context-aware: only fields that are expected to contain version
// placeholders are checked. For example, url in "download" is checked, but url
// in "download_file" is not (static URLs are expected there).
func DetectHardcodedVersions(r *Recipe) []HardcodedVersion {
	var detected []HardcodedVersion

	for stepIdx, step := range r.Steps {
		rules, hasRules := actionVersionRules[step.Action]
		if !hasRules {
			continue
		}

		for _, rule := range rules {
			if !rule.ExpectPlaceholder {
				continue
			}

			// Get the field value
			value, ok := step.Params[rule.Field].(string)
			if !ok || value == "" {
				continue
			}

			// Fast path: skip if already has version placeholder
			if hasVersionPlaceholder(value) {
				continue
			}

			// Scan for version patterns
			if version := findVersionPattern(value); version != "" {
				detected = append(detected, HardcodedVersion{
					Step:      stepIdx + 1, // 1-based for user display
					Action:    step.Action,
					Field:     rule.Field,
					Value:     version,
					FullValue: value,
				})
			}
		}
	}

	return detected
}
