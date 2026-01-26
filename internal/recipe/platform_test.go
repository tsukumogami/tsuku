package recipe

import (
	"testing"
)

func TestSupportsPlatform(t *testing.T) {
	tests := []struct {
		name            string
		supportedOS     []string
		supportedArch   []string
		unsupportedPlat []string
		targetOS        string
		targetArch      string
		expectedSupport bool
		description     string
	}{
		{
			name:            "missing fields support all platforms",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "Recipe without constraints should support all platforms",
		},
		{
			name:            "empty arrays override to empty set",
			supportedOS:     []string{},
			supportedArch:   []string{},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			expectedSupport: false,
			description:     "Empty arrays should mean no platforms supported",
		},
		{
			name:            "OS-only constraint (linux)",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "Linux on any arch should be supported",
		},
		{
			name:            "OS-only constraint (darwin rejected)",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			unsupportedPlat: nil,
			targetOS:        "darwin",
			targetArch:      "amd64",
			expectedSupport: false,
			description:     "macOS should be rejected when only Linux is supported",
		},
		{
			name:            "arch-only constraint (amd64)",
			supportedOS:     nil,
			supportedArch:   []string{"amd64"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "amd64 on any OS should be supported",
		},
		{
			name:            "arch-only constraint (arm64 rejected)",
			supportedOS:     nil,
			supportedArch:   []string{"amd64"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "arm64",
			expectedSupport: false,
			description:     "arm64 should be rejected when only amd64 is supported",
		},
		{
			name:            "denylist-only (darwin/arm64 rejected)",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			targetOS:        "darwin",
			targetArch:      "arm64",
			expectedSupport: false,
			description:     "macOS ARM64 should be rejected when in denylist",
		},
		{
			name:            "denylist-only (darwin/amd64 allowed)",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			targetOS:        "darwin",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "macOS x86_64 should be allowed when only ARM64 is denied",
		},
		{
			name:            "combined allowlist + denylist",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			targetOS:        "darwin",
			targetArch:      "arm64",
			expectedSupport: false,
			description:     "macOS ARM64 should be rejected even though darwin is in allowlist",
		},
		{
			name:            "combined allowlist + denylist (allowed case)",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			targetOS:        "darwin",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "macOS x86_64 should be allowed",
		},
		{
			name:            "combined allowlist + denylist (linux allowed)",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			targetOS:        "linux",
			targetArch:      "arm64",
			expectedSupport: true,
			description:     "Linux ARM64 should be allowed (only darwin/arm64 is denied)",
		},
		{
			name:            "multiple OS and arch constraints",
			supportedOS:     []string{"linux", "darwin", "windows"},
			supportedArch:   []string{"amd64", "arm64"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "Linux amd64 should be in the Cartesian product",
		},
		{
			name:            "multiple OS and arch constraints (rejected)",
			supportedOS:     []string{"linux", "darwin", "windows"},
			supportedArch:   []string{"amd64", "arm64"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "386",
			expectedSupport: false,
			description:     "Linux 386 should be rejected (not in arch allowlist)",
		},
		{
			name:            "multiple denylist entries",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64", "windows/arm64"},
			targetOS:        "windows",
			targetArch:      "arm64",
			expectedSupport: false,
			description:     "Windows ARM64 should be rejected (in denylist)",
		},
		{
			name:            "complex: allowlist + multiple denies",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   []string{"amd64", "arm64"},
			unsupportedPlat: []string{"darwin/arm64", "linux/arm64"},
			targetOS:        "darwin",
			targetArch:      "amd64",
			expectedSupport: true,
			description:     "macOS amd64 should be allowed (in Cartesian product, not denied)",
		},
		{
			name:            "complex: allowlist + multiple denies (rejected)",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   []string{"amd64", "arm64"},
			unsupportedPlat: []string{"darwin/arm64", "linux/arm64"},
			targetOS:        "linux",
			targetArch:      "arm64",
			expectedSupport: false,
			description:     "Linux ARM64 should be rejected (denied)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:          tt.supportedOS,
					SupportedArch:        tt.supportedArch,
					UnsupportedPlatforms: tt.unsupportedPlat,
				},
			}

			result := r.SupportsPlatform(tt.targetOS, tt.targetArch)
			if result != tt.expectedSupport {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedSupport, result)
			}
		})
	}
}

