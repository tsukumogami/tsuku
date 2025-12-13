package executor

import (
	"encoding/json"
	"testing"
	"time"
)

func TestInstallationPlanJSONRoundTrip(t *testing.T) {
	// Create a plan with all fields populated
	original := InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "ripgrep",
		Version:       "14.1.0",
		Platform: Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		GeneratedAt:  time.Date(2025, 12, 10, 15, 30, 0, 0, time.UTC),
		RecipeHash:   "abc123def456",
		RecipeSource: "registry",
		Steps: []ResolvedStep{
			{
				Action:    "download_archive",
				Params:    map[string]interface{}{"strip_dirs": float64(1)},
				Evaluable: true,
				URL:       "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz",
				Checksum:  "sha256:abcdef123456",
				Size:      1234567,
			},
			{
				Action:    "install_binaries",
				Params:    map[string]interface{}{"files": []interface{}{"rg"}},
				Evaluable: true,
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back
	var roundtrip InstallationPlan
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify all fields
	if roundtrip.FormatVersion != original.FormatVersion {
		t.Errorf("FormatVersion: got %d, want %d", roundtrip.FormatVersion, original.FormatVersion)
	}
	if roundtrip.Tool != original.Tool {
		t.Errorf("Tool: got %q, want %q", roundtrip.Tool, original.Tool)
	}
	if roundtrip.Version != original.Version {
		t.Errorf("Version: got %q, want %q", roundtrip.Version, original.Version)
	}
	if roundtrip.Platform.OS != original.Platform.OS {
		t.Errorf("Platform.OS: got %q, want %q", roundtrip.Platform.OS, original.Platform.OS)
	}
	if roundtrip.Platform.Arch != original.Platform.Arch {
		t.Errorf("Platform.Arch: got %q, want %q", roundtrip.Platform.Arch, original.Platform.Arch)
	}
	if !roundtrip.GeneratedAt.Equal(original.GeneratedAt) {
		t.Errorf("GeneratedAt: got %v, want %v", roundtrip.GeneratedAt, original.GeneratedAt)
	}
	if roundtrip.RecipeHash != original.RecipeHash {
		t.Errorf("RecipeHash: got %q, want %q", roundtrip.RecipeHash, original.RecipeHash)
	}
	if roundtrip.RecipeSource != original.RecipeSource {
		t.Errorf("RecipeSource: got %q, want %q", roundtrip.RecipeSource, original.RecipeSource)
	}
	if len(roundtrip.Steps) != len(original.Steps) {
		t.Fatalf("Steps length: got %d, want %d", len(roundtrip.Steps), len(original.Steps))
	}
}

func TestResolvedStepJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		step ResolvedStep
	}{
		{
			name: "download step with all fields",
			step: ResolvedStep{
				Action:    "download",
				Params:    map[string]interface{}{"url": "https://example.com/file.tar.gz"},
				Evaluable: true,
				URL:       "https://example.com/file.tar.gz",
				Checksum:  "sha256:abc123",
				Size:      12345,
			},
		},
		{
			name: "non-download step without optional fields",
			step: ResolvedStep{
				Action:    "install_binaries",
				Params:    map[string]interface{}{"files": []interface{}{"foo", "bar"}},
				Evaluable: true,
			},
		},
		{
			name: "non-evaluable step",
			step: ResolvedStep{
				Action:    "run_command",
				Params:    map[string]interface{}{"command": "make build"},
				Evaluable: false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.step)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var roundtrip ResolvedStep
			if err := json.Unmarshal(data, &roundtrip); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if roundtrip.Action != tc.step.Action {
				t.Errorf("Action: got %q, want %q", roundtrip.Action, tc.step.Action)
			}
			if roundtrip.Evaluable != tc.step.Evaluable {
				t.Errorf("Evaluable: got %v, want %v", roundtrip.Evaluable, tc.step.Evaluable)
			}
			if roundtrip.URL != tc.step.URL {
				t.Errorf("URL: got %q, want %q", roundtrip.URL, tc.step.URL)
			}
			if roundtrip.Checksum != tc.step.Checksum {
				t.Errorf("Checksum: got %q, want %q", roundtrip.Checksum, tc.step.Checksum)
			}
			if roundtrip.Size != tc.step.Size {
				t.Errorf("Size: got %d, want %d", roundtrip.Size, tc.step.Size)
			}
		})
	}
}

