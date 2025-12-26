package actions

import (
	"strings"
	"testing"
)

func TestValidateAction_UnknownAction(t *testing.T) {
	result := ValidateAction("nonexistent_action", nil)
	if !result.HasErrors() {
		t.Error("expected error for unknown action")
	}
	if len(result.Errors) != 1 || result.Errors[0] != "unknown action 'nonexistent_action'" {
		t.Errorf("unexpected error message: %v", result.Errors)
	}
}

func TestValidateAction_ActionWithPreflight(t *testing.T) {
	// Actions implementing Preflight validate their parameters
	// download requires 'url' parameter
	result := ValidateAction("download", nil)
	if !result.HasErrors() {
		t.Error("expected error for download without url parameter")
	}

	// With valid params (URL with variables and checksum_url), should pass
	result = ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file.tar.gz",
		"checksum_url": "https://example.com/{version}/checksums.txt",
	})
	if result.HasErrors() {
		t.Errorf("expected no errors for download with valid params, got: %v", result.Errors)
	}
}

func TestValidateAction_ActionWithoutPreflight(t *testing.T) {
	// Actions that don't implement Preflight pass validation
	// chmod is an example that doesn't require specific params in Preflight
	result := ValidateAction("chmod", nil)
	if result.HasErrors() {
		t.Errorf("expected no errors for action that passes Preflight validation, got: %v", result.Errors)
	}
}

func TestValidateAction_Warnings(t *testing.T) {
	// Test that warnings are returned separately from errors
	// Download without checksum_url should produce a warning
	result := ValidateAction("download", map[string]interface{}{
		"url": "https://example.com/{version}/file.tar.gz",
	})
	if result.HasErrors() {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
	if !result.HasWarnings() {
		t.Error("expected warning for missing checksum_url")
	}
}

func TestDownloadAction_StaticChecksumError(t *testing.T) {
	// Test that static checksum parameter triggers error
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file.tar.gz",
		"checksum":     "abc123",
		"checksum_url": "https://example.com/{version}/checksums.txt",
	})
	if !result.HasErrors() {
		t.Error("expected error for static checksum parameter")
	}
	found := false
	for _, err := range result.Errors {
		if err == "download action does not support static 'checksum'; use 'checksum_url' for dynamic verification or 'download_file' for static URLs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected specific checksum error message, got: %v", result.Errors)
	}
}

func TestDownloadAction_MissingChecksumURLWarning(t *testing.T) {
	// Test that missing checksum_url triggers warning
	result := ValidateAction("download", map[string]interface{}{
		"url": "https://example.com/{version}/file.tar.gz",
	})
	if result.HasErrors() {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
	if !result.HasWarnings() {
		t.Error("expected warning for missing checksum_url")
	}
	found := false
	for _, warn := range result.Warnings {
		if warn == "no 'checksum_url' configured; downloaded files will not be verified for integrity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected specific checksum_url warning message, got: %v", result.Warnings)
	}
}

func TestDownloadAction_StaticURLError(t *testing.T) {
	// Test that URL without variables triggers error
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/static/file.tar.gz",
		"checksum_url": "https://example.com/checksums.txt",
	})
	if !result.HasErrors() {
		t.Error("expected error for static URL without variables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "download URL contains no variables; use 'download_file' action for static URLs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected specific static URL error message, got: %v", result.Errors)
	}
}

func TestDownloadAction_URLWithVariablesPass(t *testing.T) {
	// Test that URL with variables passes validation (no error for that check)
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file-{os}-{arch}.tar.gz",
		"checksum_url": "https://example.com/{version}/checksums.txt",
	})
	if result.HasErrors() {
		t.Errorf("expected no errors for URL with variables, got: %v", result.Errors)
	}
	if result.HasWarnings() {
		t.Errorf("expected no warnings when checksum_url is provided, got: %v", result.Warnings)
	}
}

func TestPreflightResult_ToError(t *testing.T) {
	// Test ToError with no errors
	result := &PreflightResult{}
	if result.ToError() != nil {
		t.Error("expected nil error for empty result")
	}

	// Test ToError with one error
	result.AddError("single error")
	err := result.ToError()
	if err == nil {
		t.Error("expected error for result with errors")
	}
	if err.Error() != "single error" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Test ToError with multiple errors
	result.AddError("second error")
	err = result.ToError()
	if err == nil {
		t.Error("expected error for result with multiple errors")
	}
	if err.Error() != "single error (and 1 more errors)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPreflightResult_AddMethods(t *testing.T) {
	result := &PreflightResult{}

	result.AddError("error1")
	result.AddErrorf("error %d", 2)
	result.AddWarning("warning1")
	result.AddWarningf("warning %d", 2)

	if len(result.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(result.Warnings))
	}
	if !result.HasErrors() {
		t.Error("expected HasErrors to return true")
	}
	if !result.HasWarnings() {
		t.Error("expected HasWarnings to return true")
	}
}