func TestSupportsPlatformRuntime(t *testing.T) {
	// Test the convenience method that uses runtime.GOOS and runtime.GOARCH
	r := &Recipe{
		Metadata: MetadataSection{}, // No constraints
	}

	// Should support current runtime
	if !r.SupportsPlatformRuntime() {
		t.Error("Recipe without constraints should support current runtime platform")
	}
}

func TestValidatePlatformConstraints(t *testing.T) {
	tests := []struct {
		name            string
		supportedOS     []string
		supportedArch   []string
		unsupportedPlat []string
		expectError     bool
		expectWarnings  int
		description     string
	}{
		{
			name:            "valid constraints (no warnings)",
			supportedOS:     []string{"linux"},
			supportedArch:   []string{"amd64"},
			unsupportedPlat: nil,
			expectError:     false,
			expectWarnings:  0,
			description:     "Simple Linux+amd64 constraint should be valid",
		},
		{
			name:            "valid denylist",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			expectError:     false,
			expectWarnings:  0,
			description:     "Denying darwin/arm64 when darwin is in allowlist should be valid",
		},
		{
			name:            "warning: no-op exclusion",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			expectError:     false,
			expectWarnings:  1,
			description:     "Denying darwin/arm64 when only linux is allowed should warn",
		},
		{
			name:            "multiple no-op exclusions",
			supportedOS:     []string{"linux"},
			supportedArch:   []string{"amd64"},
			unsupportedPlat: []string{"darwin/arm64", "windows/386"},
			expectError:     false,
			expectWarnings:  2,
			description:     "Multiple no-op exclusions should each generate a warning",
		},
		{
			name:            "error: empty result set",
			supportedOS:     []string{"linux"},
			supportedArch:   []string{"arm64"},
			unsupportedPlat: []string{"linux/arm64"},
			expectError:     true,
			expectWarnings:  0,
			description:     "Excluding the only allowed platform should error",
		},
		{
			name:            "error: empty allowlist",
			supportedOS:     []string{},
			supportedArch:   []string{},
			unsupportedPlat: nil,
			expectError:     true,
			expectWarnings:  0,
			description:     "Empty allowlists should result in error (no platforms)",
		},
		{
			name:            "no constraints (valid)",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: nil,
			expectError:     false,
			expectWarnings:  0,
			description:     "Recipe without constraints should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:          tt.supportedOS,
					SupportedArch:        tt.supportedArch,
					UnsupportedPlatforms: tt.unsupportedPlat,
				},
			}

			warnings, err := r.ValidatePlatformConstraints()

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("%s: expected error, got nil", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: expected no error, got: %v", tt.description, err)
			}

			// Check warning count
			if len(warnings) != tt.expectWarnings {
				t.Errorf("%s: expected %d warnings, got %d", tt.description, tt.expectWarnings, len(warnings))
			}
		})
	}
}

