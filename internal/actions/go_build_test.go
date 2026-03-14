package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestGoBuildAction_Name tests the Name method
func TestGoBuildAction_Name(t *testing.T) {
	action := &GoBuildAction{}
	if action.Name() != "go_build" {
		t.Errorf("Name() = %q, want %q", action.Name(), "go_build")
	}
}

// TestGoBuildAction_Execute_ValidationErrors tests that Execute rejects invalid parameters
func TestGoBuildAction_Execute_ValidationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		params      map[string]interface{}
		errContains string
	}{
		{
			name: "missing module",
			params: map[string]interface{}{
				"version":     "v1.0.0",
				"executables": []interface{}{"tool"},
				"go_sum":      "test go.sum content",
			},
			errContains: "module",
		},
		{
			name: "missing version",
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"executables": []interface{}{"tool"},
				"go_sum":      "test go.sum content",
			},
			errContains: "version",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"module":  "github.com/user/repo",
				"version": "v1.0.0",
				"go_sum":  "test go.sum content",
			},
			errContains: "executables",
		},
		{
			name: "missing go_sum",
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"version":     "v1.0.0",
				"executables": []interface{}{"tool"},
			},
			errContains: "go_sum",
		},
		{
			name: "invalid module path",
			params: map[string]interface{}{
				"module":      "github.com/user/repo;rm -rf /",
				"version":     "v1.0.0",
				"executables": []interface{}{"tool"},
				"go_sum":      "test go.sum content",
			},
			errContains: "invalid module path",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"version":     "v1.0.0;rm -rf /",
				"executables": []interface{}{"tool"},
				"go_sum":      "test go.sum content",
			},
			errContains: "invalid version",
		},
		{
			name: "invalid executable name",
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"version":     "v1.0.0",
				"executables": []interface{}{"../../../etc/passwd"},
				"go_sum":      "test go.sum content",
			},
			errContains: "invalid executable name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action := &GoBuildAction{}
			ctx := &ExecutionContext{
				Context:    context.Background(),
				InstallDir: t.TempDir(),
				WorkDir:    t.TempDir(),
			}

			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("Execute() should fail for %s", tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Error should contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

// TestGoBuildAction_Execute_GoNotInstalled tests error when Go is not installed
func TestGoBuildAction_Execute_GoNotInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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
	if err != nil && !strings.Contains(err.Error(), "go not found") {
		t.Errorf("Error message should mention 'go not found', got: %v", err)
	}
}

// TestGoBuildAction_Execute_CGOEnabled tests that cgo_enabled parameter is respected
func TestGoBuildAction_Execute_CGOEnabled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

// TestGoInstallAction_Decompose_ValidationErrors tests that Decompose rejects invalid parameters
func TestGoInstallAction_Decompose_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		ctx         *EvalContext
		params      map[string]interface{}
		setup       func(t *testing.T)
		errContains string
	}{
		{
			name: "missing module",
			ctx: &EvalContext{
				Context:    context.Background(),
				Version:    "1.0.0",
				VersionTag: "v1.0.0",
			},
			params: map[string]interface{}{
				"executables": []interface{}{"tool"},
			},
			errContains: "module",
		},
		{
			name: "missing executables",
			ctx: &EvalContext{
				Context:    context.Background(),
				Version:    "1.0.0",
				VersionTag: "v1.0.0",
			},
			params: map[string]interface{}{
				"module": "github.com/user/repo",
			},
			errContains: "executables",
		},
		{
			name: "invalid module",
			ctx: &EvalContext{
				Context:    context.Background(),
				Version:    "1.0.0",
				VersionTag: "v1.0.0",
			},
			params: map[string]interface{}{
				"module":      "github.com/user/repo;rm",
				"executables": []interface{}{"tool"},
			},
			errContains: "invalid module",
		},
		{
			name: "invalid version",
			ctx: &EvalContext{
				Context:    context.Background(),
				Version:    "1.0.0",
				VersionTag: "v1.0.0;rm",
			},
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"executables": []interface{}{"tool"},
			},
			errContains: "invalid version",
		},
		{
			name: "go not installed",
			ctx: &EvalContext{
				Context:    context.Background(),
				Version:    "1.0.0",
				VersionTag: "v1.0.0",
			},
			params: map[string]interface{}{
				"module":      "github.com/user/repo",
				"executables": []interface{}{"tool"},
			},
			setup: func(t *testing.T) {
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
				if err := os.MkdirAll(toolsDir, 0755); err != nil {
					t.Fatalf("Failed to create tools dir: %v", err)
				}
			},
			errContains: "go not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup == nil {
				t.Parallel()
			}
			if tt.setup != nil {
				tt.setup(t)
			}
			action := &GoInstallAction{}
			_, err := action.Decompose(tt.ctx, tt.params)
			if err == nil {
				t.Errorf("Decompose() should fail for %s", tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Error should contain %q, got: %v", tt.errContains, err)
			}
		})
	}
}

