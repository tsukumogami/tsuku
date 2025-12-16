package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/executor"
)

func TestLoadPlanFromSource_File(t *testing.T) {
	// Create a temp file with valid plan JSON
	plan := executor.InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform: executor.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Steps: []executor.ResolvedStep{
			{
				Action:    "download",
				Params:    map[string]interface{}{"url": "https://example.com/file.tar.gz"},
				Checksum:  "abc123",
				Evaluable: true,
			},
		},
	}

	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.json")
	f, err := os.Create(planPath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer f.Close()

	// Write plan JSON
	planJSON := `{
		"format_version": 2,
		"tool": "test-tool",
		"version": "1.0.0",
		"platform": {"os": "` + plan.Platform.OS + `", "arch": "` + plan.Platform.Arch + `"},
		"steps": [{"action": "download", "params": {"url": "https://example.com/file.tar.gz"}, "checksum": "abc123", "evaluable": true}]
	}`
	if _, err := f.WriteString(planJSON); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}
	f.Close()

	// Test loading from file
	loaded, err := loadPlanFromSource(planPath)
	if err != nil {
		t.Fatalf("loadPlanFromSource() error = %v", err)
	}

	if loaded.Tool != "test-tool" {
		t.Errorf("Tool = %q, want %q", loaded.Tool, "test-tool")
	}
	if loaded.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", loaded.Version, "1.0.0")
	}
	if loaded.FormatVersion != 2 {
		t.Errorf("FormatVersion = %d, want %d", loaded.FormatVersion, 2)
	}
}

func TestLoadPlanFromSource_FileNotFound(t *testing.T) {
	_, err := loadPlanFromSource("/nonexistent/path/plan.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to open plan file") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "failed to open plan file")
	}
}

func TestLoadPlanFromSource_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "invalid.json")
	if err := os.WriteFile(planPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := loadPlanFromSource(planPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse plan from") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "failed to parse plan from")
	}
	// Should NOT contain the stdin hint
	if strings.Contains(err.Error(), "Hint:") {
		t.Error("file parse error should not contain stdin hint")
	}
}

func TestLoadPlanFromSource_Stdin(t *testing.T) {
	planJSON := `{
		"format_version": 2,
		"tool": "stdin-tool",
		"version": "2.0.0",
		"platform": {"os": "` + runtime.GOOS + `", "arch": "` + runtime.GOARCH + `"},
		"steps": [{"action": "chmod", "params": {"mode": "755"}, "evaluable": true}]
	}`

	// Use the internal function to inject a mock stdin
	reader := strings.NewReader(planJSON)
	loaded, err := loadPlanFromSourceWithReader("-", reader)
	if err != nil {
		t.Fatalf("loadPlanFromSourceWithReader() error = %v", err)
	}

	if loaded.Tool != "stdin-tool" {
		t.Errorf("Tool = %q, want %q", loaded.Tool, "stdin-tool")
	}
	if loaded.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", loaded.Version, "2.0.0")
	}
}

func TestLoadPlanFromSource_StdinInvalidJSON(t *testing.T) {
	reader := strings.NewReader("not valid json")
	_, err := loadPlanFromSourceWithReader("-", reader)
	if err == nil {
		t.Fatal("expected error for invalid JSON from stdin")
	}
	if !strings.Contains(err.Error(), "failed to parse plan from stdin") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "failed to parse plan from stdin")
	}
	if !strings.Contains(err.Error(), "Hint:") {
		t.Error("stdin parse error should contain debugging hint")
	}
}

func TestValidateExternalPlan_Valid(t *testing.T) {
	plan := &executor.InstallationPlan{
		FormatVersion: 2,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform: executor.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Steps: []executor.ResolvedStep{
			{
				Action:   "download",
				Params:   map[string]interface{}{"url": "https://example.com/file.tar.gz"},
				Checksum: "abc123",
			},
			{
				Action: "extract",
				Params: map[string]interface{}{"source": "file.tar.gz"},
			},
		},
	}

	// No tool name specified - should pass
	if err := validateExternalPlan(plan, ""); err != nil {
		t.Errorf("validateExternalPlan() with empty toolName error = %v", err)
	}

	// Matching tool name - should pass
	if err := validateExternalPlan(plan, "test-tool"); err != nil {
		t.Errorf("validateExternalPlan() with matching toolName error = %v", err)
	}
}

func TestValidateExternalPlan_ToolNameMismatch(t *testing.T) {
	plan := &executor.InstallationPlan{
		FormatVersion: 2,
		Tool:          "actual-tool",
		Version:       "1.0.0",
		Platform: executor.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Steps: []executor.ResolvedStep{
			{Action: "chmod", Params: map[string]interface{}{"mode": "755"}},
		},
	}

	err := validateExternalPlan(plan, "different-tool")
	if err == nil {
		t.Fatal("expected error for tool name mismatch")
	}
	if !strings.Contains(err.Error(), "actual-tool") {
		t.Errorf("error = %q, want to contain plan's tool %q", err.Error(), "actual-tool")
	}
	if !strings.Contains(err.Error(), "different-tool") {
		t.Errorf("error = %q, want to contain specified tool %q", err.Error(), "different-tool")
	}
}

// Note: Structural validation tests (format version, missing checksum, platform mismatch)
// have been removed. validateExternalPlan now only handles external-plan-specific checks
// (tool name). Structural validation is handled by executor.ExecutePlan via executor.ValidatePlan.
