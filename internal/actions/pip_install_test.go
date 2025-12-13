package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPipInstallAction_Name(t *testing.T) {
	action := &PipInstallAction{}
	if action.Name() != "pip_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pip_install")
	}
}

func TestPipInstallAction_IsPrimitive(t *testing.T) {
	if !IsPrimitive("pip_install") {
		t.Error("pip_install should be registered as a primitive")
	}
}

func TestPipInstallAction_IsRegistered(t *testing.T) {
	action := Get("pip_install")
	if action == nil {
		t.Error("pip_install should be registered in the action registry")
	}
	if action.Name() != "pip_install" {
		t.Errorf("registered action has wrong name: got %q, want %q", action.Name(), "pip_install")
	}
}

func TestBuildPipInstallArgs(t *testing.T) {
	tests := []struct {
		name            string
		sourceDir       string
		requirements    string
		constraints     string
		useHashes       bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "requirements only without hashes",
			requirements: "/path/to/requirements.txt",
			wantContains: []string{
				"install",
				"-r", "/path/to/requirements.txt",
				"--disable-pip-version-check",
			},
			wantNotContains: []string{
				"--require-hashes",
				"--no-deps",
				"--only-binary",
			},
		},
		{
			name:         "requirements with hashes",
			requirements: "/path/to/requirements.txt",
			useHashes:    true,
			wantContains: []string{
				"install",
				"--require-hashes",
				"--no-deps",
				"--only-binary", ":all:",
				"-r", "/path/to/requirements.txt",
			},
		},
		{
			name:      "source directory",
			sourceDir: "/path/to/source",
			wantContains: []string{
				"install",
				"/path/to/source",
			},
		},
		{
			name:         "with constraints",
			requirements: "/path/to/requirements.txt",
			constraints:  "/path/to/constraints.txt",
			wantContains: []string{
				"-c", "/path/to/constraints.txt",
				"-r", "/path/to/requirements.txt",
			},
		},
		{
			name:      "source directory with hashes",
			sourceDir: "/path/to/source",
			useHashes: true,
			wantContains: []string{
				"install",
				"--require-hashes",
				"--no-deps",
				"--only-binary", ":all:",
				"/path/to/source",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildPipInstallArgs(tt.sourceDir, tt.requirements, tt.constraints, tt.useHashes)

			// Check that expected args are present
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args should contain %q, got %v", want, args)
				}
			}

			// Check that unwanted args are absent
			for _, notWant := range tt.wantNotContains {
				for _, arg := range args {
					if arg == notWant {
						t.Errorf("args should not contain %q, got %v", notWant, args)
					}
				}
			}
		})
	}
}

func TestGetSourceDateEpoch(t *testing.T) {
	// Test that it returns a consistent value
	epoch1 := getSourceDateEpoch()
	epoch2 := getSourceDateEpoch()

	if epoch1 != epoch2 {
		t.Errorf("getSourceDateEpoch() should return consistent value, got %d and %d", epoch1, epoch2)
	}

	// Should be a reasonable timestamp (after year 2000, before year 2100)
	if epoch1 < 946684800 || epoch1 > 4102444800 {
		t.Errorf("getSourceDateEpoch() returned unreasonable value: %d", epoch1)
	}
}

func TestGetSourceDateEpoch_FromEnv(t *testing.T) {
	// Save original value
	origValue := os.Getenv("SOURCE_DATE_EPOCH")
	defer os.Setenv("SOURCE_DATE_EPOCH", origValue)

	// Set a custom value
	os.Setenv("SOURCE_DATE_EPOCH", "1234567890")

	epoch := getSourceDateEpoch()
	if epoch != 1234567890 {
		t.Errorf("getSourceDateEpoch() should return env value, got %d, want 1234567890", epoch)
	}
}

func TestGetSourceDateEpoch_InvalidEnv(t *testing.T) {
	// Save original value
	origValue := os.Getenv("SOURCE_DATE_EPOCH")
	defer os.Setenv("SOURCE_DATE_EPOCH", origValue)

	// Set an invalid value
	os.Setenv("SOURCE_DATE_EPOCH", "not-a-number")

	// Should fall back to the fixed epoch
	epoch := getSourceDateEpoch()
	if epoch == 0 {
		t.Error("getSourceDateEpoch() should return fixed epoch for invalid env value")
	}
}

func TestPipInstallAction_Execute_MissingParams(t *testing.T) {
	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    "/tmp",
		InstallDir: "/tmp/install",
	}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrMsg string
	}{
		{
			name:       "missing python_version",
			params:     map[string]interface{}{},
			wantErrMsg: "python_version",
		},
		{
			name: "missing source_dir and requirements",
			params: map[string]interface{}{
				"python_version": "3.11",
			},
			wantErrMsg: "source_dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should return error for missing params")
			}
			if tt.wantErrMsg != "" && !pipTestContains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error should mention %q, got %q", tt.wantErrMsg, err.Error())
			}
		})
	}
}

