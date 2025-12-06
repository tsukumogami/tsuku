package version

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestValidateVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		// Valid cases
		{"simple semver", "1.2.3", false},
		{"with v prefix", "v1.2.3", false},
		{"with prerelease", "1.2.3-rc.1", false},
		{"with build", "1.2.3+build.123", false},
		{"with prerelease and build", "1.2.3-beta+build", false},
		{"alphanumeric", "go1.21.0", false},
		{"underscores", "1.2.3_alpha", false},
		{"dots only", "1.2.3.4", false},
		{"dashes", "1.2.3-4-5", false},
		{"plus signs", "1.2.3+4+5", false},

		// Invalid cases
		{"empty string", "", true},
		{"contains space", "1.2.3 beta", true},
		{"contains newline", "1.2.3\n", true},
		{"contains semicolon", "1.2.3;echo", true},
		{"contains pipe", "1.2.3|cat", true},
		{"contains dollar", "1.2.3$var", true},
		{"contains backtick", "1.2.3`cmd`", true},
		{"contains parentheses", "1.2.3()", true},
		{"contains brackets", "1.2.3[]", true},
		{"contains quotes", "1.2.3\"", true},
		{"contains ampersand", "1.2.3&", true},
		// @ and / are allowed to support scoped package versions like @biomejs/biome@2.3.8
		{"contains at sign", "biome@1.2.3", false},
		{"scoped package", "@biomejs/biome@2.3.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersionString(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersionString(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersionString_MaxLength(t *testing.T) {
	// Exactly at max length - should pass
	validVersion := strings.Repeat("1", MaxVersionLength)
	if err := ValidateVersionString(validVersion); err != nil {
		t.Errorf("ValidateVersionString() with max length should pass, got error: %v", err)
	}

	// One over max length - should fail
	tooLong := strings.Repeat("1", MaxVersionLength+1)
	if err := ValidateVersionString(tooLong); err == nil {
		t.Error("ValidateVersionString() with overlength should fail, got nil")
	}
}

func TestTransformVersion_Raw(t *testing.T) {
	tests := []struct {
		version string
		format  string
	}{
		{"1.2.3", recipe.VersionFormatRaw},
		{"v1.2.3", recipe.VersionFormatRaw},
		{"go1.21.0", recipe.VersionFormatRaw},
		{"1.2.3", ""}, // Empty format defaults to raw
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result, err := TransformVersion(tt.version, tt.format)
			if err != nil {
				t.Errorf("TransformVersion(%q, %q) error = %v", tt.version, tt.format, err)
			}
			if result != tt.version {
				t.Errorf("TransformVersion(%q, %q) = %q, want %q", tt.version, tt.format, result, tt.version)
			}
		})
	}
}

func TestTransformVersion_Semver(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{"simple semver", "1.2.3", "1.2.3", false},
		{"with v prefix", "v1.2.3", "1.2.3", false},
		{"with prerelease stripped", "v1.2.3-rc.1", "1.2.3", false},
		{"with build stripped", "1.2.3+build", "1.2.3", false},
		{"biome format", "biome-2.3.8", "2.3.8", false},
		{"go format", "go1.21.0", "1.21.0", false},
		{"complex prefix", "tool-name-v2.3.8-linux", "2.3.8", false},
		{"no semver pattern", "latest", "latest", true},
		{"only major.minor", "1.2", "1.2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TransformVersion(tt.version, recipe.VersionFormatSemver)
			if (err != nil) != tt.wantErr {
				t.Errorf("TransformVersion(%q, semver) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
			if result != tt.want {
				t.Errorf("TransformVersion(%q, semver) = %q, want %q", tt.version, result, tt.want)
			}
		})
	}
}

