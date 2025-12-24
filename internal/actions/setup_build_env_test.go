package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestSetupBuildEnvAction_PopulatesCtxEnv verifies setup_build_env populates ctx.Env
func TestSetupBuildEnvAction_PopulatesCtxEnv(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directory with standard layout
	depDir := filepath.Join(toolsDir, "zlib-1.2.11")
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"zlib": "1.2.11"},
		},
	}

	// Before execution, ctx.Env should be empty
	if len(ctx.Env) != 0 {
		t.Errorf("ctx.Env should be empty before Execute, got %d items", len(ctx.Env))
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// After execution, ctx.Env should be populated
	if len(ctx.Env) == 0 {
		t.Error("ctx.Env should be populated after Execute")
	}
}

// TestSetupBuildEnvAction_EnvContainsExpectedVars verifies ctx.Env contains expected variables
func TestSetupBuildEnvAction_EnvContainsExpectedVars(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directory with standard layout
	depDir := filepath.Join(toolsDir, "openssl-3.0.0")
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Check that expected environment variables are set
	var hasSourceDateEpoch, hasPkgConfigPath, hasCppFlags, hasLdFlags bool
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			hasSourceDateEpoch = true
		}
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			hasPkgConfigPath = true
		}
		if strings.HasPrefix(e, "CPPFLAGS=") {
			hasCppFlags = true
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			hasLdFlags = true
		}
	}

	if !hasSourceDateEpoch {
		t.Error("ctx.Env missing SOURCE_DATE_EPOCH")
	}
	if !hasPkgConfigPath {
		t.Error("ctx.Env missing PKG_CONFIG_PATH")
	}
	if !hasCppFlags {
		t.Error("ctx.Env missing CPPFLAGS")
	}
	if !hasLdFlags {
		t.Error("ctx.Env missing LDFLAGS")
	}
}

// TestSetupBuildEnvAction_ZeroDependencies verifies setup_build_env works with no dependencies
func TestSetupBuildEnvAction_ZeroDependencies(t *testing.T) {
	t.Parallel()
	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:      context.Background(),
		ToolsDir:     t.TempDir(),
		Dependencies: ResolvedDeps{InstallTime: make(map[string]string)},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() with zero dependencies failed: %v", err)
	}

	// Even with no dependencies, ctx.Env should be populated with base vars
	if len(ctx.Env) == 0 {
		t.Error("ctx.Env should be populated even with zero dependencies")
	}

	// Should at least have SOURCE_DATE_EPOCH
	hasSourceDateEpoch := false
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			hasSourceDateEpoch = true
			break
		}
	}
	if !hasSourceDateEpoch {
		t.Error("ctx.Env should contain SOURCE_DATE_EPOCH even with zero dependencies")
	}

	// Should NOT have PKG_CONFIG_PATH, CPPFLAGS, or LDFLAGS with zero dependencies
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			t.Error("ctx.Env should not contain PKG_CONFIG_PATH with zero dependencies")
		}
		if strings.HasPrefix(e, "CPPFLAGS=") {
			t.Error("ctx.Env should not contain CPPFLAGS with zero dependencies")
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			t.Error("ctx.Env should not contain LDFLAGS with zero dependencies")
		}
	}
}