func TestRegisteredNames(t *testing.T) {
	names := RegisteredNames()

	// Should have actions registered
	if len(names) == 0 {
		t.Error("expected registered actions")
	}

	// Should include known actions
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	expected := []string{"download", "extract", "chmod", "install_binaries"}
	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected action '%s' to be registered", exp)
		}
	}

	// Should be sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %s comes after %s", names[i], names[i-1])
		}
	}
}

// Tests for unused os_mapping/arch_mapping warnings

func TestDownloadAction_UnusedOSMapping(t *testing.T) {
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file-{arch}.tar.gz",
		"checksum_url": "https://example.com/{version}/checksums.txt",
		"os_mapping":   map[string]interface{}{"darwin": "macos"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused os_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "os_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected os_mapping warning, got: %v", result.Warnings)
	}
}

func TestDownloadAction_UnusedArchMapping(t *testing.T) {
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file-{os}.tar.gz",
		"checksum_url": "https://example.com/{version}/checksums.txt",
		"arch_mapping": map[string]interface{}{"amd64": "x64"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused arch_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "arch_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected arch_mapping warning, got: %v", result.Warnings)
	}
}

func TestDownloadAction_UsedMappingsNoWarning(t *testing.T) {
	result := ValidateAction("download", map[string]interface{}{
		"url":          "https://example.com/{version}/file-{os}-{arch}.tar.gz",
		"checksum_url": "https://example.com/{version}/checksums.txt",
		"os_mapping":   map[string]interface{}{"darwin": "macos"},
		"arch_mapping": map[string]interface{}{"amd64": "x64"},
	})
	// Should have no warnings about unused mappings
	for _, w := range result.Warnings {
		if strings.Contains(w, "mapping") && strings.Contains(w, "no effect") {
			t.Errorf("unexpected mapping warning: %s", w)
		}
	}
}

func TestDownloadArchiveAction_UnusedOSMapping(t *testing.T) {
	result := ValidateAction("download_archive", map[string]interface{}{
		"url":        "https://example.com/{version}/file-{arch}.tar.gz",
		"os_mapping": map[string]interface{}{"darwin": "macos"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused os_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "os_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected os_mapping warning, got: %v", result.Warnings)
	}
}

func TestDownloadArchiveAction_UnusedArchMapping(t *testing.T) {
	result := ValidateAction("download_archive", map[string]interface{}{
		"url":          "https://example.com/{version}/file-{os}.tar.gz",
		"arch_mapping": map[string]interface{}{"amd64": "x64"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused arch_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "arch_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected arch_mapping warning, got: %v", result.Warnings)
	}
}

func TestGitHubArchiveAction_UnusedOSMapping(t *testing.T) {
	result := ValidateAction("github_archive", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{arch}.tar.gz",
		"os_mapping":    map[string]interface{}{"darwin": "macos"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused os_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "os_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected os_mapping warning, got: %v", result.Warnings)
	}
}

func TestGitHubArchiveAction_UnusedArchMapping(t *testing.T) {
	result := ValidateAction("github_archive", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}.tar.gz",
		"arch_mapping":  map[string]interface{}{"amd64": "x64"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused arch_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "arch_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected arch_mapping warning, got: %v", result.Warnings)
	}
}

func TestGitHubArchiveAction_UsedMappingsNoWarning(t *testing.T) {
	result := ValidateAction("github_archive", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"os_mapping":    map[string]interface{}{"darwin": "macos"},
		"arch_mapping":  map[string]interface{}{"amd64": "x64"},
	})
	// Should have no warnings about unused mappings
	for _, w := range result.Warnings {
		if strings.Contains(w, "mapping") && strings.Contains(w, "no effect") {
			t.Errorf("unexpected mapping warning: %s", w)
		}
	}
}

func TestGitHubFileAction_UnusedOSMapping(t *testing.T) {
	result := ValidateAction("github_file", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{arch}",
		"os_mapping":    map[string]interface{}{"darwin": "macos"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused os_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "os_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected os_mapping warning, got: %v", result.Warnings)
	}
}

func TestGitHubFileAction_UnusedArchMapping(t *testing.T) {
	result := ValidateAction("github_file", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}",
		"arch_mapping":  map[string]interface{}{"amd64": "x64"},
	})
	if !result.HasWarnings() {
		t.Error("expected warning for unused arch_mapping")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "arch_mapping") && strings.Contains(w, "no effect") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected arch_mapping warning, got: %v", result.Warnings)
	}
}

