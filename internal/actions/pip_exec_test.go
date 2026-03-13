package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestPipExecAction_Name(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	if action.Name() != "pip_exec" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pip_exec")
	}
}

func TestPipExecAction_Registration(t *testing.T) {
	t.Parallel()
	action := Get("pip_exec")
	if action == nil {
		t.Fatal("pip_exec action not registered")
	}
	if action.Name() != "pip_exec" {
		t.Errorf("registered action Name() = %q, want %q", action.Name(), "pip_exec")
	}
}

func TestPipExecAction_IsPrimitive(t *testing.T) {
	t.Parallel()
	if !IsPrimitive("pip_exec") {
		t.Error("pip_exec should be registered as a primitive")
	}
}

func TestPipExecAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	if action.IsDeterministic() {
		t.Error("pip_exec should not be deterministic (has residual non-determinism)")
	}
}

func TestPipExecAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	deps := action.Dependencies()

	// Should require python-standalone at install time
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "python-standalone" {
		t.Errorf("InstallTime = %v, want [python-standalone]", deps.InstallTime)
	}

	// Should require python-standalone at runtime
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "python-standalone" {
		t.Errorf("Runtime = %v, want [python-standalone]", deps.Runtime)
	}
}

func TestPipExecAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}

	// pip_exec needs network to download packages from PyPI
	nv, ok := interface{}(action).(NetworkValidator)
	if !ok {
		t.Fatal("pip_exec should implement NetworkValidator")
	}
	if !nv.RequiresNetwork() {
		t.Error("pip_exec should require network")
	}
}

func TestPipExecAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	tests := []struct {
		name   string
		params map[string]interface{}
		errMsg string
	}{
		{
			name:   "missing package",
			params: map[string]interface{}{},
			errMsg: "pip_exec action requires 'package' parameter",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"package": "ruff",
			},
			errMsg: "pip_exec action requires 'executables' parameter with at least one executable",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"package":     "ruff",
				"executables": []string{},
			},
			errMsg: "pip_exec action requires 'executables' parameter with at least one executable",
		},
		{
			name: "missing locked_requirements",
			params: map[string]interface{}{
				"package":     "ruff",
				"executables": []string{"ruff"},
			},
			errMsg: "pip_exec action requires 'locked_requirements' parameter",
		},
		{
			name: "empty locked_requirements",
			params: map[string]interface{}{
				"package":             "ruff",
				"executables":         []string{"ruff"},
				"locked_requirements": "",
			},
			errMsg: "pip_exec action requires 'locked_requirements' parameter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := action.Execute(ctx, tc.params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tc.errMsg {
				t.Errorf("error = %q, want %q", err.Error(), tc.errMsg)
			}
		})
	}
}

func TestCountRequirementsPackages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		requirements string
		want         int
	}{
		{
			name:         "empty",
			requirements: "",
			want:         0,
		},
		{
			name:         "single package",
			requirements: "ruff==0.1.0 --hash=sha256:abc123",
			want:         1,
		},
		{
			name: "multiple packages",
			requirements: `ruff==0.1.0 --hash=sha256:abc123
click==8.1.0 --hash=sha256:def456
typing-extensions==4.8.0 --hash=sha256:ghi789`,
			want: 3,
		},
		{
			name: "with comments",
			requirements: `# This is a comment
ruff==0.1.0 --hash=sha256:abc123
# Another comment
click==8.1.0 --hash=sha256:def456`,
			want: 2,
		},
		{
			name: "with empty lines",
			requirements: `ruff==0.1.0 --hash=sha256:abc123

click==8.1.0 --hash=sha256:def456

`,
			want: 2,
		},
		{
			name: "with pip options",
			requirements: `--no-binary :all:
ruff==0.1.0 --hash=sha256:abc123`,
			want: 1,
		},
		{
			name:         "url-based package",
			requirements: `ruff @ https://files.pythonhosted.org/packages/ruff-0.1.0.whl --hash=sha256:abc123`,
			want:         1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := countRequirementsPackages(tc.requirements)
			if got != tc.want {
				t.Errorf("countRequirementsPackages() = %d, want %d", got, tc.want)
			}
		})
	}
}

// -- pip_exec.go: PipExecAction.Execute with valid params up to pip resolution --

// -- pip_exec.go: Execute with all required params (exercises optional param paths) --

func TestPipExecAction_Execute_AllRequiredParams(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Provide all required params to exercise code past locked_requirements check
	err := action.Execute(ctx, map[string]any{
		"package":             "flask",
		"version":             "3.0.0",
		"executables":         []any{"flask"},
		"locked_requirements": "flask==3.0.0",
		"has_native_addons":   true,
	})
	// Should fail at python resolution, not param validation
	if err == nil {
		t.Error("Expected error (python not found)")
	}
	if strings.Contains(err.Error(), "requires") {
		t.Errorf("Expected python resolution error, got param error: %v", err)
	}
}

// -- pip_exec.go: fixPythonShebang --

func TestFixPythonShebang_AbsolutePythonPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.py")
	content := "#!/usr/bin/python3\nimport sys\nprint('hello')\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	if err := fixPythonShebang(scriptPath); err != nil {
		t.Fatalf("fixPythonShebang() error = %v", err)
	}

	result, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result[:2]) != "#!" {
		t.Error("result should start with shebang")
	}
}

func TestFixPythonShebang_NotAScript(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "binary")
	if err := os.WriteFile(scriptPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err == nil {
		t.Error("fixPythonShebang() expected error for non-script file")
	}
}

func TestFixPythonShebang_NotPython(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.sh")
	content := "#!/bin/bash\necho hello\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err == nil {
		t.Error("fixPythonShebang() expected error for non-Python script")
	}
}

func TestFixPythonShebang_AlreadyRelative(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "script.py")
	content := "#!/usr/bin/env ./python3\nimport sys\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	if err := fixPythonShebang(scriptPath); err != nil {
		t.Errorf("fixPythonShebang() error = %v for already relative path", err)
	}
}

// -- pip_exec.go: countRequirementsPackages --

func TestCountRequirementsPackages_Continuation(t *testing.T) {
	t.Parallel()
	// Test continuation lines (backslash) are skipped
	input := "requests==2.31.0 \\\n  --hash=sha256:abc\n"
	got := countRequirementsPackages(input)
	if got != 1 {
		t.Errorf("countRequirementsPackages() = %d, want 1", got)
	}
}

// -- pip_exec.go: Dependencies --

func TestPipExecAction_Dependencies_Direct(t *testing.T) {
	t.Parallel()
	action := PipExecAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "python-standalone" {
		t.Errorf("Dependencies().InstallTime = %v, want [python-standalone]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "python-standalone" {
		t.Errorf("Dependencies().Runtime = %v, want [python-standalone]", deps.Runtime)
	}
}

// -- pip_exec.go: Execute param validation --

func TestPipExecAction_Execute_MissingPackage(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing package param")
	}
}

func TestPipExecAction_Execute_MissingVersion(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
	})
	if err == nil {
		t.Error("Expected error for missing version param")
	}
}

func TestPipExecAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package": "flask",
		"version": "3.0.0",
	})
	if err == nil {
		t.Error("Expected error for missing executables param")
	}
}

func TestPipExecAction_Execute_MissingLockedRequirements(t *testing.T) {
	t.Parallel()
	action := &PipExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"package":     "flask",
		"version":     "3.0.0",
		"executables": []any{"flask"},
	})
	if err == nil {
		t.Error("Expected error for missing locked_requirements")
	}
}
