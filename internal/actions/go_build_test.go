package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestGoBuildAction_Name tests the Name method
func TestGoBuildAction_Name(t *testing.T) {
	action := &GoBuildAction{}
	if action.Name() != "go_build" {
		t.Errorf("Name() = %q, want %q", action.Name(), "go_build")
	}
}

// TestGoBuildAction_Execute_MissingModule tests that Execute fails without module
func TestGoBuildAction_Execute_MissingModule(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'module' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "module") {
		t.Errorf("Error message should mention 'module', got: %v", err)
	}
}

// TestGoBuildAction_Execute_MissingVersion tests that Execute fails without version
func TestGoBuildAction_Execute_MissingVersion(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'version' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "version") {
		t.Errorf("Error message should mention 'version', got: %v", err)
	}
}

// TestGoBuildAction_Execute_MissingExecutables tests that Execute fails without executables
func TestGoBuildAction_Execute_MissingExecutables(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":  "github.com/user/repo",
		"version": "v1.0.0",
		"go_sum":  "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "executables") {
		t.Errorf("Error message should mention 'executables', got: %v", err)
	}
}

// TestGoBuildAction_Execute_MissingGoSum tests that Execute fails without go_sum
func TestGoBuildAction_Execute_MissingGoSum(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'go_sum' parameter is missing")
	}
	if err != nil && !containsStr(err.Error(), "go_sum") {
		t.Errorf("Error message should mention 'go_sum', got: %v", err)
	}
}

// TestGoBuildAction_Execute_InvalidModule tests command injection in module
func TestGoBuildAction_Execute_InvalidModule(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo;rm -rf /",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in module path")
	}
	if err != nil && !containsStr(err.Error(), "invalid module path") {
		t.Errorf("Error message should mention 'invalid module path', got: %v", err)
	}
}

// TestGoBuildAction_Execute_InvalidVersion tests command injection in version
func TestGoBuildAction_Execute_InvalidVersion(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0;rm -rf /",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in version")
	}
	if err != nil && !containsStr(err.Error(), "invalid version") {
		t.Errorf("Error message should mention 'invalid version', got: %v", err)
	}
}

// TestGoBuildAction_Execute_InvalidExecutable tests path traversal in executable
func TestGoBuildAction_Execute_InvalidExecutable(t *testing.T) {
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"../../../etc/passwd"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with path traversal in executable name")
	}
	if err != nil && !containsStr(err.Error(), "invalid executable name") {
		t.Errorf("Error message should mention 'invalid executable name', got: %v", err)
	}
}

// TestGoBuildAction_Execute_GoNotInstalled tests error when Go is not installed
func TestGoBuildAction_Execute_GoNotInstalled(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when Go is not installed")
	}
	if err != nil && !containsStr(err.Error(), "go not found") {
		t.Errorf("Error message should mention 'go not found', got: %v", err)
	}
}

// TestGoBuildAction_Execute_CGOEnabled tests that cgo_enabled parameter is respected
func TestGoBuildAction_Execute_CGOEnabled(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure with a mock go that captures env
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go executable that fails but captures env
	goPath := filepath.Join(goDir, "go")
	mockScript := "#!/bin/sh\necho \"CGO_ENABLED=$CGO_ENABLED\" >&2\nexit 1\n"
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
		"cgo_enabled": true,
	}

	// This will fail but we're testing that the code path runs with CGO enabled
	_ = action.Execute(ctx, params)
	// Just verifying we didn't panic and the cgo_enabled case is covered
}

// TestGoBuildAction_Execute_BuildFlags tests custom build flags
func TestGoBuildAction_Execute_BuildFlags(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create mock go that fails
	goPath := filepath.Join(goDir, "go")
	mockScript := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "test go.sum content",
		"build_flags": []interface{}{"-ldflags=-s -w"},
	}

	// This will fail but we're testing that custom build flags are accepted
	_ = action.Execute(ctx, params)
}

