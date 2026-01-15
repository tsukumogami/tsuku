package version

import (
	"reflect"
	"testing"
)

func TestSortVersionsDescending(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single version",
			input:    []string{"1.0.0"},
			expected: []string{"1.0.0"},
		},
		{
			name:     "already sorted",
			input:    []string{"3.0.0", "2.0.0", "1.0.0"},
			expected: []string{"3.0.0", "2.0.0", "1.0.0"},
		},
		{
			name:     "reverse order",
			input:    []string{"1.0.0", "2.0.0", "3.0.0"},
			expected: []string{"3.0.0", "2.0.0", "1.0.0"},
		},
		{
			name:     "mixed semver versions",
			input:    []string{"v1.9.0", "v1.26.0", "v1.3.0", "v1.7.0"},
			expected: []string{"v1.26.0", "v1.9.0", "v1.7.0", "v1.3.0"},
		},
		{
			name:     "with prereleases",
			input:    []string{"1.0.0-alpha", "1.0.0", "1.0.0-rc.1", "1.0.0-beta"},
			expected: []string{"1.0.0", "1.0.0-rc.1", "1.0.0-beta", "1.0.0-alpha"},
		},
		{
			name:     "calver versions",
			input:    []string{"2024.01.15", "2023.12.01", "2024.06.30"},
			expected: []string{"2024.06.30", "2024.01.15", "2023.12.01"},
		},
		{
			name:     "go versions",
			input:    []string{"go1.20.1", "go1.21.5", "go1.19.0"},
			expected: []string{"go1.21.5", "go1.20.1", "go1.19.0"},
		},
		{
			name:     "dlv-like unsorted input",
			input:    []string{"v1.9.0", "v1.3.0", "v1.7.0", "v0.7.0-alpha", "v1.26.0"},
			expected: []string{"v1.26.0", "v1.9.0", "v1.7.0", "v1.3.0", "v0.7.0-alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to verify input is not modified
			inputCopy := make([]string, len(tt.input))
			copy(inputCopy, tt.input)

			result := SortVersionsDescending(tt.input)

			// Check result
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("SortVersionsDescending() = %v, want %v", result, tt.expected)
			}

			// Verify input was not modified
			if !reflect.DeepEqual(tt.input, inputCopy) {
				t.Errorf("SortVersionsDescending() modified input: got %v, original %v", tt.input, inputCopy)
			}
		})
	}
}

func TestIsSortedDescending(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		expected bool
	}{
		{
			name:     "empty",
			versions: []string{},
			expected: true,
		},
		{
			name:     "single",
			versions: []string{"1.0.0"},
			expected: true,
		},
		{
			name:     "sorted descending",
			versions: []string{"3.0.0", "2.0.0", "1.0.0"},
			expected: true,
		},
		{
			name:     "sorted ascending",
			versions: []string{"1.0.0", "2.0.0", "3.0.0"},
			expected: false,
		},
		{
			name:     "unsorted",
			versions: []string{"2.0.0", "3.0.0", "1.0.0"},
			expected: false,
		},
		{
			name:     "with prereleases sorted",
			versions: []string{"1.0.0", "1.0.0-rc.1", "1.0.0-alpha"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSortedDescending(tt.versions)
			if result != tt.expected {
				t.Errorf("IsSortedDescending(%v) = %v, want %v", tt.versions, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions_Prereleases(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// Stable vs prerelease
		{
			name:     "stable > alpha",
			v1:       "1.0.0",
			v2:       "1.0.0-alpha",
			expected: 1,
		},
		{
			name:     "stable > beta",
			v1:       "1.0.0",
			v2:       "1.0.0-beta",
			expected: 1,
		},
		{
			name:     "stable > rc",
			v1:       "1.0.0",
			v2:       "1.0.0-rc.1",
			expected: 1,
		},
		{
			name:     "alpha < stable",
			v1:       "1.0.0-alpha",
			v2:       "1.0.0",
			expected: -1,
		},

		// Prerelease ordering
		{
			name:     "alpha < beta",
			v1:       "1.0.0-alpha",
			v2:       "1.0.0-beta",
			expected: -1,
		},
		{
			name:     "beta < rc",
			v1:       "1.0.0-beta",
			v2:       "1.0.0-rc",
			expected: -1,
		},
		{
			name:     "rc.1 < rc.2",
			v1:       "1.0.0-rc.1",
			v2:       "1.0.0-rc.2",
			expected: -1,
		},

		// With v prefix
		{
			name:     "v prefix stable > alpha",
			v1:       "v1.0.0",
			v2:       "v1.0.0-alpha",
			expected: 1,
		},

		// Different core versions with prereleases
		{
			name:     "1.1.0-alpha > 1.0.0",
			v1:       "1.1.0-alpha",
			v2:       "1.0.0",
			expected: 1,
		},
		{
			name:     "1.0.0-alpha < 1.1.0-alpha",
			v1:       "1.0.0-alpha",
			v2:       "1.1.0-alpha",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions_Normalization(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// v prefix handling
		{
			name:     "v prefix equal",
			v1:       "v1.0.0",
			v2:       "1.0.0",
			expected: 0,
		},
		{
			name:     "v1.2.3 > v1.2.2",
			v1:       "v1.2.3",
			v2:       "v1.2.2",
			expected: 1,
		},

		// go prefix handling
		{
			name:     "go version comparison",
			v1:       "go1.21.5",
			v2:       "go1.20.1",
			expected: 1,
		},
		{
			name:     "go prefix normalized equal",
			v1:       "go1.21.0",
			v2:       "1.21.0",
			expected: 0,
		},

		// Release_ format
		{
			name:     "Release format",
			v1:       "Release_1_15_0",
			v2:       "Release_1_14_0",
			expected: 1,
		},

		// Multi-part tags
		{
			name:     "kustomize-style tag",
			v1:       "kustomize/v5.7.1",
			v2:       "kustomize/v5.6.0",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions_BuildMetadata(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "build metadata ignored",
			v1:       "1.0.0+build.123",
			v2:       "1.0.0+build.456",
			expected: 0,
		},
		{
			name:     "build metadata with different versions",
			v1:       "1.0.1+build.1",
			v2:       "1.0.0+build.999",
			expected: 1,
		},
		{
			name:     "build metadata vs no metadata",
			v1:       "1.0.0+build.1",
			v2:       "1.0.0",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestSplitPrerelease(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		expectedCore string
		expectedPre  string
	}{
		{
			name:         "no prerelease",
			version:      "1.0.0",
			expectedCore: "1.0.0",
			expectedPre:  "",
		},
		{
			name:         "with prerelease",
			version:      "1.0.0-rc.1",
			expectedCore: "1.0.0",
			expectedPre:  "rc.1",
		},
		{
			name:         "with build metadata",
			version:      "1.0.0+build.123",
			expectedCore: "1.0.0",
			expectedPre:  "",
		},
		{
			name:         "with prerelease and build metadata",
			version:      "1.0.0-rc.1+build.123",
			expectedCore: "1.0.0",
			expectedPre:  "rc.1",
		},
		{
			name:         "alpha prerelease",
			version:      "1.0.0-alpha",
			expectedCore: "1.0.0",
			expectedPre:  "alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, pre := splitPrerelease(tt.version)
			if core != tt.expectedCore {
				t.Errorf("splitPrerelease(%q) core = %q, want %q", tt.version, core, tt.expectedCore)
			}
			if pre != tt.expectedPre {
				t.Errorf("splitPrerelease(%q) prerelease = %q, want %q", tt.version, pre, tt.expectedPre)
			}
		})
	}
}