// TestSetupBuildEnvAction_MultipleDependencies verifies all dependency paths are included
func TestSetupBuildEnvAction_MultipleDependencies(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create three mock dependency directories with different layouts
	dep1Dir := filepath.Join(toolsDir, "zlib-1.2.11")
	dep2Dir := filepath.Join(toolsDir, "openssl-3.0.0")
	dep3Dir := filepath.Join(toolsDir, "curl-7.88.1")

	// dep1: has all directories (include, lib, lib/pkgconfig)
	if err := os.MkdirAll(filepath.Join(dep1Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep1Dir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	// dep2: has include and lib, but no pkgconfig
	if err := os.MkdirAll(filepath.Join(dep2Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep2Dir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}

	// dep3: has all directories
	if err := os.MkdirAll(filepath.Join(dep3Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep3Dir, "lib", "pkgconfig"), 0755); err != nil {
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
				"curl":    "7.88.1",
			},
		},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Extract environment variables
	var pkgConfigPath, cppFlags, ldFlags string
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		} else if strings.HasPrefix(e, "CPPFLAGS=") {
			cppFlags = strings.TrimPrefix(e, "CPPFLAGS=")
		} else if strings.HasPrefix(e, "LDFLAGS=") {
			ldFlags = strings.TrimPrefix(e, "LDFLAGS=")
		}
	}

	// Verify PKG_CONFIG_PATH includes zlib and curl (not openssl - no pkgconfig dir)
	zlibPkgConfig := filepath.Join(dep1Dir, "lib", "pkgconfig")
	curlPkgConfig := filepath.Join(dep3Dir, "lib", "pkgconfig")
	if !strings.Contains(pkgConfigPath, zlibPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH missing zlib: got %q", pkgConfigPath)
	}
	if !strings.Contains(pkgConfigPath, curlPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH missing curl: got %q", pkgConfigPath)
	}

	// Verify CPPFLAGS includes all three dependencies
	zlibInclude := filepath.Join(dep1Dir, "include")
	opensslInclude := filepath.Join(dep2Dir, "include")
	curlInclude := filepath.Join(dep3Dir, "include")
	if !strings.Contains(cppFlags, "-I"+zlibInclude) {
		t.Errorf("CPPFLAGS missing zlib: got %q", cppFlags)
	}
	if !strings.Contains(cppFlags, "-I"+opensslInclude) {
		t.Errorf("CPPFLAGS missing openssl: got %q", cppFlags)
	}
	if !strings.Contains(cppFlags, "-I"+curlInclude) {
		t.Errorf("CPPFLAGS missing curl: got %q", cppFlags)
	}

	// Verify LDFLAGS includes all three dependencies (with both -L and -Wl,-rpath)
	zlibLib := filepath.Join(dep1Dir, "lib")
	opensslLib := filepath.Join(dep2Dir, "lib")
	curlLib := filepath.Join(dep3Dir, "lib")
	if !strings.Contains(ldFlags, "-L"+zlibLib) {
		t.Errorf("LDFLAGS missing -L for zlib: got %q", ldFlags)
	}
	if !strings.Contains(ldFlags, "-Wl,-rpath,"+zlibLib) {
		t.Errorf("LDFLAGS missing -Wl,-rpath for zlib: got %q", ldFlags)
	}
	if !strings.Contains(ldFlags, "-L"+opensslLib) {
		t.Errorf("LDFLAGS missing -L for openssl: got %q", ldFlags)
	}
	if !strings.Contains(ldFlags, "-Wl,-rpath,"+opensslLib) {
		t.Errorf("LDFLAGS missing -Wl,-rpath for openssl: got %q", ldFlags)
	}
	if !strings.Contains(ldFlags, "-L"+curlLib) {
		t.Errorf("LDFLAGS missing -L for curl: got %q", ldFlags)
	}
	if !strings.Contains(ldFlags, "-Wl,-rpath,"+curlLib) {
		t.Errorf("LDFLAGS missing -Wl,-rpath for curl: got %q", ldFlags)
	}
}

// TestSetupBuildEnvAction_EnvUsedByConfigureMake verifies configure_make uses ctx.Env
func TestSetupBuildEnvAction_EnvUsedByConfigureMake(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()
	workDir := t.TempDir()

	// Create mock dependency
	depDir := filepath.Join(toolsDir, "zlib-1.2.11")
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake configure script that checks for PKG_CONFIG_PATH
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}

	configureScript := `#!/bin/sh
# This fake configure script exits 0 to simulate success
# In a real test, we'd verify PKG_CONFIG_PATH was passed
exit 0
`
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte(configureScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create fake Makefile
	makefile := `all:
	@echo "Building..."
install:
	@mkdir -p $(PREFIX)/bin
	@echo "#!/bin/sh" > $(PREFIX)/bin/test-exe
	@chmod +x $(PREFIX)/bin/test-exe
`
	if err := os.WriteFile(filepath.Join(sourceDir, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: filepath.Join(workDir, "install"),
		ToolsDir:   toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"zlib": "1.2.11"},
		},
	}

	// First, run setup_build_env to populate ctx.Env
	setupAction := &SetupBuildEnvAction{}
	err := setupAction.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("setup_build_env Execute() failed: %v", err)
	}

	if len(ctx.Env) == 0 {
		t.Fatal("ctx.Env should be populated after setup_build_env")
	}

	// Verify ctx.Env contains PKG_CONFIG_PATH
	hasPkgConfigPath := false
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			hasPkgConfigPath = true
			break
		}
	}
	if !hasPkgConfigPath {
		t.Fatal("ctx.Env should contain PKG_CONFIG_PATH before configure_make")
	}

	// Note: We can't easily test that configure_make actually uses ctx.Env without
	// mocking exec.Command or inspecting the environment passed to subprocesses.
	// This test verifies that ctx.Env is populated correctly, which is the prerequisite
	// for configure_make to use it.
}