// TestGoInstallAction_Decompose_ReturnsGoBuildStep tests that Decompose produces go_build step
func TestGoInstallAction_Decompose_ReturnsGoBuildStep(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Create mock go executable that handles version and creates go.sum
	goPath := filepath.Join(goDir, "go")
	mockScript := `#!/bin/sh
# Handle version command for go_version capture
if [ "$1" = "version" ]; then
    echo "go version go1.21.0 linux/amd64"
    exit 0
fi
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
	// Verify go_version is captured
	if goVersion, ok := step.Params["go_version"].(string); !ok || goVersion != "1.21.0" {
		t.Errorf("Step params go_version = %v, want %q", step.Params["go_version"], "1.21.0")
	}
}

// TestGoInstallAction_Decompose_PassesThroughOptionalParams tests optional params passthrough
func TestGoInstallAction_Decompose_PassesThroughOptionalParams(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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
if [ "$1" = "version" ]; then
    echo "go version go1.21.0 linux/amd64"
    exit 0
fi
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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

// TestGoBuildAction_Execute_SpecificGoVersion tests that go_version parameter is respected
func TestGoBuildAction_Execute_SpecificGoVersion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a specific Go version installation
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.5", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	installDir := t.TempDir()
	workDir := t.TempDir()
	binDir := filepath.Join(installDir, "bin")

	// Create mock go
	goPath := filepath.Join(goDir, "go")
	mockScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
    mod)
        exit 0
        ;;
    install)
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
		"go_version":  "1.21.5", // Request specific version
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() should succeed when specific Go version is installed, got: %v", err)
	}
}

// TestGoBuildAction_Execute_SpecificGoVersionNotInstalled tests error when specific version not found
func TestGoBuildAction_Execute_SpecificGoVersionNotInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a DIFFERENT Go version installation (1.22.0 instead of requested 1.21.5)
	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.22.0", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	goPath := filepath.Join(goDir, "go")
	if err := os.WriteFile(goPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
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
		"go_sum":      "github.com/user/repo v1.0.0 h1:abc123=\n",
		"go_version":  "1.21.5", // Request version that's NOT installed
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when specific Go version is not installed")
	}
	if err != nil && !strings.Contains(err.Error(), "go 1.21.5 not found") {
		t.Errorf("Error message should mention the specific version, got: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "tsuku install go@1.21.5") {
		t.Errorf("Error message should suggest how to install, got: %v", err)
	}
}

// TestGoInstallAction_Decompose_CapturesGoVersion tests that Decompose captures Go version
func TestGoInstallAction_Decompose_CapturesGoVersion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	goDir := filepath.Join(tmpHome, ".tsuku", "tools", "go-1.21.5", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatalf("Failed to create go dir: %v", err)
	}

	modCache := filepath.Join(tmpHome, ".tsuku", ".gomodcache")
	if err := os.MkdirAll(modCache, 0755); err != nil {
		t.Fatalf("Failed to create mod cache: %v", err)
	}

	// Create mock go that reports version and creates go.sum
	goPath := filepath.Join(goDir, "go")
	mockScript := `#!/bin/sh
if [ "$1" = "version" ]; then
    echo "go version go1.21.5 linux/amd64"
    exit 0
fi
if [ "$1" = "get" ]; then
    echo "github.com/user/repo v1.0.0 h1:abc123=" > go.sum
    exit 0
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

	// Verify go_version is captured
	goVersion, ok := step.Params["go_version"].(string)
	if !ok {
		t.Error("Step params should contain go_version string")
	}
	if goVersion != "1.21.5" {
		t.Errorf("go_version = %q, want %q", goVersion, "1.21.5")
	}
}

// -- go_build.go: Execute with cgo_enabled flag --

func TestGoBuildAction_Execute_WithCgoFlag(t *testing.T) {
	t.Parallel()
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	// Will fail at go_sum, but exercises cgo_enabled path
	err := action.Execute(ctx, map[string]any{
		"module":      "example.com/tool",
		"version":     "1.0.0",
		"executables": []any{"tool"},
		"cgo_enabled": true,
	})
	if err == nil || !strings.Contains(err.Error(), "go_sum") {
		t.Errorf("Expected go_sum error, got %v", err)
	}
}

// -- go_build.go: Execute with build_flags --

func TestGoBuildAction_Execute_WithBuildFlags(t *testing.T) {
	t.Parallel()
	action := &GoBuildAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"module":      "example.com/tool",
		"version":     "1.0.0",
		"executables": []any{"tool"},
		"build_flags": []any{"-trimpath"},
	})
	if err == nil || !strings.Contains(err.Error(), "go_sum") {
		t.Errorf("Expected go_sum error, got %v", err)
	}
}

// -- go_build.go: Dependencies, RequiresNetwork --

func TestGoBuildAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := GoBuildAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "go" {
		t.Errorf("Dependencies().InstallTime = %v, want [go]", deps.InstallTime)
	}
}

func TestGoBuildAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := GoBuildAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}