func TestGitHubFileAction_UsedMappingsNoWarning(t *testing.T) {
	result := ValidateAction("github_file", map[string]interface{}{
		"repo":          "owner/repo",
		"asset_pattern": "tool-{version}-{os}-{arch}",
		"os_mapping":    map[string]interface{}{"darwin": "macos"},
		"arch_mapping":  map[string]interface{}{"amd64": "x64"},
	})
	// Should have no warnings about unused mappings
	for _, w := range result.Warnings {
		if strings.Contains(w, "mapping") && strings.Contains(w, "no effect") {
			t.Errorf("unexpected mapping warning: %s", w)
		}
	}
}

func TestInstallBinariesAction_EmptyBinaries(t *testing.T) {
	result := ValidateAction("install_binaries", map[string]interface{}{
		"binaries": []interface{}{},
	})
	if !result.HasErrors() {
		t.Error("expected error for empty binaries array")
	}
}

func TestGitHubArchiveAction_InvalidRepoFormat(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{"valid", "cli/cli", false},
		{"no slash", "cli", true},
		{"too many slashes", "org/repo/extra", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAction("github_archive", map[string]interface{}{
				"repo":          tt.repo,
				"asset_pattern": "test-{os}.tar.gz",
				"binaries":      []interface{}{"test"},
			})
			if tt.wantErr && !result.HasErrors() {
				t.Errorf("expected error for repo %q", tt.repo)
			}
			if !tt.wantErr && result.HasErrors() {
				t.Errorf("unexpected error for repo %q: %v", tt.repo, result.Errors)
			}
		})
	}
}

func TestGitHubFileAction_InvalidRepoFormat(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{"valid", "cli/cli", false},
		{"no slash", "cli", true},
		{"too many slashes", "org/repo/extra", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAction("github_file", map[string]interface{}{
				"repo":          tt.repo,
				"asset_pattern": "test-{os}",
				"binary_name":   "test",
			})
			if tt.wantErr && !result.HasErrors() {
				t.Errorf("expected error for repo %q", tt.repo)
			}
			if !tt.wantErr && result.HasErrors() {
				t.Errorf("unexpected error for repo %q: %v", tt.repo, result.Errors)
			}
		})
	}
}

func TestContainsPlaceholder(t *testing.T) {
	tests := []struct {
		input       string
		placeholder string
		expected    bool
	}{
		{"https://example.com/{os}/{arch}/file.tar.gz", "os", true},
		{"https://example.com/{os}/{arch}/file.tar.gz", "arch", true},
		{"https://example.com/{version}/file.tar.gz", "os", false},
		{"https://example.com/{version}/file.tar.gz", "arch", false},
		{"tool-{os}-{arch}.tar.gz", "os", true},
		{"tool-{os}-{arch}.tar.gz", "version", false},
		{"", "os", false},
		{"{os}", "os", true},
		{"os", "os", false}, // No braces
	}

	for _, tc := range tests {
		result := containsPlaceholder(tc.input, tc.placeholder)
		if result != tc.expected {
			t.Errorf("containsPlaceholder(%q, %q) = %v, want %v",
				tc.input, tc.placeholder, result, tc.expected)
		}
	}
}

func TestGitHubFileAction_ArchiveExtensionWarning(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		wantWarning bool
	}{
		{"tar.gz", "tool-{os}-{arch}.tar.gz", true},
		{"zip", "tool-{os}.zip", true},
		{"tgz", "tool.tgz", true},
		{"binary no ext", "tool-{os}-{arch}", false},
		{"exe", "tool.exe", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAction("github_file", map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": tt.pattern,
				"binary_name":   "tool",
			})
			hasArchiveWarning := false
			for _, w := range result.Warnings {
				if strings.Contains(w, "archive extension") {
					hasArchiveWarning = true
					break
				}
			}
			if tt.wantWarning && !hasArchiveWarning {
				t.Errorf("expected archive extension warning for pattern %q", tt.pattern)
			}
			if !tt.wantWarning && hasArchiveWarning {
				t.Errorf("unexpected archive extension warning for pattern %q", tt.pattern)
			}
		})
	}
}

// Tests for package manager actions requiring executables parameter