func TestGetSupportedPlatforms(t *testing.T) {
	tests := []struct {
		name             string
		supportedOS      []string
		supportedArch    []string
		unsupportedPlat  []string
		minExpected      int
		shouldContain    []string
		shouldNotContain []string
		description      string
	}{
		{
			name:             "no constraints returns tsuku-supported platforms",
			supportedOS:      nil,
			supportedArch:    nil,
			unsupportedPlat:  nil,
			minExpected:      4, // tsuku supports: (linux, darwin) × (amd64, arm64) = 4 platforms
			shouldContain:    []string{"linux/amd64", "darwin/arm64", "linux/arm64", "darwin/amd64"},
			shouldNotContain: []string{"windows/amd64", "freebsd/amd64"},
			description:      "Recipe without constraints should return tsuku-supported platform combinations",
		},
		{
			name:             "OS-only constraint",
			supportedOS:      []string{"linux"},
			supportedArch:    nil,
			unsupportedPlat:  nil,
			minExpected:      2, // linux with tsuku-supported archs (amd64, arm64)
			shouldContain:    []string{"linux/amd64", "linux/arm64"},
			shouldNotContain: []string{"darwin/amd64", "windows/amd64"},
			description:      "Linux-only should include linux × tsuku-supported arch combinations",
		},
		{
			name:             "denylist exclusion",
			supportedOS:      []string{"linux", "darwin"},
			supportedArch:    nil,
			unsupportedPlat:  []string{"darwin/arm64"},
			minExpected:      3, // 4 platforms - 1 excluded = 3
			shouldContain:    []string{"linux/amd64", "darwin/amd64", "linux/arm64"},
			shouldNotContain: []string{"darwin/arm64"},
			description:      "Should exclude darwin/arm64 but include other platform combinations",
		},
		{
			name:             "specific OS and arch",
			supportedOS:      []string{"linux"},
			supportedArch:    []string{"amd64", "arm64"},
			unsupportedPlat:  nil,
			minExpected:      2,
			shouldContain:    []string{"linux/amd64", "linux/arm64"},
			shouldNotContain: []string{"linux/386", "darwin/amd64"},
			description:      "Should only include specified OS/arch combinations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:          tt.supportedOS,
					SupportedArch:        tt.supportedArch,
					UnsupportedPlatforms: tt.unsupportedPlat,
				},
			}

			platforms := r.GetSupportedPlatforms()

			// Check minimum count
			if len(platforms) < tt.minExpected {
				t.Errorf("%s: expected at least %d platforms, got %d",
					tt.description, tt.minExpected, len(platforms))
			}

			// Check should contain
			for _, expected := range tt.shouldContain {
				found := false
				for _, platform := range platforms {
					if platform == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: expected to contain %s, but not found",
						tt.description, expected)
				}
			}

			// Check should not contain
			for _, notExpected := range tt.shouldNotContain {
				for _, platform := range platforms {
					if platform == notExpected {
						t.Errorf("%s: should not contain %s, but found it",
							tt.description, notExpected)
					}
				}
			}
		})
	}
}

func TestUnsupportedPlatformError(t *testing.T) {
	tests := []struct {
		name                 string
		recipeName           string
		currentOS            string
		currentArch          string
		supportedOS          []string
		supportedArch        []string
		unsupportedPlatforms []string
		expectedSubstrings   []string
		description          string
	}{
		{
			name:                 "simple OS constraint",
			recipeName:           "hello-nix",
			currentOS:            "darwin",
			currentArch:          "arm64",
			supportedOS:          []string{"linux"},
			supportedArch:        nil,
			unsupportedPlatforms: nil,
			expectedSubstrings: []string{
				"hello-nix",
				"darwin/arm64",
				"Allowed: linux OS",
				"all arch",
			},
			description: "Should show recipe name, current platform, and constraints",
		},
		{
			name:                 "with denylist",
			recipeName:           "btop",
			currentOS:            "darwin",
			currentArch:          "arm64",
			supportedOS:          []string{"linux", "darwin"},
			supportedArch:        nil,
			unsupportedPlatforms: []string{"darwin/arm64"},
			expectedSubstrings: []string{
				"btop",
				"darwin/arm64",
				"Allowed: linux, darwin OS",
				"Except: darwin/arm64",
			},
			description: "Should show allowlist and denylist",
		},
		{
			name:                 "all platforms (empty constraints)",
			recipeName:           "tool",
			currentOS:            "linux",
			currentArch:          "amd64",
			supportedOS:          nil,
			supportedArch:        nil,
			unsupportedPlatforms: nil,
			expectedSubstrings: []string{
				"tool",
				"linux/amd64",
			},
			description: "Should show recipe name and current platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &UnsupportedPlatformError{
				RecipeName:           tt.recipeName,
				CurrentOS:            tt.currentOS,
				CurrentArch:          tt.currentArch,
				SupportedOS:          tt.supportedOS,
				SupportedArch:        tt.supportedArch,
				UnsupportedPlatforms: tt.unsupportedPlatforms,
			}

			errMsg := err.Error()

			for _, substr := range tt.expectedSubstrings {
				if !contains(errMsg, substr) {
					t.Errorf("%s: expected substring '%s' in error message:\n%s",
						tt.description, substr, errMsg)
				}
			}
		})
	}
}

