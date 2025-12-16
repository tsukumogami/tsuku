package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGemExecAction_Name(t *testing.T) {
	a := &GemExecAction{}
	if got := a.Name(); got != "gem_exec" {
		t.Errorf("Name() = %v, want gem_exec", got)
	}
}

func TestGemExecAction_RequiresSourceDir(t *testing.T) {
	a := &GemExecAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
	}

	// Missing source_dir
	err := a.Execute(ctx, map[string]interface{}{
		"command": "install",
	})
	if err == nil || err.Error() != "gem_exec requires 'source_dir' parameter" {
		t.Errorf("Expected source_dir required error, got: %v", err)
	}
}

func TestGemExecAction_RequiresCommand(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a Gemfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Missing command
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
	})
	if err == nil || err.Error() != "gem_exec requires 'command' parameter" {
		t.Errorf("Expected command required error, got: %v", err)
	}
}

func TestGemExecAction_ValidatesGemfileExists(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// No Gemfile in source_dir
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "install",
	})
	if err == nil {
		t.Error("Expected error for missing Gemfile")
	}
}

func TestGemExecAction_ValidatesLockfileWhenRequired(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a Gemfile but no lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// use_lockfile defaults to true, should fail without Gemfile.lock
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "install",
	})
	if err == nil {
		t.Error("Expected error for missing Gemfile.lock when use_lockfile is true")
	}

	// With use_lockfile=false, should proceed (will fail later at bundler execution)
	err = a.Execute(ctx, map[string]interface{}{
		"source_dir":   workDir,
		"command":      "install",
		"use_lockfile": false,
	})
	// Will fail because bundler isn't available in test, but should pass validation
	if err != nil && err.Error() == "Gemfile.lock not found but use_lockfile is true" {
		t.Error("Should not check lockfile when use_lockfile is false")
	}
}

func TestGemExecAction_RejectsShellMetacharacters(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Test shell metacharacters are rejected
	badCommands := []string{
		"install; rm -rf /",
		"install | cat /etc/passwd",
		"install && whoami",
		"install $HOME",
		"install `whoami`",
	}

	for _, cmd := range badCommands {
		err := a.Execute(ctx, map[string]interface{}{
			"source_dir": workDir,
			"command":    cmd,
		})
		if err == nil || err.Error() != "invalid command: contains shell metacharacters" {
			t.Errorf("Expected shell metacharacter error for command %q, got: %v", cmd, err)
		}
	}
}

func TestGemExecAction_BuildEnvironment(t *testing.T) {
	a := &GemExecAction{}

	sourceDir := "/tmp/src"
	outputDir := "/tmp/out"

	// Test with lockfile enforcement
	env := a.buildEnvironment(sourceDir, outputDir, true, nil)

	hasGemfile := false
	hasFrozen := false
	hasGemHome := false
	hasSourceDateEpoch := false

	for _, e := range env {
		switch {
		case e == "BUNDLE_GEMFILE=/tmp/src/Gemfile":
			hasGemfile = true
		case e == "BUNDLE_FROZEN=true":
			hasFrozen = true
		case e == "GEM_HOME=/tmp/out":
			hasGemHome = true
		case e == "SOURCE_DATE_EPOCH=315619200":
			hasSourceDateEpoch = true
		}
	}

	if !hasGemfile {
		t.Error("Missing BUNDLE_GEMFILE in environment")
	}
	if !hasFrozen {
		t.Error("Missing BUNDLE_FROZEN=true when use_lockfile is true")
	}
	if !hasGemHome {
		t.Error("Missing GEM_HOME in environment")
	}
	if !hasSourceDateEpoch {
		t.Error("Missing SOURCE_DATE_EPOCH for reproducible builds")
	}

	// Test without lockfile enforcement
	envNoLock := a.buildEnvironment(sourceDir, outputDir, false, nil)
	for _, e := range envNoLock {
		if e == "BUNDLE_FROZEN=true" {
			t.Error("BUNDLE_FROZEN should not be set when use_lockfile is false")
		}
	}
}

