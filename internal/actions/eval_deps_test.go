package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetEvalDeps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action   string
		wantDeps []string
	}{
		{"npm_install", []string{"nodejs"}},
		{"go_install", []string{"go"}},
		{"download", nil},       // Core primitive, no eval deps
		{"extract", nil},        // Core primitive, no eval deps
		{"unknown_action", nil}, // Unknown action
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			deps := GetEvalDeps(tc.action)
			if len(deps) != len(tc.wantDeps) {
				t.Errorf("GetEvalDeps(%q) = %v, want %v", tc.action, deps, tc.wantDeps)
				return
			}
			for i, dep := range deps {
				if dep != tc.wantDeps[i] {
					t.Errorf("GetEvalDeps(%q)[%d] = %q, want %q", tc.action, i, dep, tc.wantDeps[i])
				}
			}
		})
	}
}

func TestCheckEvalDeps_AllMissing(t *testing.T) {
	t.Parallel()

	// Use a temp directory that doesn't have any installed tools
	toolsDir := t.TempDir()

	deps := []string{"nodejs", "go"}
	missing := checkEvalDepsInDir(deps, toolsDir)

	if len(missing) != 2 {
		t.Errorf("checkEvalDepsInDir() returned %d missing, want 2", len(missing))
	}
}

func TestCheckEvalDeps_SomeInstalled(t *testing.T) {
	t.Parallel()

	// Use a temp directory with one installed tool
	toolsDir := t.TempDir()
	nodejsDir := filepath.Join(toolsDir, "nodejs-20.0.0")
	if err := os.MkdirAll(nodejsDir, 0755); err != nil {
		t.Fatalf("failed to create nodejs dir: %v", err)
	}

	deps := []string{"nodejs", "go"}
	missing := checkEvalDepsInDir(deps, toolsDir)

	if len(missing) != 1 {
		t.Errorf("checkEvalDepsInDir() returned %d missing, want 1", len(missing))
	}
	if len(missing) > 0 && missing[0] != "go" {
		t.Errorf("checkEvalDepsInDir() missing[0] = %q, want 'go'", missing[0])
	}
}

func TestCheckEvalDeps_AllInstalled(t *testing.T) {
	t.Parallel()

	// Use a temp directory with both tools installed
	toolsDir := t.TempDir()
	nodejsDir := filepath.Join(toolsDir, "nodejs-20.0.0")
	goDir := filepath.Join(toolsDir, "go-1.21.0")
	if err := os.MkdirAll(nodejsDir, 0755); err != nil {
		t.Fatalf("failed to create nodejs dir: %v", err)
	}
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("failed to create go dir: %v", err)
	}

	deps := []string{"nodejs", "go"}
	missing := checkEvalDepsInDir(deps, toolsDir)

	if len(missing) != 0 {
		t.Errorf("checkEvalDepsInDir() returned %d missing, want 0", len(missing))
	}
}

func TestCheckEvalDeps_EmptyList(t *testing.T) {
	t.Parallel()

	missing := CheckEvalDeps(nil)
	if missing != nil {
		t.Errorf("CheckEvalDeps(nil) = %v, want nil", missing)
	}

	missing = CheckEvalDeps([]string{})
	if missing != nil {
		t.Errorf("CheckEvalDeps([]) = %v, want nil", missing)
	}
}

func TestIsToolInstalled(t *testing.T) {
	t.Parallel()

	// Create temp directory with a tool
	tmpDir := t.TempDir()
	nodejsDir := filepath.Join(tmpDir, "nodejs-20.0.0")
	if err := os.MkdirAll(nodejsDir, 0755); err != nil {
		t.Fatalf("failed to create nodejs dir: %v", err)
	}

	// Test installed tool
	if !isToolInstalled(tmpDir, "nodejs") {
		t.Error("isToolInstalled() = false, want true for installed nodejs")
	}

	// Test not installed tool
	if isToolInstalled(tmpDir, "go") {
		t.Error("isToolInstalled() = true, want false for missing go")
	}

	// Test non-existent directory
	if isToolInstalled("/nonexistent/path", "nodejs") {
		t.Error("isToolInstalled() = true, want false for non-existent directory")
	}
}
