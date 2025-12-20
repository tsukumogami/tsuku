package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupBuildEnvAction_Name(t *testing.T) {
	t.Parallel()
	action := &SetupBuildEnvAction{}
	if action.Name() != "setup_build_env" {
		t.Errorf("Name() = %q, want %q", action.Name(), "setup_build_env")
	}
}

func TestSetupBuildEnvAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := &SetupBuildEnvAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestSetupBuildEnvAction_Execute_NoDependencies(t *testing.T) {
	t.Parallel()
	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:      context.Background(),
		ToolsDir:     t.TempDir(),
		Dependencies: ResolvedDeps{InstallTime: make(map[string]string)},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Execute() with no dependencies failed: %v", err)
	}
}

func TestSetupBuildEnvAction_Execute_WithDependencies(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directories with standard layout
	dep1Dir := filepath.Join(toolsDir, "zlib-1.2.11")
	dep2Dir := filepath.Join(toolsDir, "openssl-3.0.0")

	// Create directory structure for dep1
	if err := os.MkdirAll(filepath.Join(dep1Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep1Dir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create directory structure for dep2
	if err := os.MkdirAll(filepath.Join(dep2Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep2Dir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}

	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{
				"zlib":    "1.2.11",
				"openssl": "3.0.0",
			},
		},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Execute() with dependencies failed: %v", err)
	}
}

func TestSetupBuildEnvAction_Execute_MissingDirectories(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directory without standard subdirectories
	depDir := filepath.Join(toolsDir, "custom-1.0.0")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatal(err)
	}

	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"custom": "1.0.0"},
		},
	}

	// Should succeed even with missing directories (graceful degradation)
	err := action.Execute(ctx, nil)
	if err != nil {
		t.Errorf("Execute() with missing directories should succeed: %v", err)
	}
}

func TestSetupBuildEnvAction_Registered(t *testing.T) {
	t.Parallel()
	// Verify setup_build_env is registered as a primitive action
	if !IsPrimitive("setup_build_env") {
		t.Error("setup_build_env should be registered as a primitive action")
	}

	// Verify it's in the action registry
	action := Get("setup_build_env")
	if action == nil {
		t.Error("setup_build_env should be registered in the action registry")
	}
}