// TestGoBuildAction_IsPrimitive tests that go_build is registered as primitive
func TestGoBuildAction_IsPrimitive(t *testing.T) {
	if !IsPrimitive("go_build") {
		t.Error("go_build should be registered as a primitive action")
	}
}

// TestGoInstallAction_Decomposable tests that GoInstallAction implements Decomposable
func TestGoInstallAction_Decomposable(t *testing.T) {
	action := &GoInstallAction{}
	_, ok := interface{}(action).(Decomposable)
	if !ok {
		t.Error("GoInstallAction should implement Decomposable interface")
	}
}

// TestGoInstallAction_Decompose_MissingModule tests Decompose fails without module
func TestGoInstallAction_Decompose_MissingModule(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"executables": []interface{}{"tool"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail when 'module' parameter is missing")
	}
}

// TestGoInstallAction_Decompose_MissingExecutables tests Decompose fails without executables
func TestGoInstallAction_Decompose_MissingExecutables(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"module": "github.com/user/repo",
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail when 'executables' parameter is missing")
	}
}

// TestGoInstallAction_Decompose_InvalidModule tests Decompose fails with invalid module
func TestGoInstallAction_Decompose_InvalidModule(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo;rm",
		"executables": []interface{}{"tool"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail with invalid module path")
	}
}

// TestGoInstallAction_Decompose_InvalidVersion tests Decompose fails with invalid version
func TestGoInstallAction_Decompose_InvalidVersion(t *testing.T) {
	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0;rm",
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail with invalid version")
	}
}

// TestGoInstallAction_Decompose_GoNotInstalled tests Decompose fails when Go not installed
func TestGoInstallAction_Decompose_GoNotInstalled(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail when Go is not installed")
	}
	if err != nil && !containsStr(err.Error(), "go not found") {
		t.Errorf("Error message should mention 'go not found', got: %v", err)
	}
}

// TestGoInstallAction_Decompose_ReturnsGoBuildStep tests that Decompose produces go_build step
func TestGoInstallAction_Decompose_ReturnsGoBuildStep(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	// Create Go installation structure with a mock go that writes go.sum
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	// Create module cache directory
	modCache := filepath.Join(tmpHome, ".tsuku", ".gomodcache")
	if err := os.MkdirAll(modCache, 0755); err != nil {
		t.Fatalf("Failed to create mod cache: %v", err)
	}

	// Create mock go executable that creates go.sum
	goPath := filepath.Join(goDir, "go")
	mockScript := `#!/bin/sh
# Create go.sum with test content when 'go get' is called
if [ "$1" = "get" ]; then
    # Write to go.sum in the current directory (set by Dir)
    echo "github.com/user/repo v1.0.0 h1:abc123=" > go.sum
    echo "github.com/user/repo v1.0.0/go.mod h1:def456=" >> go.sum
fi
exit 0
`
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() failed: %v", err)
	}

	if len(steps) != 1 {
		t.Fatalf("Decompose() should return 1 step, got %d", len(steps))
	}

	step := steps[0]
	if step.Action != "go_build" {
		t.Errorf("Step action = %q, want %q", step.Action, "go_build")
	}

	// Verify params
	if module, ok := step.Params["module"].(string); !ok || module != "github.com/user/repo" {
		t.Errorf("Step params module = %v, want %q", step.Params["module"], "github.com/user/repo")
	}
	if version, ok := step.Params["version"].(string); !ok || version != "v1.0.0" {
		t.Errorf("Step params version = %v, want %q", step.Params["version"], "v1.0.0")
	}
	if _, ok := step.Params["go_sum"].(string); !ok {
		t.Error("Step params should contain go_sum string")
	}
	if execs, ok := step.Params["executables"].([]string); !ok || len(execs) == 0 {
		t.Error("Step params should contain executables")
	}
}

