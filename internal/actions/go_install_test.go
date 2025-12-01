package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestIsValidGoModule tests Go module path validation
func TestIsValidGoModule(t *testing.T) {
	tests := []struct {
		name      string
		module    string
		wantValid bool
	}{
		// Valid module paths
		{"simple github", "github.com/user/repo", true},
		{"with subdirectory", "github.com/user/repo/cmd/tool", true},
		{"golang.org module", "golang.org/x/tools", true},
		{"with hyphens", "github.com/some-user/some-repo", true},
		{"with underscores", "github.com/some_user/some_repo", true},
		{"with dots", "github.com/user/repo.v2", true},
		{"mixed case", "github.com/User/Repo", true},
		{"long path", "github.com/org/repo/internal/pkg/subpkg", true},
		{"gopkg.in format", "gopkg.in/yaml.v3", true},
		{"k8s.io module", "k8s.io/client-go", true},

		// Invalid module paths
		{"empty string", "", false},
		{"no slash", "github.com", false},
		{"starts with number", "123.com/user/repo", false},
		{"starts with hyphen", "-github.com/user/repo", false},
		{"contains space", "github.com/user/repo name", false},
		{"contains semicolon", "github.com/user/repo;rm", false},
		{"contains dollar", "github.com/user/$repo", false},
		{"contains backtick", "github.com/user/`repo`", false},
		{"contains pipe", "github.com/user|repo", false},
		{"contains ampersand", "github.com/user&repo", false},
		{"double slash", "github.com//user/repo", false},
		{"path traversal", "github.com/user/../etc", false},
		{"path traversal 2", "github.com/../../../etc/passwd", false},
		{"too long", "github.com/" + string(make([]byte, 250)), false},
		{"shell command", "github.com/user/repo;rm -rf /", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidGoModule(tt.module)
			if got != tt.wantValid {
				t.Errorf("isValidGoModule(%q) = %v, want %v", tt.module, got, tt.wantValid)
			}
		})
	}
}

// TestIsValidGoVersion tests version string validation
func TestIsValidGoVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantValid bool
	}{
		// Valid versions
		{"empty (means latest)", "", true},
		{"latest keyword", "latest", true},
		{"simple semver", "1.0.0", true},
		{"with v prefix", "v1.0.0", true},
		{"two part", "v1.0", true},
		{"single digit", "v1", true},
		{"with prerelease alpha", "v1.0.0-alpha", true},
		{"with prerelease beta", "v1.0.0-beta1", true},
		{"with prerelease rc", "v1.0.0-rc.1", true},
		{"with build metadata", "v1.0.0+build", true},
		{"complex prerelease", "v1.0.0-alpha.1+build.123", true},
		{"large version", "v100.200.300", true},
		{"git commit like", "v0.0.0-20240101120000-abcdef123456", true},

		// Invalid versions
		{"starts with hyphen", "-1.0.0", false},
		{"starts with dot", ".1.0.0", false},
		{"contains space", "v1.0.0 ", false},
		{"shell metachar semicolon", "v1.0.0;rm", false},
		{"shell metachar dollar", "v1.0.0$var", false},
		{"shell metachar backtick", "v1.0.0`cmd`", false},
		{"shell metachar pipe", "v1.0.0|cat", false},
		{"shell metachar ampersand", "v1.0.0&cmd", false},
		{"path traversal", "v1.0.0/../etc", false},
		{"too long", "v1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", false}, // > 50 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidGoVersion(tt.version)
			if got != tt.wantValid {
				t.Errorf("isValidGoVersion(%q) = %v, want %v", tt.version, got, tt.wantValid)
			}
		})
	}
}

// TestGoInstallAction_Execute_MissingModule tests that Execute fails without module parameter
func TestGoInstallAction_Execute_MissingModule(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		// Missing "module" parameter
		"executables": []interface{}{"test-exe"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'module' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "module") {
		t.Errorf("Error message should mention 'module', got: %v", err)
	}
}

// TestGoInstallAction_Execute_MissingExecutables tests that Execute fails without executables
func TestGoInstallAction_Execute_MissingExecutables(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module": "github.com/user/repo",
		// Missing "executables" parameter
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "executables") {
		t.Errorf("Error message should mention 'executables', got: %v", err)
	}
}