func TestTransformVersion_SemverFull(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{"simple semver", "1.2.3", "1.2.3", false},
		{"with v prefix", "v1.2.3", "1.2.3", false},
		{"with prerelease", "v1.2.3-rc.1", "1.2.3-rc.1", false},
		{"with build", "v1.2.3+build.123", "1.2.3+build.123", false},
		{"with both", "v1.2.3-beta.1+build", "1.2.3-beta.1+build", false},
		{"complex prefix", "tool-1.2.3-alpha", "1.2.3-alpha", false},
		{"no semver pattern", "latest", "latest", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TransformVersion(tt.version, recipe.VersionFormatSemverFull)
			if (err != nil) != tt.wantErr {
				t.Errorf("TransformVersion(%q, semver_full) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
			if result != tt.want {
				t.Errorf("TransformVersion(%q, semver_full) = %q, want %q", tt.version, result, tt.want)
			}
		})
	}
}

func TestTransformVersion_StripV(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"lowercase v", "v1.2.3", "1.2.3"},
		{"uppercase V", "V1.2.3", "1.2.3"},
		{"no v prefix", "1.2.3", "1.2.3"},
		{"v in middle", "1v2.3", "1v2.3"},
		{"only v", "v", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TransformVersion(tt.version, recipe.VersionFormatStripV)
			if err != nil {
				t.Errorf("TransformVersion(%q, strip_v) error = %v", tt.version, err)
			}
			if result != tt.want {
				t.Errorf("TransformVersion(%q, strip_v) = %q, want %q", tt.version, result, tt.want)
			}
		})
	}
}

func TestTransformVersion_UnknownFormat(t *testing.T) {
	// Unknown formats should be treated as raw (forward compatibility)
	version := "v1.2.3"
	result, err := TransformVersion(version, "future_format")
	if err != nil {
		t.Errorf("TransformVersion with unknown format should not error, got: %v", err)
	}
	if result != version {
		t.Errorf("TransformVersion with unknown format = %q, want %q", result, version)
	}
}

func TestTransformVersion_InvalidInput(t *testing.T) {
	// Invalid version strings should return error regardless of format
	invalidVersions := []string{
		"",
		"1.2.3 space",
		"1.2.3;cmd",
		"1.2.3|pipe",
	}

	formats := []string{
		recipe.VersionFormatRaw,
		recipe.VersionFormatSemver,
		recipe.VersionFormatSemverFull,
		recipe.VersionFormatStripV,
	}

	for _, version := range invalidVersions {
		for _, format := range formats {
			t.Run(version+"_"+format, func(t *testing.T) {
				_, err := TransformVersion(version, format)
				if err == nil {
					t.Errorf("TransformVersion(%q, %q) should error for invalid input", version, format)
				}
			})
		}
	}
}

func TestTransformVersion_RealWorldExamples(t *testing.T) {
	// Real-world version strings from the design document
	tests := []struct {
		name    string
		version string
		format  string
		want    string
	}{
		// biome@2.3.8 → 2.3.8 (@ is now allowed in version strings)
		{"biome format", "biome@2.3.8", recipe.VersionFormatSemver, "2.3.8"},
		// Scoped npm package format
		{"scoped package format", "@biomejs/biome@2.3.8", recipe.VersionFormatSemver, "2.3.8"},

		// v1.29.0 → 1.29.0
		{"strip v prefix", "v1.29.0", recipe.VersionFormatStripV, "1.29.0"},

		// 2.4.0-0 → 2.4.0 (semver strips prerelease)
		{"strip prerelease", "2.4.0-0", recipe.VersionFormatSemver, "2.4.0"},

		// Go version format
		{"go version", "go1.21.0", recipe.VersionFormatSemver, "1.21.0"},

		// Keep prerelease with semver_full
		{"keep prerelease", "v1.2.3-rc.1", recipe.VersionFormatSemverFull, "1.2.3-rc.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TransformVersion(tt.version, tt.format)
			if err != nil {
				t.Errorf("TransformVersion(%q, %q) error = %v", tt.version, tt.format, err)
			}
			if result != tt.want {
				t.Errorf("TransformVersion(%q, %q) = %q, want %q", tt.version, tt.format, result, tt.want)
			}
		})
	}
}
