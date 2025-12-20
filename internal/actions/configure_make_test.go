package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing executables")
	}
	if err.Error() != "configure_make action requires 'executables' parameter with at least one executable" {
		t.Errorf("Unexpected error: %v", err)
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