func TestFormatPlatformConstraints(t *testing.T) {
	tests := []struct {
		name            string
		supportedOS     []string
		supportedArch   []string
		unsupportedPlat []string
		expectedSubstr  string
		description     string
	}{
		{
			name:            "no constraints",
			supportedOS:     nil,
			supportedArch:   nil,
			unsupportedPlat: nil,
			expectedSubstr:  "all platforms",
			description:     "Recipe without constraints should say 'all platforms'",
		},
		{
			name:            "OS-only constraint",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			unsupportedPlat: nil,
			expectedSubstr:  "OS: linux",
			description:     "Should show OS constraint",
		},
		{
			name:            "arch-only constraint",
			supportedOS:     nil,
			supportedArch:   []string{"amd64", "arm64"},
			unsupportedPlat: nil,
			expectedSubstr:  "Arch: amd64, arm64",
			description:     "Should show arch constraint",
		},
		{
			name:            "with denylist",
			supportedOS:     []string{"linux", "darwin"},
			supportedArch:   nil,
			unsupportedPlat: []string{"darwin/arm64"},
			expectedSubstr:  "Except: darwin/arm64",
			description:     "Should show exception",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:          tt.supportedOS,
					SupportedArch:        tt.supportedArch,
					UnsupportedPlatforms: tt.unsupportedPlat,
				},
			}

			result := r.FormatPlatformConstraints()
			// Use the contains function from types_test.go for substring checking
			if !contains(result, tt.expectedSubstr) {
				t.Errorf("%s: expected substring '%s' in '%s'",
					tt.description, tt.expectedSubstr, result)
			}
		})
	}
}

func TestSupportsPlatformWithLibc(t *testing.T) {
	tests := []struct {
		name            string
		supportedOS     []string
		supportedArch   []string
		supportedLibc   []string
		unsupportedPlat []string
		targetOS        string
		targetArch      string
		targetLibc      string
		expectedSupport bool
		description     string
	}{
		{
			name:            "no libc constraint - all allowed",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   nil,
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "musl",
			expectedSupport: true,
			description:     "Empty supported_libc should allow all libc types",
		},
		{
			name:            "glibc-only constraint - glibc allowed",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "glibc",
			expectedSupport: true,
			description:     "glibc should be allowed when in supported_libc",
		},
		{
			name:            "glibc-only constraint - musl rejected",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "musl",
			expectedSupport: false,
			description:     "musl should be rejected when only glibc is supported",
		},
		{
			name:            "musl-only constraint - musl allowed",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"musl"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "musl",
			expectedSupport: true,
			description:     "musl should be allowed when in supported_libc",
		},
		{
			name:            "musl-only constraint - glibc rejected",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"musl"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "glibc",
			expectedSupport: false,
			description:     "glibc should be rejected when only musl is supported",
		},
		{
			name:            "both libc allowed",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"glibc", "musl"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "musl",
			expectedSupport: true,
			description:     "musl should be allowed when both are in supported_libc",
		},
		{
			name:            "libc constraint ignored on darwin",
			supportedOS:     nil,
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "darwin",
			targetArch:      "arm64",
			targetLibc:      "",
			expectedSupport: true,
			description:     "libc constraint should be ignored on non-Linux platforms",
		},
		{
			name:            "combined OS and libc constraint",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "glibc",
			expectedSupport: true,
			description:     "Both OS and libc constraints should pass",
		},
		{
			name:            "combined OS and libc constraint - wrong OS",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "darwin",
			targetArch:      "amd64",
			targetLibc:      "",
			expectedSupport: false,
			description:     "OS constraint should fail even if libc would pass",
		},
		{
			name:            "combined OS and libc constraint - wrong libc",
			supportedOS:     []string{"linux"},
			supportedArch:   nil,
			supportedLibc:   []string{"glibc"},
			unsupportedPlat: nil,
			targetOS:        "linux",
			targetArch:      "amd64",
			targetLibc:      "musl",
			expectedSupport: false,
			description:     "Libc constraint should fail even if OS passes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:          tt.supportedOS,
					SupportedArch:        tt.supportedArch,
					SupportedLibc:        tt.supportedLibc,
					UnsupportedPlatforms: tt.unsupportedPlat,
				},
			}

			result := r.SupportsPlatformWithLibc(tt.targetOS, tt.targetArch, tt.targetLibc)
			if result != tt.expectedSupport {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedSupport, result)
			}
		})
	}
}

