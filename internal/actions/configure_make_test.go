package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestConfigureMakeAction_Name(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	if action.Name() != "configure_make" {
		t.Errorf("Name() = %q, want %q", action.Name(), "configure_make")
	}
}

func TestConfigureMakeAction_Execute_MissingSourceDir(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"executables": []string{"test"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing source_dir")
	}
	if err.Error() != "configure_make action requires 'source_dir' parameter" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfigureMakeAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"source_dir": sourceDir,
	}

	// executables is optional; the action should not fail due to missing executables.
	// It will fail later at the configure step (the fake script doesn't do real work),
	// but the error must not be about missing executables.
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "executables") {
		t.Errorf("Should not fail due to missing executables, got: %v", err)
	}
}

func TestConfigureMakeAction_Execute_ConfigureNotFound(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create empty source dir (no configure script)
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"source_dir":  sourceDir,
		"executables": []string{"test"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing configure script")
	}
	if !strings.Contains(err.Error(), "configure script not found") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfigureMakeAction_Execute_InvalidExecutableName(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	testCases := []string{
		"../evil",
		"/absolute/path",
		"with/slash",
		"..",
		".",
		"",
	}

	for _, tc := range testCases {
		params := map[string]interface{}{
			"source_dir":  sourceDir,
			"executables": []string{tc},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid executable name %q", tc)
		}
	}
}

func TestConfigureMakeAction_Execute_InvalidConfigureArg(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	invalidArgs := []string{
		"--opt;rm -rf /",
		"--opt && evil",
		"--opt | cat /etc/passwd",
		"--opt `id`",
		"$(whoami)",
	}

	for _, arg := range invalidArgs {
		params := map[string]interface{}{
			"source_dir":     sourceDir,
			"executables":    []string{"test"},
			"configure_args": []string{arg},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid configure arg %q", arg)
		}
		if !strings.Contains(err.Error(), "invalid configure argument") {
			t.Errorf("Unexpected error for %q: %v", arg, err)
		}
	}
}

func TestConfigureMakeAction_Execute_RelativeSourceDir(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	// Use relative path
	params := map[string]interface{}{
		"source_dir":  "src",
		"executables": []string{"test"},
	}

	// This will fail at configure step (fake script), but proves relative path resolved
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "configure script not found") {
		t.Error("Relative source_dir should have been resolved")
	}
}

func TestConfigureMakeAction_Registered(t *testing.T) {
	t.Parallel()
	// Verify configure_make is registered as a primitive action
	if !IsPrimitive("configure_make") {
		t.Error("configure_make should be registered as a primitive action")
	}

	// Verify it's in the action registry
	action := Get("configure_make")
	if action == nil {
		t.Error("configure_make should be registered in the action registry")
	}
}

func TestConfigureMakeAction_NotDeterministic(t *testing.T) {
	t.Parallel()
	// configure_make uses system compiler, so it's not deterministic
	if IsDeterministic("configure_make") {
		t.Error("configure_make should not be deterministic")
	}
}

func TestIsValidConfigureArg(t *testing.T) {
	t.Parallel()
	validArgs := []string{
		"--prefix=/usr/local",
		"--enable-shared",
		"--disable-static",
		"--with-ssl",
		"--without-debug",
		"CFLAGS=-O2",
		"--host=x86_64-linux-gnu",
	}

	for _, arg := range validArgs {
		if !isValidConfigureArg(arg) {
			t.Errorf("isValidConfigureArg(%q) = false, want true", arg)
		}
	}

	invalidArgs := []string{
		"",                        // empty
		"--opt;rm",                // shell metachar
		"--opt && echo",           // shell metachar
		"--opt | cat",             // shell metachar
		"--opt `id`",              // shell metachar
		"$(whoami)",               // shell metachar
		"--opt\necho",             // newline
		string(make([]byte, 501)), // too long
	}

	for _, arg := range invalidArgs {
		if isValidConfigureArg(arg) {
			if len(arg) <= 20 {
				t.Errorf("isValidConfigureArg(%q) = true, want false", arg)
			} else {
				t.Errorf("isValidConfigureArg(len=%d) = true, want false", len(arg))
			}
		}
	}
}

func TestBuildAutotoolsEnv_NoDependencies(t *testing.T) {
	t.Parallel()
	ctx := &ExecutionContext{
		Context:      context.Background(),
		ToolsDir:     t.TempDir(),
		Dependencies: ResolvedDeps{InstallTime: make(map[string]string)},
	}

	env := buildAutotoolsEnv(ctx)

	// Verify SOURCE_DATE_EPOCH is set
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("SOURCE_DATE_EPOCH not set in environment")
	}

	// Verify no PKG_CONFIG_PATH, CPPFLAGS, or LDFLAGS when no dependencies
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			t.Error("PKG_CONFIG_PATH should not be set with no dependencies")
		}
		if strings.HasPrefix(e, "CPPFLAGS=") {
			t.Error("CPPFLAGS should not be set with no dependencies")
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			t.Error("LDFLAGS should not be set with no dependencies")
		}
	}
}