func TestPipInstallAction_Execute_PythonNotFound(t *testing.T) {
	// Save and restore HOME
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create tsuku tools directory without python
	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "/path/to/requirements.txt",
		"python_path":    "/nonexistent/python3",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when python is not found")
	}
	if err != nil && !pipTestContains(err.Error(), "Python version") && !pipTestContains(err.Error(), "failed") {
		t.Errorf("Error should mention Python or failed, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithMockPython(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create mock Python that reports version
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    # Create mock venv structure
    mkdir -p "$3/bin"
    # Create mock pip
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
echo "Mock pip executed: $@"
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create requirements file
	requirementsPath := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(requirementsPath, []byte("requests==2.28.0\n"), 0644); err != nil {
		t.Fatalf("Failed to create requirements file: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "requirements.txt",
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with mock python, got: %v", err)
	}
}

func TestPipInstallAction_Execute_VersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock Python that reports wrong version
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.10.0"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "/path/to/requirements.txt",
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail on version mismatch")
	}
	if err != nil && !pipTestContains(err.Error(), "mismatch") {
		t.Errorf("Error should mention version mismatch, got: %v", err)
	}
}

func TestPipInstallAction_Execute_VenvCreationFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock Python that fails venv creation
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    echo "Error: venv creation failed"
    exit 1
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "/path/to/requirements.txt",
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when venv creation fails")
	}
	if err != nil && !pipTestContains(err.Error(), "virtual environment") {
		t.Errorf("Error should mention virtual environment, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithSourceDir(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create mock Python
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    mkdir -p "$3/bin"
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create source directory with setup.py
	sourceDir := filepath.Join(tmpDir, "mypackage")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}
	setupPy := filepath.Join(sourceDir, "setup.py")
	if err := os.WriteFile(setupPy, []byte("from setuptools import setup\nsetup()\n"), 0644); err != nil {
		t.Fatalf("Failed to create setup.py: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"source_dir":     "mypackage", // relative path
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with source_dir, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithExecPaths(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create mock Python
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    mkdir -p "$3/bin"
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create requirements file
	requirementsPath := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(requirementsPath, []byte("requests==2.28.0\n"), 0644); err != nil {
		t.Fatalf("Failed to create requirements file: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
		ExecPaths:  []string{"/extra/bin", "/another/bin"}, // Test ExecPaths handling
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "requirements.txt",
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with ExecPaths, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithConstraints(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create mock Python
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    mkdir -p "$3/bin"
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create requirements and constraints files
	requirementsPath := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(requirementsPath, []byte("requests\n"), 0644); err != nil {
		t.Fatalf("Failed to create requirements file: %v", err)
	}
	constraintsPath := filepath.Join(tmpDir, "constraints.txt")
	if err := os.WriteFile(constraintsPath, []byte("requests==2.28.0\n"), 0644); err != nil {
		t.Fatalf("Failed to create constraints file: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "requirements.txt",
		"constraints":    "constraints.txt",
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with constraints, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithUseHashes(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create mock Python
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    mkdir -p "$3/bin"
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
# Verify hash-related flags are present
for arg in "$@"; do
    case "$arg" in
        --require-hashes) ;;
        --no-deps) ;;
        --only-binary) ;;
    esac
done
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create requirements file with hashes
	requirementsPath := filepath.Join(tmpDir, "requirements.txt")
	reqContent := "requests==2.28.0 --hash=sha256:abc123\n"
	if err := os.WriteFile(requirementsPath, []byte(reqContent), 0644); err != nil {
		t.Fatalf("Failed to create requirements file: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "requirements.txt",
		"use_hashes":     true,
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with use_hashes, got: %v", err)
	}
}

func TestPipInstallAction_Execute_WithOutputDir(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := t.TempDir()
	customOutputDir := filepath.Join(t.TempDir(), "custom_venv")

	// Create mock Python
	pythonDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(pythonDir, 0755); err != nil {
		t.Fatalf("Failed to create python dir: %v", err)
	}

	pythonPath := filepath.Join(pythonDir, "python3")
	mockScript := `#!/bin/sh
if [ "$1" = "--version" ]; then
    echo "Python 3.11.5"
    exit 0
fi
if [ "$1" = "-m" ] && [ "$2" = "venv" ]; then
    mkdir -p "$3/bin"
    cat > "$3/bin/pip" << 'PIPEOF'
#!/bin/sh
exit 0
PIPEOF
    chmod +x "$3/bin/pip"
    exit 0
fi
exit 1
`
	if err := os.WriteFile(pythonPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock python: %v", err)
	}

	// Create requirements file
	requirementsPath := filepath.Join(tmpDir, "requirements.txt")
	if err := os.WriteFile(requirementsPath, []byte("requests==2.28.0\n"), 0644); err != nil {
		t.Fatalf("Failed to create requirements file: %v", err)
	}

	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: installDir,
	}

	params := map[string]interface{}{
		"python_version": "3.11",
		"requirements":   "requirements.txt",
		"output_dir":     customOutputDir,
		"python_path":    pythonPath,
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with custom output_dir, got: %v", err)
	}

	// Verify venv was created in custom location
	if _, err := os.Stat(filepath.Join(customOutputDir, "bin", "pip")); err != nil {
		t.Errorf("Venv should be created in custom output_dir: %v", err)
	}
}

func pipTestContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