func TestNpmInstallAction_RequiresExecutables(t *testing.T) {
	result := ValidateAction("npm_install", map[string]interface{}{
		"package": "some-package",
	})
	if !result.HasErrors() {
		t.Error("expected error for missing executables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "npm_install action requires 'executables' parameter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected executables error, got: %v", result.Errors)
	}
}

func TestPipxInstallAction_RequiresExecutables(t *testing.T) {
	result := ValidateAction("pipx_install", map[string]interface{}{
		"package": "some-package",
	})
	if !result.HasErrors() {
		t.Error("expected error for missing executables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "pipx_install action requires 'executables' parameter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected executables error, got: %v", result.Errors)
	}
}

func TestCargoInstallAction_RequiresExecutables(t *testing.T) {
	result := ValidateAction("cargo_install", map[string]interface{}{
		"crate": "some-crate",
	})
	if !result.HasErrors() {
		t.Error("expected error for missing executables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "cargo_install action requires 'executables' parameter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected executables error, got: %v", result.Errors)
	}
}

func TestGoInstallAction_RequiresExecutables(t *testing.T) {
	result := ValidateAction("go_install", map[string]interface{}{
		"module": "github.com/some/module",
	})
	if !result.HasErrors() {
		t.Error("expected error for missing executables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "go_install action requires 'executables' parameter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected executables error, got: %v", result.Errors)
	}
}

func TestGemInstallAction_RequiresExecutables(t *testing.T) {
	result := ValidateAction("gem_install", map[string]interface{}{
		"gem": "some-gem",
	})
	if !result.HasErrors() {
		t.Error("expected error for missing executables")
	}
	found := false
	for _, err := range result.Errors {
		if err == "gem_install action requires 'executables' parameter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected executables error, got: %v", result.Errors)
	}
}

func TestRequireSystemAction_MissingInstallGuide(t *testing.T) {
	result := ValidateAction("require_system", map[string]interface{}{
		"command": "gcc",
	})
	if !result.HasWarnings() {
		t.Error("expected warning for missing install_guide")
	}
}

func TestRequireSystemAction_WithInstallGuide(t *testing.T) {
	result := ValidateAction("require_system", map[string]interface{}{
		"command": "gcc",
		"install_guide": map[string]interface{}{
			"darwin": "brew install gcc",
			"linux":  "apt install gcc",
		},
	})
	// Should not have install_guide warning
	for _, w := range result.Warnings {
		if strings.Contains(w, "install_guide") {
			t.Errorf("unexpected install_guide warning: %s", w)
		}
	}
}

func TestRequireSystemAction_MinVersionWithoutDetection(t *testing.T) {
	// min_version without version_flag
	result := ValidateAction("require_system", map[string]interface{}{
		"command":       "gcc",
		"min_version":   "10.0",
		"install_guide": map[string]interface{}{"linux": "apt install gcc"},
	})
	if !result.HasErrors() {
		t.Error("expected error for min_version without version detection")
	}

	// min_version with only version_flag (missing regex)
	result = ValidateAction("require_system", map[string]interface{}{
		"command":       "gcc",
		"min_version":   "10.0",
		"version_flag":  "--version",
		"install_guide": map[string]interface{}{"linux": "apt install gcc"},
	})
	if !result.HasErrors() {
		t.Error("expected error for min_version without version_regex")
	}
}

func TestRequireSystemAction_CompleteVersionDetection(t *testing.T) {
	result := ValidateAction("require_system", map[string]interface{}{
		"command":       "gcc",
		"min_version":   "10.0",
		"version_flag":  "--version",
		"version_regex": `gcc \(.*\) (\d+\.\d+)`,
		"install_guide": map[string]interface{}{"linux": "apt install gcc"},
	})
	// Should not have version detection error
	for _, err := range result.Errors {
		if strings.Contains(err, "version detection") {
			t.Errorf("unexpected version detection error: %s", err)
		}
	}
}

func TestRunCommandAction_HardcodedPathsWarning(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantWarning bool
	}{
		{"tsuku home tilde", "cp ~/.tsuku/bin/tool /dest", true},
		{"tsuku home env", "ls $HOME/.tsuku/tools/", true},
		{"tsuku tools path", "chmod +x .tsuku/tools/foo/bin/bar", true},
		{"system path", "/usr/bin/make install", false},
		{"tmp path", "cp /tmp/file dest", false},
		{"variable used", "cp {install_dir}/bin/tool /dest", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAction("run_command", map[string]interface{}{
				"command": tt.command,
			})
			hasPathWarning := false
			for _, w := range result.Warnings {
				if strings.Contains(w, "hardcoded") {
					hasPathWarning = true
					break
				}
			}
			if tt.wantWarning && !hasPathWarning {
				t.Errorf("expected hardcoded path warning for command %q", tt.command)
			}
			if !tt.wantWarning && hasPathWarning {
				t.Errorf("unexpected hardcoded path warning for command %q", tt.command)
			}
		})
	}
}