func TestGemExecAction_BuildEnvironmentWithCustomVars(t *testing.T) {
	a := &GemExecAction{}

	customEnv := map[string]string{
		"CC":     "gcc-12",
		"CFLAGS": "-O2",
	}

	env := a.buildEnvironment("/tmp/src", "/tmp/out", true, customEnv)

	hasCC := false
	hasCFLAGS := false

	for _, e := range env {
		switch {
		case e == "CC=gcc-12":
			hasCC = true
		case e == "CFLAGS=-O2":
			hasCFLAGS = true
		}
	}

	if !hasCC {
		t.Error("Missing custom CC environment variable")
	}
	if !hasCFLAGS {
		t.Error("Missing custom CFLAGS environment variable")
	}
}

func TestGemExecIsPrimitive(t *testing.T) {
	if !IsPrimitive("gem_exec") {
		t.Error("gem_exec should be registered as a primitive")
	}
}

func TestGemExecAction_FindBundler_InToolsDir(t *testing.T) {
	a := &GemExecAction{}
	tmpDir := t.TempDir()

	// Create mock ruby installation with bundle
	rubyDir := filepath.Join(tmpDir, "tools", "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(rubyDir, "bundle")
	if err := os.WriteFile(bundlePath, []byte("#!/bin/sh\necho 'mock bundle'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		ToolsDir: filepath.Join(tmpDir, "tools"),
	}

	found := a.findBundler(ctx)
	if found != bundlePath {
		t.Errorf("findBundler() = %q, want %q", found, bundlePath)
	}
}

func TestGemExecAction_FindBundler_NotFound(t *testing.T) {
	a := &GemExecAction{}
	tmpDir := t.TempDir()

	// Create tools dir but no ruby installation
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH to ensure bundle isn't found
	t.Setenv("PATH", tmpDir)

	ctx := &ExecutionContext{
		ToolsDir: toolsDir,
	}

	found := a.findBundler(ctx)
	if found != "" {
		t.Errorf("findBundler() should return empty string when bundler not found, got %q", found)
	}
}

func TestGemExecAction_RelativeSourceDir(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()
	srcDir := filepath.Join(workDir, "src")

	// Create source directory with Gemfile
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	gemfilePath := filepath.Join(srcDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(srcDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Using relative source_dir should work (will fail at bundler execution)
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": "src", // relative path
		"command":    "install",
	})
	// Should fail because bundler not found, not because of path issues
	if err != nil && err.Error() == "Gemfile not found in source_dir: "+srcDir {
		t.Error("Relative source_dir should be expanded correctly")
	}
}

func TestGemExecAction_RelativeOutputDir(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Using relative output_dir should work (will fail at bundler execution)
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"output_dir": "output", // relative path
		"command":    "install",
	})
	// Should proceed past path validation
	if err != nil && (err.Error() == "gem_exec requires 'source_dir' parameter" ||
		err.Error() == "Gemfile not found in source_dir: "+workDir) {
		t.Error("Relative output_dir should be expanded correctly")
	}
}

func TestGemExecAction_ExecuteWithMockBundler(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock bundler
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(workDir, "vendor", "bundle", "bin")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock bundle that succeeds
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "install",
	})
	if err != nil {
		t.Errorf("Execute() with mock bundler should succeed, got: %v", err)
	}
}

func TestGemExecAction_ExecuteWithExecutablesVerification(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()
	outputDir := filepath.Join(workDir, "output")

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock bundler
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create the executable that should be verified
	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	exePath := filepath.Join(binDir, "my-tool")
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho 'hello'\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock bundle that succeeds
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_dir":  workDir,
		"output_dir":  outputDir,
		"command":     "install",
		"executables": []interface{}{"my-tool"},
	})
	if err != nil {
		t.Errorf("Execute() with executable verification should succeed, got: %v", err)
	}
}

func TestGemExecAction_ExecutableVerificationFails(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()
	outputDir := filepath.Join(workDir, "output")

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock bundler
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create bin dir but NOT the executable
	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock bundle that succeeds
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_dir":  workDir,
		"output_dir":  outputDir,
		"command":     "install",
		"executables": []interface{}{"missing-tool"},
	})
	if err == nil {
		t.Error("Execute() should fail when executable not found")
	}
	if err != nil && !containsStr(err.Error(), "expected executable") {
		t.Errorf("Error should mention missing executable, got: %v", err)
	}
}