func TestValidatePlatformConstraintsLibc(t *testing.T) {
	tests := []struct {
		name           string
		supportedLibc  []string
		expectError    bool
		expectWarnings int
		description    string
	}{
		{
			name:           "valid single libc (glibc)",
			supportedLibc:  []string{"glibc"},
			expectError:    false,
			expectWarnings: 0,
			description:    "Single valid libc value should pass",
		},
		{
			name:           "valid single libc (musl)",
			supportedLibc:  []string{"musl"},
			expectError:    false,
			expectWarnings: 0,
			description:    "Single valid libc value should pass",
		},
		{
			name:           "valid both libc",
			supportedLibc:  []string{"glibc", "musl"},
			expectError:    false,
			expectWarnings: 0,
			description:    "Both valid libc values should pass",
		},
		{
			name:           "invalid libc value",
			supportedLibc:  []string{"uclibc"},
			expectError:    true,
			expectWarnings: 0,
			description:    "Invalid libc value should error",
		},
		{
			name:           "mixed valid and invalid libc",
			supportedLibc:  []string{"glibc", "bionic"},
			expectError:    true,
			expectWarnings: 0,
			description:    "Invalid libc value should error even if valid ones present",
		},
		{
			name:           "empty libc (valid)",
			supportedLibc:  nil,
			expectError:    false,
			expectWarnings: 0,
			description:    "Empty supported_libc should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedLibc: tt.supportedLibc,
				},
			}

			warnings, err := r.ValidatePlatformConstraints()

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error, got nil", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: expected no error, got: %v", tt.description, err)
			}
			if len(warnings) != tt.expectWarnings {
				t.Errorf("%s: expected %d warnings, got %d", tt.description, tt.expectWarnings, len(warnings))
			}
		})
	}
}

func TestFormatPlatformConstraintsWithLibc(t *testing.T) {
	tests := []struct {
		name              string
		supportedLibc     []string
		unsupportedReason string
		expectedSubstr    []string
		description       string
	}{
		{
			name:           "libc constraint shown",
			supportedLibc:  []string{"glibc"},
			expectedSubstr: []string{"Libc: glibc"},
			description:    "Should show libc constraint",
		},
		{
			name:           "multiple libc shown",
			supportedLibc:  []string{"glibc", "musl"},
			expectedSubstr: []string{"Libc: glibc, musl"},
			description:    "Should show all libc values",
		},
		{
			name:              "reason shown",
			supportedLibc:     []string{"glibc"},
			unsupportedReason: "Upstream only provides glibc binaries",
			expectedSubstr:    []string{"Libc: glibc", "Reason: Upstream only provides glibc binaries"},
			description:       "Should show reason when present",
		},
		{
			name:              "reason without libc constraint",
			supportedLibc:     nil,
			unsupportedReason: "Some other constraint",
			expectedSubstr:    []string{"Reason: Some other constraint"},
			description:       "Should show reason even without libc constraint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:       []string{"linux"}, // Need at least one constraint to show
					SupportedLibc:     tt.supportedLibc,
					UnsupportedReason: tt.unsupportedReason,
				},
			}

			result := r.FormatPlatformConstraints()
			for _, substr := range tt.expectedSubstr {
				if !contains(result, substr) {
					t.Errorf("%s: expected substring '%s' in '%s'",
						tt.description, substr, result)
				}
			}
		})
	}
}

func TestUnsupportedPlatformErrorWithLibc(t *testing.T) {
	tests := []struct {
		name               string
		currentOS          string
		currentLibc        string
		supportedLibc      []string
		unsupportedReason  string
		expectedSubstrings []string
		description        string
	}{
		{
			name:          "libc shown in error",
			currentOS:     "linux",
			currentLibc:   "musl",
			supportedLibc: []string{"glibc"},
			expectedSubstrings: []string{
				"(musl)",
				"Libc: glibc",
			},
			description: "Error should show current libc and constraint",
		},
		{
			name:              "reason shown in error",
			currentOS:         "linux",
			currentLibc:       "musl",
			supportedLibc:     []string{"glibc"},
			unsupportedReason: "Upstream only provides glibc binaries",
			expectedSubstrings: []string{
				"Libc: glibc",
				"Reason: Upstream only provides glibc binaries",
			},
			description: "Error should show reason when present",
		},
		{
			name:        "no libc on darwin",
			currentOS:   "darwin",
			currentLibc: "",
			expectedSubstrings: []string{
				"darwin/arm64",
			},
			description: "Non-Linux should not show libc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &UnsupportedPlatformError{
				RecipeName:        "test-tool",
				CurrentOS:         tt.currentOS,
				CurrentArch:       "arm64",
				CurrentLibc:       tt.currentLibc,
				SupportedLibc:     tt.supportedLibc,
				UnsupportedReason: tt.unsupportedReason,
			}

			errMsg := err.Error()

			for _, substr := range tt.expectedSubstrings {
				if !contains(errMsg, substr) {
					t.Errorf("%s: expected substring '%s' in error message:\n%s",
						tt.description, substr, errMsg)
				}
			}
		})
	}
}

