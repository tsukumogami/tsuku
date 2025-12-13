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
