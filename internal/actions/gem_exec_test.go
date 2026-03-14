package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGemExecAction_Name(t *testing.T) {
	a := &GemExecAction{}
	if got := a.Name(); got != "gem_exec" {
		t.Errorf("Name() = %v, want gem_exec", got)
	}
}

func TestGemExecAction_ParamValidation(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]interface{}
		setupFiles  map[string]string
		errContains string
	}{
		{
			name: "missing source_dir",
			params: map[string]interface{}{
				"command": "install",
			},
			errContains: "gem_exec requires 'source_dir' parameter",
		},
		{
			name: "missing command",
			params: map[string]interface{}{
				"source_dir": "WORKDIR",
			},
			setupFiles: map[string]string{
				"Gemfile": "source 'https://rubygems.org'\n",
			},
			errContains: "gem_exec requires 'command' parameter",
		},
		{
			name: "missing Gemfile",
			params: map[string]interface{}{
				"source_dir": "WORKDIR",
				"command":    "install",
			},
			errContains: "Gemfile not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &GemExecAction{}
			workDir := t.TempDir()

			for name, content := range tt.setupFiles {
				if err := os.WriteFile(filepath.Join(workDir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Replace WORKDIR sentinel with actual temp dir
			params := make(map[string]interface{})
			for k, v := range tt.params {
				if s, ok := v.(string); ok && s == "WORKDIR" {
					params[k] = workDir
				} else {
					params[k] = v
				}
			}

			ctx := &ExecutionContext{
				Context: context.Background(),
				WorkDir: workDir,
			}

			err := a.Execute(ctx, params)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errContains)
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
			}
		})
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
		Context: context.Background(),
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
		Context: context.Background(),
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
	tests := []struct {
		name          string
		sourceDir     string
		outputDir     string
		useLockfile   bool
		customEnv     map[string]string
		wantEnvVars   []string
		rejectEnvVars []string
	}{
		{
			name:        "with lockfile enforcement",
			sourceDir:   "/tmp/src",
			outputDir:   "/tmp/out",
			useLockfile: true,
			wantEnvVars: []string{
				"BUNDLE_GEMFILE=/tmp/src/Gemfile",
				"BUNDLE_FROZEN=true",
				"GEM_HOME=/tmp/out",
				"SOURCE_DATE_EPOCH=315619200",
			},
		},
		{
			name:          "without lockfile enforcement",
			sourceDir:     "/tmp/src",
			outputDir:     "/tmp/out",
			useLockfile:   false,
			wantEnvVars:   []string{"BUNDLE_GEMFILE=/tmp/src/Gemfile", "GEM_HOME=/tmp/out"},
			rejectEnvVars: []string{"BUNDLE_FROZEN=true"},
		},
		{
			name:        "with custom vars",
			sourceDir:   "/tmp/src",
			outputDir:   "/tmp/out",
			useLockfile: true,
			customEnv: map[string]string{
				"CC":     "gcc-12",
				"CFLAGS": "-O2",
			},
			wantEnvVars: []string{"CC=gcc-12", "CFLAGS=-O2"},
		},
		{
			name:          "empty output dir",
			sourceDir:     "/tmp/src",
			outputDir:     "",
			useLockfile:   true,
			rejectEnvVars: []string{"GEM_HOME="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &GemExecAction{}
			env := a.buildEnvironment(tt.sourceDir, tt.outputDir, tt.useLockfile, tt.customEnv)

			envSet := make(map[string]bool)
			for _, e := range env {
				envSet[e] = true
			}

			for _, want := range tt.wantEnvVars {
				if !envSet[want] {
					t.Errorf("missing expected env var %q", want)
				}
			}
			for _, reject := range tt.rejectEnvVars {
				if envSet[reject] {
					t.Errorf("unexpected env var %q should not be present", reject)
				}
			}
		})
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
		Context: context.Background(),
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
		Context: context.Background(),
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
	if err != nil && !strings.Contains(err.Error(), "expected executable") {
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
	// Accept either "bundle config failed" or "bundle install failed"
	if err != nil && !strings.Contains(err.Error(), "bundle") {
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
	if err != nil && !strings.Contains(err.Error(), "bundler not found") {
		t.Errorf("Error should mention bundler not found, got: %v", err)
	}
}

func TestGemExecAction_WithVersionValidation(t *testing.T) {
	tests := []struct {
		name           string
		params         map[string]interface{}
		errMustContain string
		errAlsoAccepts string
	}{
		{
			name: "ruby version validation fails",
			params: map[string]interface{}{
				"ruby_version": "3.2.0",
			},
			errMustContain: "ruby version validation failed",
			errAlsoAccepts: "bundler not found",
		},
		{
			name: "bundler version validation fails",
			params: map[string]interface{}{
				"bundler_version": "2.4.0",
			},
			errMustContain: "bundler version validation failed",
			errAlsoAccepts: "bundler not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &GemExecAction{}
			workDir := t.TempDir()

			gemfilePath := filepath.Join(workDir, "Gemfile")
			if err := os.WriteFile(gemfilePath, []byte("source 'https://rubygems.org'\n"), 0644); err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(workDir, "Gemfile.lock")
			if err := os.WriteFile(lockPath, []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n"), 0644); err != nil {
				t.Fatal(err)
			}

			ctx := &ExecutionContext{
				Context: context.Background(),
				WorkDir: workDir,
			}

			params := map[string]interface{}{
				"source_dir": workDir,
				"command":    "install",
			}
			for k, v := range tt.params {
				params[k] = v
			}

			err := a.Execute(ctx, params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errMustContain) &&
				!strings.Contains(err.Error(), tt.errAlsoAccepts) {
				t.Errorf("expected error containing %q or %q, got: %v",
					tt.errMustContain, tt.errAlsoAccepts, err)
			}
		})
	}
}

func TestGemExecAction_VersionValidation_WithMock(t *testing.T) {
	tests := []struct {
		name        string
		mockBinary  string
		mockOutput  string
		validate    func(a *GemExecAction) error
		expectErr   bool
		errContains string
	}{
		{
			name:       "ruby matching version",
			mockBinary: "ruby",
			mockOutput: "ruby 3.2.0 (2022-12-25 revision abc123)",
			validate:   func(a *GemExecAction) error { return a.validateRubyVersion("3.2") },
			expectErr:  false,
		},
		{
			name:       "ruby non-matching version",
			mockBinary: "ruby",
			mockOutput: "ruby 3.2.0 (2022-12-25 revision abc123)",
			validate:   func(a *GemExecAction) error { return a.validateRubyVersion("3.1") },
			expectErr:  true,
		},
		{
			name:        "ruby bad output",
			mockBinary:  "ruby",
			mockOutput:  "some unexpected output",
			validate:    func(a *GemExecAction) error { return a.validateRubyVersion("3.2") },
			expectErr:   true,
			errContains: "unexpected ruby",
		},
		{
			name:       "bundler matching version",
			mockBinary: "bundle",
			mockOutput: "Bundler version 2.4.10",
			validate:   func(a *GemExecAction) error { return a.validateBundlerVersion("2.4") },
			expectErr:  false,
		},
		{
			name:       "bundler non-matching version",
			mockBinary: "bundle",
			mockOutput: "Bundler version 2.4.10",
			validate:   func(a *GemExecAction) error { return a.validateBundlerVersion("2.3") },
			expectErr:  true,
		},
		{
			name:        "bundler bad output",
			mockBinary:  "bundle",
			mockOutput:  "some unexpected output",
			validate:    func(a *GemExecAction) error { return a.validateBundlerVersion("2.4") },
			expectErr:   true,
			errContains: "unexpected bundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &GemExecAction{}
			workDir := t.TempDir()

			mockScript := "#!/bin/sh\necho '" + tt.mockOutput + "'\n"
			binPath := filepath.Join(workDir, tt.mockBinary)
			if err := os.WriteFile(binPath, []byte(mockScript), 0755); err != nil {
				t.Fatal(err)
			}

			origPath := os.Getenv("PATH")
			t.Setenv("PATH", workDir+":"+origPath)

			err := tt.validate(a)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.errContains != "" && err != nil && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
			}
		})
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

			if !strings.Contains(err.Error(), tt.expectError) {
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
	if err != nil && !strings.Contains(err.Error(), "bundler not found") {
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
	if !strings.Contains(string(content), "gem 'bundler'") {
		t.Error("Gemfile should contain gem specification")
	}

	// Verify Gemfile.lock was created
	lockPath := filepath.Join(workDir, "Gemfile.lock")
	content, err = os.ReadFile(lockPath)
	if err != nil {
		t.Errorf("Gemfile.lock should be created: %v", err)
	}
	if !strings.Contains(string(content), "bundler (2.4.0)") {
		t.Error("Gemfile.lock should contain lock data")
	}
}

func TestGemExecAction_LockDataMode_CreatesWrapperScripts(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create mock bundler and ruby in tools directory
	toolsDir := filepath.Join(workDir, "tools")
	rubyBinDir := filepath.Join(toolsDir, "ruby-3.2.0", "bin")
	if err := os.MkdirAll(rubyBinDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Mock bundler that creates executables in ruby/<ver>/bin/ (standard bundler --path layout)
	bundlePath := filepath.Join(rubyBinDir, "bundle")
	mockScript := `#!/bin/sh
# Create executables in ruby versioned directory (bundler --path layout)
RUBY_BIN="$GEM_HOME/ruby/3.2.0/bin"
mkdir -p "$RUBY_BIN"
echo '#!/usr/bin/env ruby' > "$RUBY_BIN/mytool"
echo 'puts "hello"' >> "$RUBY_BIN/mytool"
chmod +x "$RUBY_BIN/mytool"
echo '#!/usr/bin/env ruby' > "$RUBY_BIN/mytool-helper"
echo 'puts "helper"' >> "$RUBY_BIN/mytool-helper"
chmod +x "$RUBY_BIN/mytool-helper"
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
		Version:    "1.0.0",
	}

	lockData := `GEM
  remote: https://rubygems.org/
  specs:
    mytool (1.0.0)

PLATFORMS
  ruby

DEPENDENCIES
  mytool (= 1.0.0)
`

	err := a.Execute(ctx, map[string]interface{}{
		"gem":         "mytool",
		"lock_data":   lockData,
		"version":     "1.0.0",
		"executables": []interface{}{"mytool", "mytool-helper"},
	})
	if err != nil {
		t.Fatalf("Execute() should succeed, got: %v", err)
	}

	// Verify wrapper scripts were created (not symlinks)
	for _, exe := range []string{"mytool", "mytool-helper"} {
		wrapperPath := filepath.Join(workDir, "bin", exe)
		info, err := os.Lstat(wrapperPath)
		if err != nil {
			t.Fatalf("wrapper for %s should exist: %v", exe, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a regular file (wrapper script), not a symlink", exe)
		}

		content, err := os.ReadFile(wrapperPath)
		if err != nil {
			t.Fatalf("should read wrapper %s: %v", exe, err)
		}
		wrapperStr := string(content)

		// Verify wrapper contains required environment setup
		if !strings.Contains(wrapperStr, "#!/bin/bash") {
			t.Errorf("wrapper for %s should start with bash shebang", exe)
		}
		if !strings.Contains(wrapperStr, "GEM_HOME=") {
			t.Errorf("wrapper for %s should set GEM_HOME", exe)
		}
		// GEM_HOME must include a subdirectory (bundler's versioned gem path),
		// not bare $INSTALL_DIR which was the pre-fix broken behavior
		if strings.Contains(wrapperStr, `GEM_HOME="$INSTALL_DIR"`) {
			t.Errorf("wrapper for %s sets GEM_HOME to bare INSTALL_DIR; should include bundler's versioned subdirectory", exe)
		}
		if !strings.Contains(wrapperStr, "GEM_PATH=") {
			t.Errorf("wrapper for %s should set GEM_PATH", exe)
		}
		if !strings.Contains(wrapperStr, rubyBinDir) {
			t.Errorf("wrapper for %s should reference ruby bin dir %s", exe, rubyBinDir)
		}
		if !strings.Contains(wrapperStr, exe+".gem") {
			t.Errorf("wrapper for %s should reference %s.gem", exe, exe)
		}

		// Verify .gem file was created
		gemPath := filepath.Join(workDir, "bin", exe+".gem")
		if _, err := os.Stat(gemPath); err != nil {
			t.Errorf(".gem file for %s should exist at %s: %v", exe, gemPath, err)
		}
	}
}

func TestGemExecAction_LockDataMode_RejectsSystemBundler(t *testing.T) {
	a := &GemExecAction{}
	workDir := t.TempDir()

	// Create tools dir but put bundler outside of it (simulating system bundler)
	toolsDir := filepath.Join(workDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}
	systemBinDir := filepath.Join(workDir, "usr", "bin")
	if err := os.MkdirAll(systemBinDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create mock system bundler
	systemBundle := filepath.Join(systemBinDir, "bundle")
	if err := os.WriteFile(systemBundle, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Override PATH so findBundler finds the system bundler
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", systemBinDir+":"+origPath)

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: workDir,
		ToolsDir:   toolsDir,
		Version:    "1.0.0",
	}

	err := a.Execute(ctx, map[string]interface{}{
		"gem":         "mytool",
		"lock_data":   "GEM\n  specs:\n    mytool (1.0.0)\n",
		"version":     "1.0.0",
		"executables": []interface{}{"mytool"},
	})
	if err == nil {
		t.Error("Execute() should fail when only system bundler is available")
	}
	if err != nil && !strings.Contains(err.Error(), "requires tsuku-managed ruby") {
		t.Errorf("error should mention tsuku-managed ruby requirement, got: %v", err)
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

// -- gem_exec.go: extractBundlerVersion --

func TestExtractBundlerVersion_Found(t *testing.T) {
	t.Parallel()
	lockData := `GEM
  remote: https://rubygems.org/
  specs:
    rake (13.0.6)

BUNDLED WITH
   2.4.22
`
	version := extractBundlerVersion(lockData)
	if version != "2.4.22" {
		t.Errorf("extractBundlerVersion() = %q, want %q", version, "2.4.22")
	}
}

func TestExtractBundlerVersion_NotFound(t *testing.T) {
	t.Parallel()
	version := extractBundlerVersion("no bundled with section")
	if version != "" {
		t.Errorf("extractBundlerVersion() = %q, want empty", version)
	}
}

// -- gem_exec.go: Dependencies --

func TestGemExecAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := GemExecAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "ruby" {
		t.Errorf("Dependencies().InstallTime = %v, want [ruby]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "ruby" {
		t.Errorf("Dependencies().Runtime = %v, want [ruby]", deps.Runtime)
	}
}