func TestValidateStepsAgainstPlatforms(t *testing.T) {
	tests := []struct {
		name            string
		supportedOS     []string
		supportedArch   []string
		steps           []Step
		expectedErrors  int
		errorSubstrings []string
		description     string
	}{
		{
			name:          "valid os_mapping",
			supportedOS:   []string{"linux", "darwin"},
			supportedArch: nil,
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"os_mapping": map[string]interface{}{
							"darwin": "macos",
							"linux":  "linux",
						},
					},
				},
			},
			expectedErrors: 0,
			description:    "os_mapping with all supported OS should pass",
		},
		{
			name:          "invalid os_mapping",
			supportedOS:   []string{"darwin"},
			supportedArch: nil,
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"os_mapping": map[string]interface{}{
							"darwin": "macos",
							"linux":  "linux",
						},
					},
				},
			},
			expectedErrors:  1,
			errorSubstrings: []string{"os_mapping contains 'linux'", "not in the recipe's supported platforms"},
			description:     "os_mapping with unsupported OS should fail",
		},
		{
			name:          "valid arch_mapping",
			supportedOS:   nil,
			supportedArch: []string{"amd64", "arm64"},
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"arch_mapping": map[string]interface{}{
							"amd64": "x64",
							"arm64": "aarch64",
						},
					},
				},
			},
			expectedErrors: 0,
			description:    "arch_mapping with all supported arch should pass",
		},
		{
			name:          "invalid arch_mapping",
			supportedOS:   nil,
			supportedArch: []string{"amd64"},
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"arch_mapping": map[string]interface{}{
							"amd64": "x64",
							"arm64": "aarch64",
						},
					},
				},
			},
			expectedErrors:  1,
			errorSubstrings: []string{"arch_mapping contains 'arm64'", "not in the recipe's supported platforms"},
			description:     "arch_mapping with unsupported arch should fail",
		},
		{
			name:          "multiple errors (mapping validation)",
			supportedOS:   []string{"darwin"},
			supportedArch: []string{"amd64"},
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"os_mapping": map[string]interface{}{
							"linux": "linux",
						},
						"arch_mapping": map[string]interface{}{
							"arm64": "aarch64",
						},
					},
				},
			},
			expectedErrors:  2, // os_mapping error, arch_mapping error
			errorSubstrings: []string{"os_mapping contains 'linux'", "arch_mapping contains 'arm64'"},
			description:     "recipe with multiple errors should return all of them",
		},
		{
			name:          "partial os_mapping is valid",
			supportedOS:   []string{"linux", "darwin"},
			supportedArch: nil,
			steps: []Step{
				{
					Action: "download",
					Params: map[string]interface{}{
						"os_mapping": map[string]interface{}{
							"darwin": "macos",
						},
					},
				},
			},
			expectedErrors: 0,
			description:    "os_mapping with only some platforms should pass (unmapped use defaults)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Recipe{
				Metadata: MetadataSection{
					SupportedOS:   tt.supportedOS,
					SupportedArch: tt.supportedArch,
				},
				Steps: tt.steps,
			}

			errors := r.ValidateStepsAgainstPlatforms()

			if len(errors) != tt.expectedErrors {
				t.Errorf("%s: expected %d errors, got %d", tt.description, tt.expectedErrors, len(errors))
				for i, err := range errors {
					t.Logf("  Error %d: %v", i, err)
				}
				return
			}

			// Check that error messages contain expected substrings
			for _, substr := range tt.errorSubstrings {
				found := false
				for _, err := range errors {
					if contains(err.Error(), substr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: expected error containing '%s', but not found in errors: %v",
						tt.description, substr, errors)
				}
			}
		})
	}
}