func TestPlatformJSONRoundTrip(t *testing.T) {
	original := Platform{
		OS:   "darwin",
		Arch: "arm64",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var roundtrip Platform
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if roundtrip.OS != original.OS {
		t.Errorf("OS: got %q, want %q", roundtrip.OS, original.OS)
	}
	if roundtrip.Arch != original.Arch {
		t.Errorf("Arch: got %q, want %q", roundtrip.Arch, original.Arch)
	}
}

func TestJSONFieldNames(t *testing.T) {
	// Verify JSON field names match the design spec (snake_case)
	plan := InstallationPlan{
		FormatVersion: 1,
		Tool:          "test",
		Version:       "1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		GeneratedAt:   time.Now(),
		RecipeHash:    "hash",
		RecipeSource:  "source",
		Steps: []ResolvedStep{
			{Action: "download", Params: map[string]interface{}{}, Evaluable: true, URL: "url", Checksum: "sum", Size: 100},
		},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse as generic map to check field names
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Check top-level field names
	expectedFields := []string{
		"format_version",
		"tool",
		"version",
		"platform",
		"generated_at",
		"recipe_hash",
		"recipe_source",
		"steps",
	}

	for _, field := range expectedFields {
		if _, ok := m[field]; !ok {
			t.Errorf("missing expected JSON field: %s", field)
		}
	}

	// Check Platform field names
	platform := m["platform"].(map[string]interface{})
	if _, ok := platform["os"]; !ok {
		t.Error("missing platform.os field")
	}
	if _, ok := platform["arch"]; !ok {
		t.Error("missing platform.arch field")
	}

	// Check Step field names
	steps := m["steps"].([]interface{})
	step := steps[0].(map[string]interface{})
	stepExpectedFields := []string{"action", "params", "evaluable", "url", "checksum", "size"}
	for _, field := range stepExpectedFields {
		if _, ok := step[field]; !ok {
			t.Errorf("missing expected step JSON field: %s", field)
		}
	}
}

func TestOmitEmptyFields(t *testing.T) {
	// Verify that optional fields with zero values are omitted
	step := ResolvedStep{
		Action:    "install_binaries",
		Params:    map[string]interface{}{},
		Evaluable: true,
		// URL, Checksum, Size intentionally left empty
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// These fields should be omitted when empty
	if _, ok := m["url"]; ok {
		t.Error("url field should be omitted when empty")
	}
	if _, ok := m["checksum"]; ok {
		t.Error("checksum field should be omitted when empty")
	}
	if _, ok := m["size"]; ok {
		t.Error("size field should be omitted when empty")
	}
}

func TestIsActionEvaluable(t *testing.T) {
	tests := []struct {
		action   string
		expected bool
	}{
		// Primitive actions - evaluable
		{"download", true},
		{"extract", true},
		{"install_binaries", true},
		{"chmod", true},
		{"set_env", true},
		{"validate_checksum", true},
		{"set_rpath", true},
		{"link_dependencies", true},
		{"install_libraries", true},
		{"npm_exec", true},

		// Composite actions - not in evaluability map (decomposed at plan time)
		// These return false because they're not in the map (unknown action behavior)
		{"download_archive", false},
		{"github_archive", false},
		{"github_file", false},
		{"hashicorp_release", false},
		{"homebrew_bottle", false},

		// Non-evaluable actions
		{"run_command", false},
		{"npm_install", false},
		{"pipx_install", false},
		{"cargo_install", false},
		{"gem_install", false},
		{"cpan_install", false},
		{"go_install", false},
		{"nix_install", false},
		{"apt_install", false},
		{"yum_install", false},
		{"brew_install", false},

		// Unknown action - should be non-evaluable for safety
		{"unknown_action", false},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			got := IsActionEvaluable(tc.action)
			if got != tc.expected {
				t.Errorf("IsActionEvaluable(%q): got %v, want %v", tc.action, got, tc.expected)
			}
		})
	}
}

func TestFormatVersionConstant(t *testing.T) {
	// Version 2 introduced composite action decomposition (issue #440)
	if PlanFormatVersion != 2 {
		t.Errorf("PlanFormatVersion: got %d, want 2", PlanFormatVersion)
	}
}

// Tests for plan validation (issue #441)

func TestValidatePlan_AllPrimitives(t *testing.T) {
	// Valid plan with only primitive actions should pass
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "download",
				Params:    map[string]interface{}{"url": "https://example.com/file.tar.gz"},
				Evaluable: true,
				URL:       "https://example.com/file.tar.gz",
				Checksum:  "sha256:abc123",
				Size:      1234,
			},
			{
				Action:    "extract",
				Params:    map[string]interface{}{"format": "tar.gz"},
				Evaluable: true,
			},
			{
				Action:    "chmod",
				Params:    map[string]interface{}{"files": []interface{}{"binary"}},
				Evaluable: true,
			},
			{
				Action:    "install_binaries",
				Params:    map[string]interface{}{"binaries": []interface{}{"binary"}},
				Evaluable: true,
			},
		},
	}

	err := ValidatePlan(plan)
	if err != nil {
		t.Errorf("ValidatePlan() returned error for valid plan: %v", err)
	}
}