// TestGoInstallAction_Decompose_PassesThroughOptionalParams tests optional params passthrough
func TestGoInstallAction_Decompose_PassesThroughOptionalParams(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	modCache := filepath.Join(tmpHome, ".tsuku", ".gomodcache")
	if err := os.MkdirAll(modCache, 0755); err != nil {
		t.Fatalf("Failed to create mod cache: %v", err)
	}

	goPath := filepath.Join(goDir, "go")
	mockScript := `#!/bin/sh
if [ "$1" = "get" ]; then
    echo "test h1:abc=" > go.sum
fi
exit 0
`
	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoInstallAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"executables": []interface{}{"tool"},
		"cgo_enabled": true,
		"build_flags": []interface{}{"-ldflags=-s -w"},
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() failed: %v", err)
	}

	step := steps[0]

	// Verify optional params are passed through
	if cgo, ok := step.Params["cgo_enabled"].(bool); !ok || !cgo {
		t.Error("cgo_enabled should be passed through as true")
	}
	if flags, ok := step.Params["build_flags"].([]string); !ok || len(flags) == 0 {
		t.Error("build_flags should be passed through")
	}
}

// TestBuildGoEnv tests the buildGoEnv helper function
func TestBuildGoEnv(t *testing.T) {
	goDir := "/usr/local/go/bin"
	binDir := "/tmp/bin"
	modCache := "/tmp/modcache"

	// Test offline mode
	env := buildGoEnv(goDir, binDir, modCache, false, true)

	foundGOBIN := false
	foundGOMODCACHE := false
	foundCGO := false
	foundGOPROXY := false
	foundGOSUMDB := false

	for _, e := range env {
		switch {
		case e == "GOBIN="+binDir:
			foundGOBIN = true
		case e == "GOMODCACHE="+modCache:
			foundGOMODCACHE = true
		case e == "CGO_ENABLED=0":
			foundCGO = true
		case e == "GOPROXY=off":
			foundGOPROXY = true
		case e == "GOSUMDB=off":
			foundGOSUMDB = true
		}
	}

	if !foundGOBIN {
		t.Error("env should contain GOBIN")
	}
	if !foundGOMODCACHE {
		t.Error("env should contain GOMODCACHE")
	}
	if !foundCGO {
		t.Error("env should contain CGO_ENABLED=0")
	}
	if !foundGOPROXY {
		t.Error("env should contain GOPROXY=off in offline mode")
	}
	if !foundGOSUMDB {
		t.Error("env should contain GOSUMDB=off in offline mode")
	}

	// Test online mode with CGO enabled
	env = buildGoEnv(goDir, binDir, modCache, true, false)

	foundCGO1 := false
	foundGOPROXYOnline := false

	for _, e := range env {
		switch {
		case e == "CGO_ENABLED=1":
			foundCGO1 = true
		case e == "GOPROXY=https://proxy.golang.org,direct":
			foundGOPROXYOnline = true
		}
	}

	if !foundCGO1 {
		t.Error("env should contain CGO_ENABLED=1 when cgoEnabled is true")
	}
	if !foundGOPROXYOnline {
		t.Error("env should contain proxy URL in online mode")
	}
}

// TestGoBuildAction_Execute_Success tests successful build with mock
func TestGoBuildAction_Execute_Success(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)

	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	workDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go that handles download, verify, and install
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
    mod)
        # download or verify - just succeed
        exit 0
        ;;
    install)
        # Create the binary
        mkdir -p %s
        touch %s/tool
        chmod +x %s/tool
        exit 0
        ;;
esac
exit 0
`, binDir, binDir, binDir)

	if err := os.WriteFile(goPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("Failed to create mock go: %v", err)
	}

	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: installDir,
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"module":      "github.com/user/repo",
		"version":     "v1.0.0",
		"executables": []interface{}{"tool"},
		"go_sum":      "github.com/user/repo v1.0.0 h1:abc123=\n",
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed, got: %v", err)
	}

	// Verify executable exists
	toolPath := filepath.Join(binDir, "tool")
	if _, err := os.Stat(toolPath); err != nil {
		t.Errorf("Expected executable should exist at %s", toolPath)
	}
}