func TestGemExecAction_BundlerExecutionFails(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock bundler that fails
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := "#!/bin/sh\necho 'error: gem not found' >&2\nexit 1\n"
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "install",
	})
	if err == nil {
		t.Error("Execute() should fail when bundler fails")
	}
	if err != nil && !containsStr(err.Error(), "bundle install failed") {
		t.Errorf("Error should mention bundle failure, got: %v", err)
	}
}

func TestGemExecAction_NonInstallCommand(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock bundler
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	// Test with non-install command (exec)
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "exec rake build",
	})
	if err != nil {
		t.Errorf("Execute() with 'exec' command should succeed, got: %v", err)
	}
}

func TestGemExecAction_BundlerNotFound(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override PATH to ensure bundler isn't found
	t.Setenv("PATH", workDir)

	// Empty tools dir - no ruby installation
	toolsDir := filepath.Join(workDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_dir": workDir,
		"command":    "install",
	})
	if err == nil {
		t.Error("Execute() should fail when bundler not found")
	}
	if err != nil && !containsStr(err.Error(), "bundler not found") {
		t.Errorf("Error should mention bundler not found, got: %v", err)
	}
}

func TestGemExecAction_WithRubyVersionValidation(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Test with ruby_version that will fail validation (ruby not in PATH in test env)
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir":   workDir,
		"command":      "install",
		"ruby_version": "3.2.0",
	})
	// Should fail at ruby version validation
	if err == nil {
		t.Error("Execute() should fail when ruby version validation fails")
	}
	if err != nil && !containsStr(err.Error(), "ruby version validation failed") {
		// It might also fail because bundler not found, which is fine
		if !containsStr(err.Error(), "bundler not found") {
			t.Errorf("Error should mention ruby version validation or bundler not found, got: %v", err)
		}
	}
}

func TestGemExecAction_WithBundlerVersionValidation(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir: workDir,
	}

	// Test with bundler_version that will fail validation (bundle not in PATH)
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir":      workDir,
		"command":         "install",
		"bundler_version": "2.4.0",
	})
	// Should fail at bundler version validation
	if err == nil {
		t.Error("Execute() should fail when bundler version validation fails")
	}
	if err != nil && !containsStr(err.Error(), "bundler version validation failed") {
		// It might also fail because bundler not found, which is fine
		if !containsStr(err.Error(), "bundler not found") {
			t.Errorf("Error should mention bundler version validation or bundler not found, got: %v", err)
		}
	}
}

func TestGemExecAction_BuildEnvironmentEmptyOutputDir(t *testing.T) {
	a := &GemExecAction{}

	// Test with empty output_dir
	env := a.buildEnvironment("/tmp/src", "", true, nil)

	// GEM_HOME should not be set when outputDir is empty
	for _, e := range env {
		if e == "GEM_HOME=" {
			t.Error("GEM_HOME should not be set to empty string")
		}
	}
}