func TestBuildAutotoolsEnv_WithDependencies(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directories
	dep1Dir := filepath.Join(toolsDir, "zlib-1.2.11")
	dep2Dir := filepath.Join(toolsDir, "openssl-3.0.0")

	// Create directory structure for dep1 (has all: include, lib, lib/pkgconfig)
	if err := os.MkdirAll(filepath.Join(dep1Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep1Dir, "lib", "pkgconfig"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create directory structure for dep2 (only has include and lib, no pkgconfig)
	if err := os.MkdirAll(filepath.Join(dep2Dir, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dep2Dir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}

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

	env := buildAutotoolsEnv(ctx)

	// Verify PKG_CONFIG_PATH contains only zlib (openssl doesn't have pkgconfig)
	var pkgConfigPath string
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
			break
		}
	}
	if pkgConfigPath == "" {
		t.Error("PKG_CONFIG_PATH not set with dependencies")
	}
	zlibPkgConfig := filepath.Join(dep1Dir, "lib", "pkgconfig")
	if !strings.Contains(pkgConfigPath, zlibPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH missing zlib: got %q, want to contain %q", pkgConfigPath, zlibPkgConfig)
	}

	// Verify CPPFLAGS contains both dependencies
	var cppFlags string
	for _, e := range env {
		if strings.HasPrefix(e, "CPPFLAGS=") {
			cppFlags = strings.TrimPrefix(e, "CPPFLAGS=")
			break
		}
	}
	if cppFlags == "" {
		t.Error("CPPFLAGS not set with dependencies")
	}
	zlibInclude := filepath.Join(dep1Dir, "include")
	opensslInclude := filepath.Join(dep2Dir, "include")
	if !strings.Contains(cppFlags, "-I"+zlibInclude) {
		t.Errorf("CPPFLAGS missing zlib: got %q, want to contain -I%s", cppFlags, zlibInclude)
	}
	if !strings.Contains(cppFlags, "-I"+opensslInclude) {
		t.Errorf("CPPFLAGS missing openssl: got %q, want to contain -I%s", cppFlags, opensslInclude)
	}

	// Verify LDFLAGS contains both dependencies
	var ldFlags string
	for _, e := range env {
		if strings.HasPrefix(e, "LDFLAGS=") {
			ldFlags = strings.TrimPrefix(e, "LDFLAGS=")
			break
		}
	}
	if ldFlags == "" {
		t.Error("LDFLAGS not set with dependencies")
	}
	zlibLib := filepath.Join(dep1Dir, "lib")
	opensslLib := filepath.Join(dep2Dir, "lib")
	if !strings.Contains(ldFlags, "-L"+zlibLib) {
		t.Errorf("LDFLAGS missing zlib: got %q, want to contain -L%s", ldFlags, zlibLib)
	}
	if !strings.Contains(ldFlags, "-L"+opensslLib) {
		t.Errorf("LDFLAGS missing openssl: got %q, want to contain -L%s", ldFlags, opensslLib)
	}
}

func TestBuildAutotoolsEnv_MissingDirectories(t *testing.T) {
	t.Parallel()
	toolsDir := t.TempDir()

	// Create mock dependency directory without standard subdirectories
	depDir := filepath.Join(toolsDir, "custom-1.0.0")
	if err := os.MkdirAll(depDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"custom": "1.0.0"},
		},
	}

	env := buildAutotoolsEnv(ctx)

	// Verify environment variables are not set when directories don't exist
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			t.Error("PKG_CONFIG_PATH should not be set when lib/pkgconfig doesn't exist")
		}
		if strings.HasPrefix(e, "CPPFLAGS=") {
			t.Error("CPPFLAGS should not be set when include doesn't exist")
		}
		if strings.HasPrefix(e, "LDFLAGS=") {
			t.Error("LDFLAGS should not be set when lib doesn't exist")
		}
	}
}

// -- configure_make.go: buildAutotoolsEnv with dependencies --

func TestBuildAutotoolsEnv_WithDeps(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create mock dependency directories
	toolsDir := filepath.Join(tmpDir, "tools")
	libsDir := filepath.Join(tmpDir, "libs")

	// Create a dependency with bin, lib, include, and pkgconfig
	depDir := filepath.Join(libsDir, "openssl-3.0.0")
	for _, sub := range []string{"bin", "lib/pkgconfig", "include"} {
		if err := os.MkdirAll(filepath.Join(depDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ToolsDir:   toolsDir,
		LibsDir:    libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	env := buildAutotoolsEnv(ctx)

	// Should contain PKG_CONFIG_PATH
	hasPkgConfig := false
	hasCppFlags := false
	hasLdFlags := false
	hasSourceDateEpoch := false
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			hasPkgConfig = true
		}
		if strings.HasPrefix(e, "CPPFLAGS=") && strings.Contains(e, "openssl") {
			hasCppFlags = true
		}
		if strings.HasPrefix(e, "LDFLAGS=") && strings.Contains(e, "openssl") {
			hasLdFlags = true
		}
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSourceDateEpoch = true
		}
	}
	if !hasPkgConfig {
		t.Error("Expected PKG_CONFIG_PATH in env")
	}
	if !hasCppFlags {
		t.Error("Expected CPPFLAGS with openssl path")
	}
	if !hasLdFlags {
		t.Error("Expected LDFLAGS with openssl path")
	}
	if !hasSourceDateEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0")
	}
}

