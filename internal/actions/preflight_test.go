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