func TestValidatePlan_CompositeAction(t *testing.T) {
	// Plan with composite action should fail
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "github_archive",
				Params:    map[string]interface{}{"repo": "owner/repo"},
				Evaluable: true,
			},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Error("ValidatePlan() should return error for plan with composite action")
	}

	// Check error message contains useful information
	errMsg := err.Error()
	if !contains(errMsg, "github_archive") {
		t.Errorf("error message should mention the action name, got: %s", errMsg)
	}
	if !contains(errMsg, "decomposed") {
		t.Errorf("error message should mention decomposition, got: %s", errMsg)
	}
}

func TestValidatePlan_UnknownAction(t *testing.T) {
	// Plan with unknown action should fail
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "nonexistent_action",
				Params:    map[string]interface{}{},
				Evaluable: true,
			},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Error("ValidatePlan() should return error for plan with unknown action")
	}

	errMsg := err.Error()
	if !contains(errMsg, "unknown") {
		t.Errorf("error message should mention 'unknown', got: %s", errMsg)
	}
	if !contains(errMsg, "nonexistent_action") {
		t.Errorf("error message should mention the action name, got: %s", errMsg)
	}
}

func TestValidatePlan_MissingChecksum(t *testing.T) {
	// Download action without checksum should fail (security requirement)
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "download",
				Params:    map[string]interface{}{"url": "https://example.com/file.tar.gz"},
				Evaluable: true,
				URL:       "https://example.com/file.tar.gz",
				// Missing Checksum field - should fail
			},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Error("ValidatePlan() should return error for download without checksum")
	}

	errMsg := err.Error()
	if !contains(errMsg, "checksum") {
		t.Errorf("error message should mention 'checksum', got: %s", errMsg)
	}
	if !contains(errMsg, "security") {
		t.Errorf("error message should mention 'security', got: %s", errMsg)
	}
}

func TestValidatePlan_EmptyPlan(t *testing.T) {
	// Empty plan (no steps) should pass validation
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps:         []ResolvedStep{},
	}

	err := ValidatePlan(plan)
	if err != nil {
		t.Errorf("ValidatePlan() should pass for empty plan, got: %v", err)
	}
}

func TestValidatePlan_OldFormatVersion(t *testing.T) {
	// Plan with old format version should fail
	plan := &InstallationPlan{
		FormatVersion: 1, // Old format that may contain composites
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps:         []ResolvedStep{},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Error("ValidatePlan() should return error for old format version")
	}

	errMsg := err.Error()
	if !contains(errMsg, "version") {
		t.Errorf("error message should mention 'version', got: %s", errMsg)
	}
}

