package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/executor"
)

func TestRunPlanBasedInstall_ValidPlan(t *testing.T) {
	// Create a temp directory for the plan file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.json")

	// Create a minimal valid plan
	planJSON := `{
		"format_version": 2,
		"tool": "test-tool",
		"version": "1.0.0",
		"platform": {"os": "` + runtime.GOOS + `", "arch": "` + runtime.GOARCH + `"},
		"steps": [
			{"action": "chmod", "params": {"path": ".install/bin/test-tool", "mode": "755"}, "evaluable": true}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planJSON), 0644); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	// Verify the plan can be loaded and validated
	plan, err := loadPlanFromSource(planPath)
	if err != nil {
		t.Fatalf("loadPlanFromSource() error = %v", err)
	}

	if plan.Tool != "test-tool" {
		t.Errorf("Tool = %q, want %q", plan.Tool, "test-tool")
	}
	if plan.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", plan.Version, "1.0.0")
	}
}

func TestRunPlanBasedInstall_ToolNameFromPlan(t *testing.T) {
	// Test that tool name defaults from plan when not specified
	plan := &executor.InstallationPlan{
		FormatVersion: 2,
		Tool:          "plan-tool",
		Version:       "2.0.0",
		Platform: executor.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Steps: []executor.ResolvedStep{
			{Action: "chmod", Params: map[string]interface{}{"mode": "755"}, Evaluable: true},
		},
	}

	// Validate with empty tool name - should pass (defaults to plan's tool)
	if err := validateExternalPlan(plan, ""); err != nil {
		t.Errorf("validateExternalPlan() with empty toolName error = %v", err)
	}
}

func TestRunPlanBasedInstall_ToolNameMismatch(t *testing.T) {
	// Test that mismatched tool name produces error
	plan := &executor.InstallationPlan{
		FormatVersion: 2,
		Tool:          "actual-tool",
		Version:       "1.0.0",
		Platform: executor.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Steps: []executor.ResolvedStep{
			{Action: "chmod", Params: map[string]interface{}{"mode": "755"}, Evaluable: true},
		},
	}

	err := validateExternalPlan(plan, "different-tool")
	if err == nil {
		t.Fatal("expected error for tool name mismatch")
	}
}

func TestInstallPlanFlag(t *testing.T) {
	// Verify --plan flag is registered
	flag := installCmd.Flags().Lookup("plan")
	if flag == nil {
		t.Fatal("--plan flag not registered")
	}
	if flag.Usage != "Install from a pre-computed plan file (use '-' for stdin)" {
		t.Errorf("--plan usage = %q, want %q", flag.Usage, "Install from a pre-computed plan file (use '-' for stdin)")
	}
}