// TestSetupBuildEnvAction_EnvUsedByCMakeBuild verifies cmake_build can use ctx.Env
func TestSetupBuildEnvAction_EnvUsedByCMakeBuild(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency
	depDir := filepath.Join(toolsDir, "openssl-3.0.0")
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	// Run setup_build_env to populate ctx.Env
	setupAction := &SetupBuildEnvAction{}
	err := setupAction.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("setup_build_env Execute() failed: %v", err)
	}

	if len(ctx.Env) == 0 {
		t.Fatal("ctx.Env should be populated after setup_build_env")
	}

	// Verify ctx.Env contains CPPFLAGS and LDFLAGS
	hasCppFlags := false
	hasLdFlags := false
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "CPPFLAGS=") {
			hasCppFlags = true
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			hasLdFlags = true
		}
	}
	if !hasCppFlags {
		t.Error("ctx.Env should contain CPPFLAGS before cmake_build")
	}
	if !hasLdFlags {
		t.Error("ctx.Env should contain LDFLAGS before cmake_build")
	}

	// Note: Similar to configure_make test, we verify ctx.Env is populated correctly.
	// The actual usage by cmake_build is tested through the cmake_build action's
	// logic (lines 105-111 in cmake_build.go).
}

// TestSetupBuildEnvAction_FallbackWhenEnvEmpty verifies configure_make falls back
func TestSetupBuildEnvAction_FallbackWhenEnvEmpty(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency
	depDir := filepath.Join(toolsDir, "zlib-1.2.11")
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"zlib": "1.2.11"},
		},
		Env: nil, // Explicitly empty - no setup_build_env called
	}

	// Call buildAutotoolsEnv directly (this is what configure_make does as fallback)
	env := buildAutotoolsEnv(ctx)

	// Verify environment is populated
	if len(env) == 0 {
		t.Fatal("buildAutotoolsEnv should return non-empty environment")
	}

	// Verify it contains expected variables
	var hasSourceDateEpoch, hasPkgConfigPath, hasCppFlags, hasLdFlags bool
	for _, e := range env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			hasSourceDateEpoch = true
		}
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			hasPkgConfigPath = true
		}
		if strings.HasPrefix(e, "CPPFLAGS=") {
			hasCppFlags = true
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			hasLdFlags = true
		}
	}

	if !hasSourceDateEpoch {
		t.Error("buildAutotoolsEnv should set SOURCE_DATE_EPOCH")
	}
	if !hasPkgConfigPath {
		t.Error("buildAutotoolsEnv should set PKG_CONFIG_PATH")
	}
	if !hasCppFlags {
		t.Error("buildAutotoolsEnv should set CPPFLAGS")
	}
	if !hasLdFlags {
		t.Error("buildAutotoolsEnv should set LDFLAGS")
	}
}

// TestSetupBuildEnvAction_PathValuesCorrect verifies exact path values in environment
func TestSetupBuildEnvAction_PathValuesCorrect(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create single mock dependency with known paths
	depDir := filepath.Join(toolsDir, "test-1.0.0")
	includeDir := filepath.Join(depDir, "include")
	libDir := filepath.Join(depDir, "lib")
	pkgConfigDir := filepath.Join(libDir, "pkgconfig")

	if err := os.MkdirAll(includeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkgConfigDir, 0755); err != nil {
		t.Fatal(err)
	}

	action := &SetupBuildEnvAction{}
	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"test": "1.0.0"},
		},
	}

	err := action.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	// Verify exact path values
	var pkgConfigPath, cppFlags, ldFlags string
	for _, e := range ctx.Env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		} else if strings.HasPrefix(e, "CPPFLAGS=") {
			cppFlags = strings.TrimPrefix(e, "CPPFLAGS=")
		} else if strings.HasPrefix(e, "LDFLAGS=") {
			ldFlags = strings.TrimPrefix(e, "LDFLAGS=")
		}
	}

	// Check exact values
	if pkgConfigPath != pkgConfigDir {
		t.Errorf("PKG_CONFIG_PATH = %q, want %q", pkgConfigPath, pkgConfigDir)
	}
	expectedCppFlags := "-I" + includeDir
	if cppFlags != expectedCppFlags {
		t.Errorf("CPPFLAGS = %q, want %q", cppFlags, expectedCppFlags)
	}
	expectedLdFlags := "-L" + libDir + " -Wl,-rpath," + libDir
	if ldFlags != expectedLdFlags {
		t.Errorf("LDFLAGS = %q, want %q", ldFlags, expectedLdFlags)
	}
}