func TestGemExecAction_ValidateRubyVersion_WithMock(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a mock ruby executable
	mockScript := "#!/bin/sh\necho 'ruby 3.2.0 (2022-12-25 revision abc123)'\n"
	rubyPath := filepath.Join(workDir, "ruby")
	if err := os.WriteFile(rubyPath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", workDir+":"+origPath)

	// Test with matching version
	err := a.validateRubyVersion("3.2")
	if err != nil {
		t.Errorf("validateRubyVersion should pass for matching version, got: %v", err)
	}

	// Test with non-matching version
	err = a.validateRubyVersion("3.1")
	if err == nil {
		t.Error("validateRubyVersion should fail for non-matching version")
	}
}

func TestGemExecAction_ValidateBundlerVersion_WithMock(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a mock bundle executable
	mockScript := "#!/bin/sh\necho 'Bundler version 2.4.10'\n"
	bundlePath := filepath.Join(workDir, "bundle")
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", workDir+":"+origPath)

	// Test with matching version
	err := a.validateBundlerVersion("2.4")
	if err != nil {
		t.Errorf("validateBundlerVersion should pass for matching version, got: %v", err)
	}

	// Test with non-matching version
	err = a.validateBundlerVersion("2.3")
	if err == nil {
		t.Error("validateBundlerVersion should fail for non-matching version")
	}
}

func TestGemExecAction_ValidateRubyVersion_BadOutput(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a mock ruby executable with unexpected output
	mockScript := "#!/bin/sh\necho 'some unexpected output'\n"
	rubyPath := filepath.Join(workDir, "ruby")
	if err := os.WriteFile(rubyPath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", workDir+":"+origPath)

	// Test with unexpected output format
	err := a.validateRubyVersion("3.2")
	if err == nil {
		t.Error("validateRubyVersion should fail with unexpected output")
	}
	if err != nil && !containsStr(err.Error(), "unexpected ruby") {
		t.Errorf("Error should mention unexpected output, got: %v", err)
	}
}

func TestGemExecAction_ValidateBundlerVersion_BadOutput(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create a mock bundle executable with unexpected output
	mockScript := "#!/bin/sh\necho 'some unexpected output'\n"
	bundlePath := filepath.Join(workDir, "bundle")
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", workDir+":"+origPath)

	// Test with unexpected output format
	err := a.validateBundlerVersion("2.4")
	if err == nil {
		t.Error("validateBundlerVersion should fail with unexpected output")
	}
	if err != nil && !containsStr(err.Error(), "unexpected bundle") {
		t.Errorf("Error should mention unexpected output, got: %v", err)
	}
}

func TestGemExecAction_ExecuteWithAllParameters(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create Gemfile and lockfile
	gemfilePath := filepath.Join(workDir, "Gemfile")
	if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock ruby and bundle executables
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Mock bundle
	bundlePath := filepath.Join(rubyDir, "bundle")
	bundleScript := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo 'Bundler version 2.4.10'\nelse\n  exit 0\nfi\n"
	if err := os.WriteFile(bundlePath, []byte(bundleScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Mock ruby (in PATH)
	rubyScript := "#!/bin/sh\necho 'ruby 3.2.0 (2022-12-25 revision abc123)'\n"
	rubyPath := filepath.Join(rubyDir, "ruby")
	if err := os.WriteFile(rubyPath, []byte(rubyScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", rubyDir+":"+origPath)

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		ToolsDir: toolsDir,
	}

	// Test with all parameters
	err := a.Execute(ctx, map[string]interface{}{
		"source_dir":       workDir,
		"command":          "install",
		"ruby_version":     "3.2",
		"bundler_version":  "2.4",
		"environment_vars": map[string]interface{}{"CC": "gcc"},
	})
	if err != nil {
		t.Errorf("Execute() with all params should succeed, got: %v", err)
	}
}

// Note: containsStr helper is defined in go_install_test.go

func TestGemExecAction_LockDataMode_Validation(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: workDir,
	}

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError string
	}{
		{
			name: "missing gem parameter",
			params: map[string]interface{}{
				"lock_data":   "GEM\n  specs:\n",
				"version":     "1.0.0",
				"executables": []interface{}{"bundle"},
			},
			expectError: "requires 'gem' parameter",
		},
		{
			name: "invalid gem name",
			params: map[string]interface{}{
				"gem":         "invalid;gem",
				"lock_data":   "GEM\n  specs:\n",
				"version":     "1.0.0",
				"executables": []interface{}{"bundle"},
			},
			expectError: "invalid gem name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"gem":       "bundler",
				"lock_data": "GEM\n  specs:\n",
				"version":   "1.0.0",
			},
			expectError: "requires 'executables' parameter",
		},
		{
			name: "invalid executable with path",
			params: map[string]interface{}{
				"gem":         "bundler",
				"lock_data":   "GEM\n  specs:\n",
				"version":     "1.0.0",
				"executables": []interface{}{"../bin/exe"},
			},
			expectError: "must not contain path separators",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"gem":         "bundler",
				"lock_data":   "GEM\n  specs:\n",
				"version":     ";echo hack",
				"executables": []interface{}{"bundle"},
			},
			expectError: "invalid gem version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !containsStr(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestGemExecAction_LockDataMode_BundlerNotFound(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Override PATH to ensure bundler isn't found
	t.Setenv("PATH", workDir)

	// Empty tools dir - no ruby installation
	toolsDir := filepath.Join(workDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: workDir,
		ToolsDir:   toolsDir,
		Version:    "2.4.0",
	}

	err := a.Execute(ctx, map[string]interface{}{
		"gem":         "bundler",
		"lock_data":   "GEM\n  specs:\n    bundler (2.4.0)\n",
		"version":     "2.4.0",
		"executables": []interface{}{"bundle"},
	})
	if err == nil {
		t.Error("Execute() should fail when bundler not found")
	}
	if err != nil && !containsStr(err.Error(), "bundler not found") {
		t.Errorf("Error should mention bundler not found, got: %v", err)
	}
}

func TestGemExecAction_LockDataMode_WithMockBundler(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create mock bundler and ruby
	toolsDir := filepath.Join(workDir, "tools")
	rubyDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock bundle that succeeds and creates expected files
	bundlePath := filepath.Join(rubyDir, "bundle")
	mockScript := `#!/bin/sh
# Create bin directory with executable
mkdir -p "$GEM_HOME/bin"
echo '#!/bin/sh' > "$GEM_HOME/bin/bundle"
chmod +x "$GEM_HOME/bin/bundle"
exit 0
`
	if err := os.WriteFile(bundlePath, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: workDir,
		ToolsDir:   toolsDir,
		Version:    "2.4.0",
	}

	lockData := `GEM
  remote: https://rubygems.org/
  specs:
    bundler (2.4.0)

PLATFORMS
  ruby

DEPENDENCIES
  bundler (= 2.4.0)
`

	err := a.Execute(ctx, map[string]interface{}{
		"gem":         "bundler",
		"lock_data":   lockData,
		"version":     "2.4.0",
		"executables": []interface{}{"bundle"},
	})
	if err != nil {
		t.Errorf("Execute() with mock bundler should succeed, got: %v", err)
	}

	// Verify Gemfile was created
	gemfilePath := filepath.Join(workDir, "Gemfile")
	content, err := os.ReadFile(gemfilePath)
	if err != nil {
		t.Errorf("Gemfile should be created: %v", err)
	}
	if !containsStr(string(content), "gem 'bundler'") {
		t.Error("Gemfile should contain gem specification")
	}

	// Verify Gemfile.lock was created
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	content, err = os.ReadFile(lockPath)
	if err != nil {
		t.Errorf("Gemfile.lock should be created: %v", err)
	}
	if !containsStr(string(content), "bundler (2.4.0)") {
		t.Error("Gemfile.lock should contain lock data")
	}
}

func TestGemExecAction_IsDeterministic(t *testing.T) {
	a := &GemExecAction{}
	if a.IsDeterministic() {
		t.Error("gem_exec should not be deterministic (has residual non-determinism)")
	}
}

func TestGemExecAction_RequiresNetwork(t *testing.T) {
	// Check that gem_exec implements NetworkValidator
	var action Action = &GemExecAction{}
	networkValidator, ok := action.(interface{ RequiresNetwork() bool })
	if !ok {
		t.Fatal("gem_exec should implement NetworkValidator")
	}
	if !networkValidator.RequiresNetwork() {
		t.Error("gem_exec should require network")
	}
}

func TestCountLockfileGems_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		lockData string
		expected int
	}{
		{
			name:     "no specs section",
			lockData: "GEM\n  remote: https://rubygems.org/\n",
			expected: 0,
		},
		{
			name:     "empty specs",
			lockData: "GEM\n  specs:\nPLATFORMS\n",
			expected: 0,
		},
		{
			name: "with checksums section",
			lockData: `GEM
  remote: https://rubygems.org/
  specs:
    bundler (2.4.0)

PLATFORMS
  ruby

CHECKSUMS
  bundler (2.4.0) sha256:abc123
`,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countLockfileGems(tt.lockData)
			if result != tt.expected {
				t.Errorf("countLockfileGems() = %d, want %d", result, tt.expected)
			}
		})
	}
}