func TestValidatePlan_MultipleErrors(t *testing.T) {
	// Plan with multiple issues should report all errors
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "github_archive", // Composite action
				Params:    map[string]interface{}{},
				Evaluable: true,
			},
			{
				Action:    "download",
				Params:    map[string]interface{}{},
				Evaluable: true,
				// Missing checksum
			},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("ValidatePlan() should return error for plan with multiple issues")
	}

	// Check that we get multiple errors
	pve, ok := err.(*PlanValidationError)
	if !ok {
		t.Fatalf("expected PlanValidationError, got %T", err)
	}

	if len(pve.Errors) < 2 {
		t.Errorf("expected at least 2 validation errors, got %d", len(pve.Errors))
	}
}

func TestValidatePlan_EcosystemPrimitives(t *testing.T) {
	// Ecosystem primitives (go_build, cargo_build, npm_exec) should pass
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "go_build",
				Params:    map[string]interface{}{"module": "github.com/example/tool"},
				Evaluable: true,
			},
			{
				Action:    "cargo_build",
				Params:    map[string]interface{}{"crate": "example"},
				Evaluable: true,
			},
			{
				Action:    "npm_exec",
				Params:    map[string]interface{}{"package": "@example/cli"},
				Evaluable: true,
			},
		},
	}

	err := ValidatePlan(plan)
	if err != nil {
		t.Errorf("ValidatePlan() should pass for ecosystem primitives, got: %v", err)
	}
}

func TestValidatePlan_NonDecomposableAction(t *testing.T) {
	// Actions that are registered but not primitive and not decomposable should fail
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "run_command", // Known but not primitive and not decomposable
				Params:    map[string]interface{}{"command": "echo test"},
				Evaluable: false,
			},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Error("ValidatePlan() should return error for non-primitive non-decomposable action")
	}
}

func TestValidatePlanStrict(t *testing.T) {
	// Test the strict variant that returns slice of errors
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "github_archive",
				Params:    map[string]interface{}{},
				Evaluable: true,
			},
		},
	}

	errors := ValidatePlanStrict(plan)
	if len(errors) == 0 {
		t.Error("ValidatePlanStrict() should return errors for invalid plan")
	}

	// Check the error details
	if errors[0].Step != 0 {
		t.Errorf("expected step 0, got %d", errors[0].Step)
	}
	if errors[0].Action != "github_archive" {
		t.Errorf("expected action 'github_archive', got %q", errors[0].Action)
	}
}

func TestValidatePlanStrict_ValidPlan(t *testing.T) {
	// Valid plan should return nil
	plan := &InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action:    "extract",
				Params:    map[string]interface{}{},
				Evaluable: true,
			},
		},
	}

	errors := ValidatePlanStrict(plan)
	if errors != nil {
		t.Errorf("ValidatePlanStrict() should return nil for valid plan, got %v", errors)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Step:    2,
		Action:  "github_archive",
		Message: "test message",
	}

	errStr := err.Error()
	if !contains(errStr, "step 2") {
		t.Errorf("error string should contain step number, got: %s", errStr)
	}
	if !contains(errStr, "github_archive") {
		t.Errorf("error string should contain action name, got: %s", errStr)
	}
	if !contains(errStr, "test message") {
		t.Errorf("error string should contain message, got: %s", errStr)
	}
}

func TestPlanValidationError_Error(t *testing.T) {
	// Test empty errors
	empty := &PlanValidationError{Errors: []ValidationError{}}
	if empty.Error() != "plan validation failed" {
		t.Errorf("empty error message unexpected: %s", empty.Error())
	}

	// Test single error
	single := &PlanValidationError{
		Errors: []ValidationError{
			{Step: 0, Action: "test", Message: "single error"},
		},
	}
	singleStr := single.Error()
	if !contains(singleStr, "invalid plan") {
		t.Errorf("single error should mention 'invalid plan', got: %s", singleStr)
	}

	// Test multiple errors
	multiple := &PlanValidationError{
		Errors: []ValidationError{
			{Step: 0, Action: "test1", Message: "first error"},
			{Step: 1, Action: "test2", Message: "second error"},
		},
	}
	multipleStr := multiple.Error()
	if !contains(multipleStr, "2 errors") {
		t.Errorf("multiple error should mention count, got: %s", multipleStr)
	}
}

// contains is a helper for string containment check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