// TestGoInstallAction_Execute_EmptyExecutables tests that Execute fails with empty executables
func TestGoInstallAction_Execute_EmptyExecutables(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{}, // Empty list
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' is empty")
	}
}

// TestGoInstallAction_Execute_InvalidModule tests that Execute fails with invalid module path
func TestGoInstallAction_Execute_InvalidModule(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo;rm -rf /", // Injection attempt
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in module path")
	}
	if err != nil && !containsStr(err.Error(), "invalid module path") {
		t.Errorf("Error message should mention 'invalid module path', got: %v", err)
	}
}

// TestGoInstallAction_Execute_InvalidVersion tests that Execute fails with invalid version
func TestGoInstallAction_Execute_InvalidVersion(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0;rm -rf /", // Injection attempt
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in version")
	}
	if err != nil && !containsStr(err.Error(), "invalid version") {
		t.Errorf("Error message should mention 'invalid version', got: %v", err)
	}
}

// TestGoInstallAction_Execute_InvalidExecutableName tests path traversal in executable names
func TestGoInstallAction_Execute_InvalidExecutableName(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"../../../etc/passwd"}, // Path traversal
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with path traversal in executable name")
	}
	if err != nil && !containsStr(err.Error(), "invalid executable name") {
		t.Errorf("Error message should mention 'invalid executable name', got: %v", err)
	}
}

// TestGoInstallAction_Execute_GoNotInstalled tests error when Go is not installed
func TestGoInstallAction_Execute_GoNotInstalled(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME with no Go installed
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create empty .tsuku/tools directory
	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when Go is not installed")
	}
	if err != nil && !containsStr(err.Error(), "go not found") {
		t.Errorf("Error message should mention 'go not found', got: %v", err)
	}
}

// TestResolveGo_NoGoInstalled tests ResolveGo when no Go is installed
func TestResolveGo_NoGoInstalled(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create empty .tsuku/tools directory (no go)
	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	result := ResolveGo()
	if result != "" {
		t.Errorf("ResolveGo() should return empty string when no Go installed, got: %s", result)
	}
}

// TestResolveGo_SingleVersion tests ResolveGo with one Go version
func TestResolveGo_SingleVersion(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go executable
	goPath := filepath.Join(goDir, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\necho mock"), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	result := ResolveGo()
	if result != goPath {
		t.Errorf("ResolveGo() = %q, want %q", result, goPath)
	}
}

// TestResolveGo_MultipleVersions tests that ResolveGo selects the latest version
func TestResolveGo_MultipleVersions(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create multiple Go versions
	versions := []string{"go-1.19.0", "go-1.21.0", "go-1.20.0"}
	for _, ver := range versions {
		goDir := filepath.Join(tmpHome, ".tsuku", "tools", ver, "bin")
		if err := os.MkdirAll(goDir, 0755); err != nil {
			t.Fatalf("Failed to create go dir: %v", err)
		}
		goPath := filepath.Join(goDir, "go")
		if err := os.WriteFile(goPath, []byte("#!/bin/sh\necho mock"), 0755); err != nil {
			t.Fatalf("Failed to create mock go: %v", err)
		}
	}

	result := ResolveGo()
	// Lexicographic sort means go-1.21.0 comes last
	expectedPath := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin", "go")
	if result != expectedPath {
		t.Errorf("ResolveGo() = %q, want %q (latest version)", result, expectedPath)
	}
}

// TestResolveGo_NonExecutable tests that non-executable go is skipped
func TestResolveGo_NonExecutable(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create non-executable go file (mode 0644, not 0755)
	goPath := filepath.Join(goDir, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\necho mock"), 0644); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	result := ResolveGo()
	if result != "" {
		t.Errorf("ResolveGo() should skip non-executable go, got: %s", result)
	}
}

// TestGoInstallAction_Execute_GoInstallFails tests that Execute handles go install failure
func TestGoInstallAction_Execute_GoInstallFails(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure with a mock go that always fails
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go executable that always fails
	goPath := filepath.Join(goDir, "go")
	mockScript := "#!/bin/sh\necho 'mock error: module not found' >&2\nexit 1\n"
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tool",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when go install fails")
	}
	if err != nil && !containsStr(err.Error(), "go install failed") {
		t.Errorf("Error message should mention 'go install failed', got: %v", err)
	}
}

