package validate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestSourceBuildLimits(t *testing.T) {
	limits := SourceBuildLimits()

	if limits.Memory != "4g" {
		t.Errorf("expected 4g memory, got %s", limits.Memory)
	}
	if limits.CPUs != "4" {
		t.Errorf("expected 4 CPUs, got %s", limits.CPUs)
	}
	if limits.PidsMax != 500 {
		t.Errorf("expected 500 pids, got %d", limits.PidsMax)
	}
	if limits.ReadOnly {
		t.Error("source builds should not be read-only")
	}
	if limits.Timeout != 15*time.Minute {
		t.Errorf("expected 15 minute timeout, got %v", limits.Timeout)
	}
}

func TestSourceBuildValidationImage(t *testing.T) {
	if SourceBuildValidationImage != "ubuntu:22.04" {
		t.Errorf("expected ubuntu:22.04, got %s", SourceBuildValidationImage)
	}
}

func TestDetectRequiredBuildTools_ConfigureMake(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "configure_make"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"autoconf":        true,
		"automake":        true,
		"libtool":         true,
		"pkg-config":      true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_CMake(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "cmake_build"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"cmake":           true,
		"ninja-build":     true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_Cargo(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "cargo_build"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"curl":            true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_Go(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "go_build"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"curl":            true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_Patch(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "apply_patch"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"patch":           true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_CPAN(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "cpan_install"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"perl":            true,
		"cpanminus":       true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestDetectRequiredBuildTools_SkipsDarwinOnly(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			// This step should be skipped (darwin-only)
			{
				Action: "cmake_build",
				When:   map[string]string{"os": "darwin"},
			},
			// This step should be included (linux-only)
			{
				Action: "configure_make",
				When:   map[string]string{"os": "linux"},
			},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	// Should have autotools but not cmake
	hasCMake := false
	hasAutoconf := false
	for _, tool := range tools {
		if tool == "cmake" {
			hasCMake = true
		}
		if tool == "autoconf" {
			hasAutoconf = true
		}
	}

	if hasCMake {
		t.Error("cmake should not be included for darwin-only step")
	}
	if !hasAutoconf {
		t.Error("autoconf should be included for linux step")
	}
}

func TestDetectRequiredBuildTools_MultipleActions(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "apply_patch"},
			{Action: "configure_make"},
		},
	}

	tools := executor.detectRequiredBuildTools(r)

	expected := map[string]bool{
		"build-essential": true,
		"patch":           true,
		"autoconf":        true,
		"automake":        true,
		"libtool":         true,
		"pkg-config":      true,
	}

	for tool := range expected {
		found := false
		for _, t := range tools {
			if t == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %s not found in %v", tool, tools)
		}
	}
}

func TestBuildSourceBuildScript_BasicStructure(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Steps: []recipe.Step{
			{Action: "configure_make"},
		},
	}

	script := executor.buildSourceBuildScript(r)

	// Verify script starts correctly
	if !strings.HasPrefix(script, "#!/bin/bash\n") {
		t.Error("script should start with shebang")
	}

	// Verify set -e for error handling
	if !strings.Contains(script, "set -e") {
		t.Error("script should have set -e for error handling")
	}

	// Verify apt-get update
	if !strings.Contains(script, "apt-get update") {
		t.Error("script should update apt")
	}

	// Verify TSUKU_HOME setup
	if !strings.Contains(script, "mkdir -p /workspace/tsuku") {
		t.Error("script should create TSUKU_HOME")
	}

	// Verify recipe copy
	if !strings.Contains(script, "cp /workspace/recipe.toml /workspace/tsuku/recipes/test-tool.toml") {
		t.Error("script should copy recipe to tsuku recipes directory")
	}

	// Verify tsuku install call
	if !strings.Contains(script, "tsuku install test-tool --force") {
		t.Error("script should call tsuku install")
	}
}

func TestBuildSourceBuildScript_InstallsBuildTools(t *testing.T) {
	detector := NewRuntimeDetector()
	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "cmake-tool",
		},
		Steps: []recipe.Step{
			{Action: "cmake_build"},
		},
	}

	script := executor.buildSourceBuildScript(r)

	// Verify cmake is installed
	if !strings.Contains(script, "cmake") {
		t.Error("script should install cmake for cmake_build action")
	}

	// Verify ninja-build is installed
	if !strings.Contains(script, "ninja-build") {
		t.Error("script should install ninja-build for cmake_build action")
	}
}

func TestValidateSourceBuild_NoRuntime(t *testing.T) {
	// Create detector that returns no runtime
	detector := NewRuntimeDetector()
	detector.lookPath = func(string) (string, error) {
		return "", ErrNoRuntime
	}

	logger := &testLogger{}
	executor := NewExecutor(detector, NewPreDownloader(), WithExecutorLogger(logger))

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test"},
		Verify: recipe.VerifySection{
			Command: "test --version",
		},
	}

	result, err := executor.ValidateSourceBuild(context.Background(), r)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !result.Skipped {
		t.Error("expected validation to be skipped when no runtime available")
	}
}

func TestValidateSourceBuild_Success(t *testing.T) {
	var capturedOpts RunOptions

	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			capturedOpts = opts
			return &RunResult{
				ExitCode: 0,
				Stdout:   "test-tool version 1.0.0",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{Action: "configure_make"},
		},
		Verify: recipe.VerifySection{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	result, err := executor.ValidateSourceBuild(context.Background(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Errorf("expected validation to pass, got stdout: %s, stderr: %s", result.Stdout, result.Stderr)
	}

	// Verify source build specific options
	if capturedOpts.Image != SourceBuildValidationImage {
		t.Errorf("expected image %s, got %s", SourceBuildValidationImage, capturedOpts.Image)
	}

	// Verify network is enabled (needed for dependency downloads)
	if capturedOpts.Network != "host" {
		t.Errorf("expected network=host for source builds, got %s", capturedOpts.Network)
	}

	// Verify longer timeout
	if capturedOpts.Limits.Timeout != 15*time.Minute {
		t.Errorf("expected 15 minute timeout, got %v", capturedOpts.Limits.Timeout)
	}

	// Verify adequate resources
	if capturedOpts.Limits.Memory != "4g" {
		t.Errorf("expected 4g memory, got %s", capturedOpts.Limits.Memory)
	}
}

func TestValidateSourceBuild_ContainerError(t *testing.T) {
	mockPodman := &mockRuntime{
		name:     "podman",
		rootless: true,
		runFunc: func(ctx context.Context, opts RunOptions) (*RunResult, error) {
			return &RunResult{
				ExitCode: 1,
				Stderr:   "build failed",
			}, nil
		},
	}

	detector := NewRuntimeDetector()
	detector.detected = mockPodman
	detector.checked = true

	executor := NewExecutor(detector, NewPreDownloader())

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{Action: "configure_make"},
		},
		Verify: recipe.VerifySection{
			Command: "test-tool --version",
			Pattern: "1.0.0",
		},
	}

	result, err := executor.ValidateSourceBuild(context.Background(), r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected validation to fail")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}
