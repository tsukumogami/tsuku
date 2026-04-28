package version

import "testing"

// TestIsStableVersion_DefaultQualifiers covers the predicate's behavior
// against DefaultStableQualifiers, which is what the GitHub provider uses
// when a recipe declares no stable_qualifiers field.
func TestIsStableVersion_DefaultQualifiers(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		// Plain semver — always stable.
		{"plain semver", "1.0.0", true},
		{"plain semver with two parts", "2.34", true},
		{"plain semver with calver-like core", "2024.1.15", true},

		// SemVer prerelease keywords — always unstable.
		{"alpha", "1.0.0-alpha", false},
		{"alpha numbered", "1.0.0-alpha.1", false},
		{"beta", "1.0.0-beta", false},
		{"rc", "1.0.0-rc.1", false},
		{"dev", "1.0.0-dev", false},

		// Milestone tags (the bug that triggered this design).
		{"gradle milestone uppercase", "9.6.0-M1", false},
		{"sbt milestone uppercase", "2.0.0-M5", false},
		{"milestone lowercase", "1.0.0-m2", false},

		// Default stable qualifiers — admitted.
		{"RELEASE uppercase", "5.3.39-RELEASE", true},
		{"release lowercase", "5.3.39-release", true},
		{"FINAL uppercase", "5.6.15-FINAL", true},
		{"Final mixed case", "5.6.15-Final", true},
		{"LTS", "1.0.0-LTS", true},
		{"GA", "1.0.0-GA", true},
		{"stable", "1.0.0-stable", true},

		// Compound suffixes are not in the allowlist (exact match only).
		{"compound final.1", "1.0.0-final.1", false},
		{"compound RELEASE-hotfix", "1.0.0-RELEASE-hotfix", false},

		// Build metadata after `+` is stripped before checking.
		{"plain with build metadata", "1.0.0+build.123", true},
		{"prerelease with build metadata", "1.0.0-rc.1+build.5", false},

		// SemVer-style variants the old keyword filter caught — still
		// rejected, now via the prerelease check.
		{"snapshot", "1.0.0-SNAPSHOT", false},
		{"nightly", "1.0.0-nightly", false},
		{"preview", "1.0.0-preview", false},

		// Non-SemVer prerelease markers spliced into the version without
		// a hyphen (e.g., jq's "1.8.2rc1"). Caught by the fallback
		// substring check.
		{"non-semver rc", "1.8.2rc1", false},
		{"non-semver beta", "2.0.0beta1", false},
		{"non-semver alpha", "1.0.0alpha", false},
		{"non-semver SNAPSHOT", "1.0SNAPSHOT", false},

		// Edge case: empty version string. The current splitPrerelease
		// returns ("", "") for empty input, so the predicate treats it as
		// stable. Callers are expected to never pass empty strings; this
		// case pins the behavior so a future refactor doesn't silently
		// change it.
		{"empty string", "", true},
	}

	stableQualifiers := buildStableQualifierSet(nil) // nil → DefaultStableQualifiers

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStableVersion(tt.version, stableQualifiers)
			if got != tt.want {
				t.Errorf("isStableVersion(%q, default) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

// TestIsStableVersion_RecipeOverride covers the case where a recipe declares
// its own stable_qualifiers list, which replaces (not extends) the default.
func TestIsStableVersion_RecipeOverride(t *testing.T) {
	tests := []struct {
		name             string
		stableQualifiers []string
		version          string
		want             bool
	}{
		// Recipe limits qualifiers to {"release"} only.
		{"override admits release", []string{"release"}, "1.0.0-RELEASE", true},
		{"override rejects final not in list", []string{"release"}, "1.0.0-FINAL", false},
		{"override rejects lts not in list", []string{"release"}, "1.0.0-LTS", false},

		// Recipe declares an exotic qualifier.
		{"exotic qualifier admitted", []string{"prod"}, "1.0.0-PROD", true},
		{"exotic qualifier rejects defaults", []string{"prod"}, "1.0.0-RELEASE", false},

		// Strict-SemVer-only recipe: empty list should NOT collapse to
		// default; an explicit empty list is treated like nil today, but
		// that is a documented limitation rather than a desired feature.
		// This test pins the current behavior for visibility.
		{"empty list falls back to default", []string{}, "1.0.0-RELEASE", true},

		// Plain semver always stable regardless of qualifiers.
		{"plain semver with empty list", []string{}, "1.0.0", true},
		{"plain semver with custom list", []string{"prod"}, "1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStableVersion(tt.version, buildStableQualifierSet(tt.stableQualifiers))
			if got != tt.want {
				t.Errorf("isStableVersion(%q, %v) = %v, want %v",
					tt.version, tt.stableQualifiers, got, tt.want)
			}
		})
	}
}

// TestBuildStableQualifierSet covers the set construction behavior:
// nil and empty slices both fall back to DefaultStableQualifiers, and
// the lookup is case-insensitive (keys are lowercased on construction).
func TestBuildStableQualifierSet(t *testing.T) {
	t.Run("nil falls back to default", func(t *testing.T) {
		set := buildStableQualifierSet(nil)
		if !set["release"] {
			t.Errorf("nil input should fall back to DefaultStableQualifiers (missing 'release')")
		}
		if !set["final"] {
			t.Errorf("nil input should fall back to DefaultStableQualifiers (missing 'final')")
		}
	})

	t.Run("empty slice falls back to default", func(t *testing.T) {
		set := buildStableQualifierSet([]string{})
		if !set["release"] {
			t.Errorf("empty input should fall back to DefaultStableQualifiers (missing 'release')")
		}
	})

	t.Run("explicit list lowercases on insert", func(t *testing.T) {
		set := buildStableQualifierSet([]string{"PROD", "Final"})
		if !set["prod"] {
			t.Errorf("expected 'prod' in set, got: %v", set)
		}
		if !set["final"] {
			t.Errorf("expected 'final' in set, got: %v", set)
		}
		if set["release"] {
			t.Errorf("default 'release' should not be present when override is set, got: %v", set)
		}
	})
}