func TestBuildAutotoolsEnv_NoDeps(t *testing.T) {
	t.Parallel()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	env := buildAutotoolsEnv(ctx)

	// Should still contain SOURCE_DATE_EPOCH
	hasSDEpoch := false
	for _, e := range env {
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSDEpoch = true
		}
	}
	if !hasSDEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0 even without deps")
	}
}

func TestBuildAutotoolsEnv_WithExecPaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "mybin")
	if err := os.MkdirAll(execPath, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ExecPaths:  []string{execPath},
	}
	env := buildAutotoolsEnv(ctx)

	hasExecInPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") && strings.Contains(e, execPath) {
			hasExecInPath = true
		}
	}
	if !hasExecInPath {
		t.Error("Expected ExecPaths in PATH env var")
	}
}

// -- configure_make.go: ConfigureMakeAction Dependencies --

func TestConfigureMakeAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := ConfigureMakeAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 3 {
		t.Errorf("Dependencies().InstallTime has %d entries, want 3", len(deps.InstallTime))
	}
	expected := map[string]bool{"make": true, "zig": true, "pkg-config": true}
	for _, dep := range deps.InstallTime {
		if !expected[dep] {
			t.Errorf("Unexpected dependency: %s", dep)
		}
	}
}

// -- configure_make.go: touchAutogeneratedFiles --

func TestTouchAutogeneratedFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create some autotools-style files
	files := []string{"configure.ac", "Makefile.in", "aclocal.m4", "configure"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Should not panic
	touchAutogeneratedFiles(tmpDir)

	// Verify files still exist
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("File %s disappeared after touchAutogeneratedFiles", f)
		}
	}
}

// -- configure_make.go: buildAutotoolsEnv with libcurl dependency --

func TestBuildAutotoolsEnv_WithLibcurl(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	libsDir := filepath.Join(tmpDir, "libs")

	// Create libcurl dependency with bin/curl-config
	depDir := filepath.Join(libsDir, "libcurl-8.0.0")
	if err := os.MkdirAll(filepath.Join(depDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "bin", "curl-config"), []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ToolsDir:   filepath.Join(tmpDir, "tools"),
		LibsDir:    libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"libcurl": "8.0.0"},
		},
	}

	env := buildAutotoolsEnv(ctx)

	hasCurlConfig := false
	hasCurlDir := false
	hasNoCurl := false
	for _, e := range env {
		if strings.HasPrefix(e, "CURL_CONFIG=") {
			hasCurlConfig = true
		}
		if strings.HasPrefix(e, "CURLDIR=") {
			hasCurlDir = true
		}
		if e == "NO_CURL=" {
			hasNoCurl = true
		}
	}
	if !hasCurlConfig {
		t.Error("Expected CURL_CONFIG in env")
	}
	if !hasCurlDir {
		t.Error("Expected CURLDIR in env")
	}
	if !hasNoCurl {
		t.Error("Expected NO_CURL= in env")
	}
}

// -- configure_make.go: findMake --

func TestFindMake(t *testing.T) {
	t.Parallel()
	result := findMake()
	if result == "" {
		t.Error("findMake() returned empty string")
	}
}

// -- configure_make.go: touchAutogeneratedFiles with full autotools file set --

func TestTouchAutogeneratedFiles_FullSet(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	autoFiles := []string{
		"configure",
		"configure.ac",
		"Makefile.in",
		"Makefile.am",
		"aclocal.m4",
		"config.h.in",
		"ltmain.sh",
	}
	for _, f := range autoFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	touchAutogeneratedFiles(tmpDir)

	// Verify files still exist
	for _, f := range autoFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("File %s disappeared after touchAutogeneratedFiles", f)
		}
	}
}

// -- configure_make.go: Execute missing configure script --

func TestConfigureMakeAction_Execute_MissingConfigureScript(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}
	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
	})
	if err == nil {
		t.Error("Expected error for missing configure script")
	}
}

// Tests to close coverage gaps in various files.

// -- configure_make.go: Preflight --

func TestConfigureMakeAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &ConfigureMakeAction{}

	tests := []struct {
		name         string
		params       map[string]any
		wantErrors   int
		wantWarnings int
	}{
		{
			name:       "valid params",
			params:     map[string]any{"source_dir": "src"},
			wantErrors: 0,
		},
		{
			name:       "missing source_dir",
			params:     map[string]any{},
			wantErrors: 1,
		},
		{
			name: "skip_configure without make_args",
			params: map[string]any{
				"source_dir":     "src",
				"skip_configure": true,
			},
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name: "skip_configure with make_args",
			params: map[string]any{
				"source_dir":     "src",
				"skip_configure": true,
				"make_args":      []any{"install"},
			},
			wantErrors:   0,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantWarnings > 0 && len(result.Warnings) != tt.wantWarnings {
				t.Errorf("Preflight() warnings = %v, want %d", result.Warnings, tt.wantWarnings)
			}
		})
	}
}