// TestGoInstallAction_Execute_ExecutableNotCreated tests verification failure
func TestGoInstallAction_Execute_ExecutableNotCreated(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure with a mock go that succeeds but creates nothing
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go executable that succeeds but doesn't create the binary
	goPath := filepath.Join(goDir, "go")
	mockScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tool",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when expected executable is not created")
	}
	if err != nil && !containsStr(err.Error(), "expected executable") {
		t.Errorf("Error message should mention 'expected executable', got: %v", err)
	}
}

// TestGoInstallAction_Execute_EmptyVersion tests using @latest when version is empty
func TestGoInstallAction_Execute_EmptyVersion(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure with a mock go that fails (we just want to test the target building)
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go executable that captures args and fails
	goPath := filepath.Join(goDir, "go")
	mockScript := "#!/bin/sh\necho \"args: $@\" >&2\nexit 1\n"
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "", // Empty version should use @latest
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tool",
		"executables": []interface{}{"tool"},
	}

	// This will fail but we're testing that the code path runs
	_ = action.Execute(ctx, params)
	// Just verifying we didn't panic and the empty version case is covered
}

// TestGoInstallAction_Execute_Success tests successful installation
func TestGoInstallAction_Execute_Success(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go executable that creates the expected binary
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf("#!/bin/sh\nmkdir -p %s && touch %s/tool && chmod +x %s/tool\n", binDir, binDir, binDir)
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tool",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with mock go that creates binary, got: %v", err)
	}

	// Verify the executable was "created" (by our mock)
	toolPath := filepath.Join(binDir, "tool")
	if _, err := os.Stat(toolPath); err != nil {
		t.Errorf("Expected executable should exist at %s", toolPath)
	}
}

// TestGoInstallAction_Execute_SuccessWithDebug tests with TSUKU_DEBUG set
func TestGoInstallAction_Execute_SuccessWithDebug(t *testing.T) {
	// Save original HOME and TSUKU_DEBUG, restore after test
	origHome := os.Getenv("HOME")
	origDebug := os.Getenv("TSUKU_DEBUG")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("TSUKU_DEBUG", origDebug)
	}()

	// Enable debug mode
	os.Setenv("TSUKU_DEBUG", "1")

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go executable that creates the expected binary and outputs something
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf("#!/bin/sh\necho 'go: downloading module'\nmkdir -p %s && touch %s/tool && chmod +x %s/tool\n", binDir, binDir, binDir)
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tool",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed, got: %v", err)
	}
}

// TestGoInstallAction_Execute_MultipleExecutables tests verifying multiple executables
func TestGoInstallAction_Execute_MultipleExecutables(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go executable that creates multiple binaries
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf("#!/bin/sh\nmkdir -p %s && touch %s/tool1 %s/tool2 && chmod +x %s/tool1 %s/tool2\n", binDir, binDir, binDir, binDir, binDir)
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tools",
		"executables": []interface{}{"tool1", "tool2"},
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed with multiple executables, got: %v", err)
	}
}

// TestGoInstallAction_Execute_SecondExecutableMissing tests when only first executable is created
func TestGoInstallAction_Execute_SecondExecutableMissing(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go executable that only creates one of two expected binaries
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf("#!/bin/sh\nmkdir -p %s && touch %s/tool1 && chmod +x %s/tool1\n", binDir, binDir, binDir)
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		Version:    "v1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/example/tools",
		"executables": []interface{}{"tool1", "tool2"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when second executable is missing")
	}
	if err != nil && !containsStr(err.Error(), "expected executable") {
		t.Errorf("Error message should mention 'expected executable', got: %v", err)
	}
}

// TestGoInstallAction_Name tests the Name method
func TestGoInstallAction_Name(t *testing.T) {
	action := &GoInstallAction{}
	if action.Name() != "go_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "go_install")
	}
}

// Helper function to check if string contains substring
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
