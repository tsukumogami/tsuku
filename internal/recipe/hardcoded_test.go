package recipe

import (
	"strings"
	"testing"
)

func TestHasVersionPlaceholder(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://example.com/tool-{version}.tar.gz", true},
		{"https://example.com/tool-{version_tag}.tar.gz", true},
		{"https://example.com/tool-1.2.3.tar.gz", false},
		{"tool-{version}", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := hasVersionPlaceholder(tt.input); got != tt.expected {
				t.Errorf("hasVersionPlaceholder(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindVersionPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Semver patterns
		{"semver basic", "tool-1.2.3.tar.gz", "1.2.3"},
		{"semver with v", "tool-v1.2.3.tar.gz", "v1.2.3"},
		// Prerelease/build versions - the detector finds the version but may include trailing chars
		// This is acceptable since detection (not parsing) is the goal
		{"semver prerelease", "tool-1.2.3-beta.1/file", "1.2.3-beta.1"},
		{"semver build", "tool-1.2.3+build/file", "1.2.3+build"},

		// Date-based patterns
		{"date calver", "tool-2024.01.tar.gz", "2024.01"},
		{"date calver full", "tool-2024.01.15.tar.gz", "2024.01.15"},

		// Two-part versions
		{"two-part", "curl-8.11.tar.gz", "8.11"},

		// URL with version
		{"url with version", "https://example.com/download/tool-8.11.1.tar.gz", "8.11.1"},

		// No version found
		{"no version", "https://example.com/tool.tar.gz", ""},
		{"empty", "", ""},

		// Excluded patterns - should NOT match
		{"api version", "/api/v2/download", ""},
		{"architecture x86_64", "linux-x86_64.tar.gz", ""},
		{"architecture aarch64", "darwin-aarch64.tar.gz", ""},
		{"python3", "python3-script.py", ""},
		{"go1.21", "go1.21-linux.tar.gz", ""},
		{"library abi", "libfoo.so.1.2.3", ""},
		{"dylib abi", "libfoo.1.dylib", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findVersionPattern(tt.input); got != tt.expected {
				t.Errorf("findVersionPattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetectHardcodedVersions(t *testing.T) {
	tests := []struct {
		name     string
		recipe   *Recipe
		expected int    // number of detections
		contains string // optional substring to check in first detection
	}{
		{
			name: "download with hardcoded version in url",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "download",
						Params: map[string]interface{}{
							"url": "https://curl.se/download/curl-8.11.1.tar.gz",
						},
					},
				},
			},
			expected: 1,
			contains: "8.11.1",
		},
		{
			name: "download with version placeholder - no detection",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "download",
						Params: map[string]interface{}{
							"url": "https://curl.se/download/curl-{version}.tar.gz",
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "download_file is skipped - static URLs expected",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "download_file",
						Params: map[string]interface{}{
							"url": "https://example.com/static-1.2.3.sh",
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "extract with hardcoded archive name",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "extract",
						Params: map[string]interface{}{
							"archive": "curl-8.11.1.tar.gz",
						},
					},
				},
			},
			expected: 1,
			contains: "8.11.1",
		},
		{
			name: "configure_make with hardcoded source_dir",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "configure_make",
						Params: map[string]interface{}{
							"source_dir": "curl-8.11.1",
						},
					},
				},
			},
			expected: 1,
			contains: "source_dir",
		},
		{
			name: "github_archive with hardcoded asset_pattern",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "github_archive",
						Params: map[string]interface{}{
							"repo":          "owner/repo",
							"asset_pattern": "tool-1.0.0-linux.tar.gz",
						},
					},
				},
			},
			expected: 1,
			contains: "1.0.0",
		},
		{
			name: "github_archive with wildcard pattern - no detection",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "github_archive",
						Params: map[string]interface{}{
							"repo":          "owner/repo",
							"asset_pattern": "tool-*-{version}-linux.tar.gz",
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "multiple hardcoded versions across steps",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "download",
						Params: map[string]interface{}{
							"url": "https://example.com/tool-2.0.0.tar.gz",
						},
					},
					{
						Action: "extract",
						Params: map[string]interface{}{
							"archive": "tool-2.0.0.tar.gz",
						},
					},
					{
						Action: "configure_make",
						Params: map[string]interface{}{
							"source_dir": "tool-2.0.0",
						},
					},
				},
			},
			expected: 3,
		},
		{
			name: "action without rules is skipped",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "chmod",
						Params: map[string]interface{}{
							"path": "tool-1.2.3/bin/tool",
						},
					},
				},
			},
			expected: 0,
		},
		{
			name: "checksum_url with hardcoded version",
			recipe: &Recipe{
				Steps: []Step{
					{
						Action: "download",
						Params: map[string]interface{}{
							"url":          "https://example.com/tool-{version}.tar.gz",
							"checksum_url": "https://example.com/checksums-1.2.3.txt",
						},
					},
				},
			},
			expected: 1,
			contains: "checksum_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := DetectHardcodedVersions(tt.recipe)
			if len(detected) != tt.expected {
				t.Errorf("DetectHardcodedVersions() returned %d detections, want %d", len(detected), tt.expected)
				for _, d := range detected {
					t.Logf("  detected: %s", d.String())
				}
				return
			}

			if tt.expected > 0 && tt.contains != "" {
				found := false
				for _, d := range detected {
					if strings.Contains(d.String(), tt.contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("detection should contain %q, got: %v", tt.contains, detected)
				}
			}
		})
	}
}

func TestHardcodedVersionString(t *testing.T) {
	h := HardcodedVersion{
		Step:      1,
		Action:    "download",
		Field:     "url",
		Value:     "8.11.1",
		FullValue: "https://curl.se/download/curl-8.11.1.tar.gz",
	}

	str := h.String()

	// Check all expected components are present
	if !strings.Contains(str, "step 1") {
		t.Errorf("String() should contain step number, got: %s", str)
	}
	if !strings.Contains(str, "download") {
		t.Errorf("String() should contain action name, got: %s", str)
	}
	if !strings.Contains(str, "url") {
		t.Errorf("String() should contain field name, got: %s", str)
	}
	if !strings.Contains(str, "8.11.1") {
		t.Errorf("String() should contain version, got: %s", str)
	}
	if !strings.Contains(str, "{version}") {
		t.Errorf("String() should contain suggested fix with {version}, got: %s", str)
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		input    string
		excluded bool
	}{
		{"/api/v2/download", true},
		{"/api/v3/resource", true},
		{"linux-x86_64.tar.gz", true},
		{"darwin-aarch64.tar.gz", true},
		{"linux-arm64.tar.gz", true},
		{"python3", true},
		{"go1.21", true},
		{"ncursesw6-config", true},
		{"libfoo.so.1.2.3", true},
		{"libbar.6.dylib", true},
		{"tool-1.2.3.tar.gz", false},
		{"https://example.com/download/tool.tar.gz", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isExcluded(tt.input); got != tt.excluded {
				t.Errorf("isExcluded(%q) = %v, want %v", tt.input, got, tt.excluded)
			}
		})
	}
}
