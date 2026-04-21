package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// logRecorder is a test reporter that captures Log calls for assertion.
type logRecorder struct {
	progress.NoopReporter
	logs []string
}

func (r *logRecorder) Log(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func TestCargoBuildAction_Name(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	if action.Name() != "cargo_build" {
		t.Errorf("Name() = %q, want %q", action.Name(), "cargo_build")
	}
}

func TestIsValidTargetTriple(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		target    string
		wantValid bool
	}{
		// Valid target triples
		{"linux gnu", "x86_64-unknown-linux-gnu", true},
		{"linux musl", "x86_64-unknown-linux-musl", true},
		{"macos intel", "x86_64-apple-darwin", true},
		{"macos arm", "aarch64-apple-darwin", true},
		{"windows msvc", "x86_64-pc-windows-msvc", true},
		{"windows gnu", "x86_64-pc-windows-gnu", true},
		{"arm linux", "arm-unknown-linux-gnueabi", true},
		{"riscv linux", "riscv64gc-unknown-linux-gnu", true},
		{"with underscore", "x86_64_v3-unknown-linux-gnu", true},

		// Invalid target triples
		{"empty", "", false},
		{"single component", "linux", false},
		{"two components", "x86_64-linux", false},
		{"shell injection semicolon", "x86_64;rm -rf /", false},
		{"shell injection dollar", "x86_64-$USER-linux", false},
		{"shell injection backtick", "x86_64-`whoami`-linux", false},
		{"path traversal", "x86_64-../../../etc-linux", false},
		{"too long", strings.Repeat("a", 101), false},
		{"spaces", "x86_64 unknown linux gnu", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTargetTriple(tt.target)
			if got != tt.wantValid {
				t.Errorf("isValidTargetTriple(%q) = %v, want %v", tt.target, got, tt.wantValid)
			}
		})
	}
}

func TestIsValidFeatureName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		feature   string
		wantValid bool
	}{
		// Valid feature names
		{"simple", "serde", true},
		{"with hyphen", "full-crypto", true},
		{"with underscore", "async_runtime", true},
		{"with numbers", "v2", true},
		{"namespaced", "tokio/rt-multi-thread", true},
		{"all caps", "FEATURE", true},

		// Invalid feature names
		{"empty", "", false},
		{"shell injection semicolon", "feature;rm", false},
		{"shell injection dollar", "feature$var", false},
		{"shell injection backtick", "feature`cmd`", false},
		{"spaces", "my feature", false},
		{"too long", strings.Repeat("a", 101), false},
		{"special char at", "feature@1", false},
		{"special char colon", "feature:1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidFeatureName(tt.feature)
			if got != tt.wantValid {
				t.Errorf("isValidFeatureName(%q) = %v, want %v", tt.feature, got, tt.wantValid)
			}
		})
	}
}

func TestBuildDeterministicCargoEnv(t *testing.T) {
	t.Parallel()
	// Create a mock cargo path and work directory
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()

	env := buildDeterministicCargoEnv(cargoPath, workDir, nil)

	// Check that deterministic variables are set
	var hasCargoIncremental, hasSourceDateEpoch, hasRustflags bool
	var cargoIncrementalValue, sourceDateEpochValue, rustflagsValue string

	for _, e := range env {
		if strings.HasPrefix(e, "CARGO_INCREMENTAL=") {
			hasCargoIncremental = true
			cargoIncrementalValue = e[18:]
		}
		if strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			hasSourceDateEpoch = true
			sourceDateEpochValue = e[18:]
		}
		if strings.HasPrefix(e, "RUSTFLAGS=") {
			hasRustflags = true
			rustflagsValue = e[10:]
		}
	}

	if !hasCargoIncremental {
		t.Error("CARGO_INCREMENTAL should be set")
	} else if cargoIncrementalValue != "0" {
		t.Errorf("CARGO_INCREMENTAL = %q, want %q", cargoIncrementalValue, "0")
	}

	if !hasSourceDateEpoch {
		t.Error("SOURCE_DATE_EPOCH should be set")
	} else if sourceDateEpochValue != "0" {
		t.Errorf("SOURCE_DATE_EPOCH = %q, want %q", sourceDateEpochValue, "0")
	}

	// RUSTFLAGS is no longer unconditionally set since we removed -C embed-bitcode=no
	// to fix compatibility with crates that enable LTO in their Cargo.toml.
	// RUSTFLAGS may still be present if inherited from the environment.
	if hasRustflags && strings.Contains(rustflagsValue, "-C embed-bitcode=no") {
		t.Errorf("RUSTFLAGS should not contain '-C embed-bitcode=no' (conflicts with LTO), got %q", rustflagsValue)
	}
}

func TestBuildDeterministicCargoEnv_PathIncludesCargoDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()

	env := buildDeterministicCargoEnv(cargoPath, workDir, nil)

	// Check that PATH includes the cargo directory
	var pathValue string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathValue = e[5:]
			break
		}
	}

	if !strings.HasPrefix(pathValue, tmpDir+":") {
		t.Errorf("PATH should start with cargo directory, got %q", pathValue)
	}
}

func TestBuildDeterministicCargoEnv_IsolatedCargoHome(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()

	env := buildDeterministicCargoEnv(cargoPath, workDir, nil)

	// Check that CARGO_HOME is set to isolated directory
	var cargoHomeValue string
	for _, e := range env {
		if strings.HasPrefix(e, "CARGO_HOME=") {
			cargoHomeValue = e[11:]
			break
		}
	}

	expectedCargoHome := filepath.Join(workDir, ".cargo-home")
	if cargoHomeValue != expectedCargoHome {
		t.Errorf("CARGO_HOME = %q, want %q", cargoHomeValue, expectedCargoHome)
	}
}

func TestCargoBuildAction_Execute_MissingSourceDir(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		// Missing "source_dir" parameter
		"executables": []interface{}{"myapp"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'source_dir' parameter is missing")
	}
	if err != nil && !strings.Contains(err.Error(), "source_dir") {
		t.Errorf("Error message should mention 'source_dir', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir": workDir,
		// Missing "executables" parameter
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' parameter is missing")
	}
	if err != nil && !strings.Contains(err.Error(), "executables") {
		t.Errorf("Error message should mention 'executables', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_EmptyExecutables(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{}, // Empty list
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' is empty")
	}
}

func TestCargoBuildAction_Execute_CargoTomlNotFound(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	workDir := t.TempDir() // Empty directory, no Cargo.toml

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when Cargo.toml is not found")
	}
	if err != nil && !strings.Contains(err.Error(), "Cargo.toml not found") {
		t.Errorf("Error message should mention 'Cargo.toml not found', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_CargoLockNotFound(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml but no Cargo.lock
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
		"locked":      true, // Request locked build
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when Cargo.lock is not found for locked build")
	}
	if err != nil && !strings.Contains(err.Error(), "Cargo.lock not found") {
		t.Errorf("Error message should mention 'Cargo.lock not found', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_InvalidExecutableName(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	tests := []struct {
		name string
		exe  string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"absolute path", "/bin/bash"},
		{"backslash", "app\\test"},
		{"double dot", ".."},
		{"single dot", "."},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"source_dir":  workDir,
				"executables": []interface{}{tt.exe},
			}

			err := action.Execute(ctx, params)
			if err == nil {
				t.Errorf("Execute() should fail with invalid executable name %q", tt.exe)
			}
			if err != nil && !strings.Contains(err.Error(), "invalid executable name") {
				t.Errorf("Error message should mention 'invalid executable name', got: %v", err)
			}
		})
	}
}

func TestCargoBuildAction_Execute_InvalidTargetTriple(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml and Cargo.lock
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
		"target":      "invalid;rm -rf /", // Injection attempt
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with invalid target triple")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid target triple") {
		t.Errorf("Error message should mention 'invalid target triple', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_InvalidFeatureName(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create a temp directory with Cargo.toml and Cargo.lock
	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
		"features":    []interface{}{"feature;rm -rf /"}, // Injection attempt
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with invalid feature name")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid feature name") {
		t.Errorf("Error message should mention 'invalid feature name', got: %v", err)
	}
}

func TestCargoBuildAction_Execute_RelativeSourceDir(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}

	// Create work directory with a subdirectory containing Cargo.toml and Cargo.lock
	workDir := t.TempDir()
	projectDir := filepath.Join(workDir, "myproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	cargoToml := filepath.Join(projectDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(projectDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	// Use relative path
	params := map[string]interface{}{
		"source_dir":  "myproject", // Relative to WorkDir
		"executables": []interface{}{"myapp"},
	}

	// This will fail because cargo is not installed, but it should get past
	// the Cargo.toml and Cargo.lock checks (which verifies relative path resolution works)
	err := action.Execute(ctx, params)
	// The error should NOT be about Cargo.toml or Cargo.lock not found
	if err != nil && (strings.Contains(err.Error(), "Cargo.toml not found") ||
		strings.Contains(err.Error(), "Cargo.lock not found")) {
		t.Errorf("Relative source_dir should be resolved correctly, got: %v", err)
	}
}

func TestCargoBuildAction_Execute_LockedDefault(t *testing.T) {
	t.Parallel()
	// This test verifies that locked defaults to true by asserting the reporter
	// receives "Locked: true" before cargo is invoked.
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	recorder := &logRecorder{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
		Reporter:   recorder,
	}

	// Don't specify locked - should default to true
	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
	}

	// Discard error - cargo is not installed, but reporter.Log calls happen before invocation
	_ = action.Execute(ctx, params)

	found := false
	for _, line := range recorder.logs {
		if strings.Contains(line, "Locked: true") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected reporter to log 'Locked: true' for default locked parameter")
	}
}

func TestCargoBuildIsPrimitive(t *testing.T) {
	t.Parallel()
	// Verify that cargo_build is registered as a primitive
	if !IsPrimitive("cargo_build") {
		t.Error("cargo_build should be registered as a primitive action")
	}
}

func TestCargoBuildIsRegistered(t *testing.T) {
	t.Parallel()
	// Verify that cargo_build is registered in the action registry
	action := Get("cargo_build")
	if action == nil {
		t.Error("cargo_build should be registered in the action registry")
	}
	if action != nil && action.Name() != "cargo_build" {
		t.Errorf("action.Name() = %q, want %q", action.Name(), "cargo_build")
	}
}

func TestCargoBuildAction_Execute_OfflineDefault(t *testing.T) {
	t.Parallel()
	// This test verifies that offline defaults to true by asserting the reporter
	// receives "Offline: true" before cargo is invoked.
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	recorder := &logRecorder{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
		Reporter:   recorder,
	}

	// Don't specify offline - should default to true
	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
	}

	// Discard error - cargo is not installed, but reporter.Log calls happen before invocation
	_ = action.Execute(ctx, params)

	found := false
	for _, line := range recorder.logs {
		if strings.Contains(line, "Offline: true") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected reporter to log 'Offline: true' for default offline parameter")
	}
}

func TestCargoBuildAction_Execute_NoDefaultFeatures(t *testing.T) {
	t.Parallel()
	// Test that no_default_features parameter is handled
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":          workDir,
		"executables":         []interface{}{"myapp"},
		"no_default_features": true,
	}

	// The test will fail because cargo is not installed, but the parameter
	// handling code path is exercised
	_ = action.Execute(ctx, params)
}

func TestCargoBuildAction_Execute_AllFeatures(t *testing.T) {
	t.Parallel()
	// Test that all_features parameter is handled
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":   workDir,
		"executables":  []interface{}{"myapp"},
		"all_features": true,
	}

	// The test will fail because cargo is not installed, but the parameter
	// handling code path is exercised
	_ = action.Execute(ctx, params)
}

func TestCargoBuildAction_Execute_OfflineDisabled(t *testing.T) {
	t.Parallel()
	// Test that offline can be explicitly disabled
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	cargoLock := filepath.Join(workDir, "Cargo.lock")
	if err := os.WriteFile(cargoLock, []byte("# This file is auto-generated\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.lock: %v", err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
		"offline":     false, // Explicitly disable offline mode
	}

	// When offline is false, we should skip the pre-fetch step
	// The test will fail because cargo is not installed, but without the
	// "cargo fetch failed" error that would occur with offline=true
	_ = action.Execute(ctx, params)
}

func TestCargoBuildAction_Execute_LockedDisabled(t *testing.T) {
	t.Parallel()
	// Test that locked can be explicitly disabled (skips Cargo.lock check)
	action := &CargoBuildAction{}

	workDir := t.TempDir()
	cargoToml := filepath.Join(workDir, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}
	// Intentionally NOT creating Cargo.lock

	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		WorkDir:    workDir,
	}

	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
		"locked":      false, // Disable locked mode
		"offline":     false, // Also disable offline to avoid fetch
	}

	// With locked=false, should NOT fail due to missing Cargo.lock
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "Cargo.lock not found") {
		t.Error("With locked=false, should not check for Cargo.lock")
	}
}

// --- linkCargoRegistryCache tests ---

func TestLinkCargoRegistryCache_CreatesSymlink(t *testing.T) {
	t.Parallel()

	cargoHome := filepath.Join(t.TempDir(), ".cargo-home")
	registryCache := t.TempDir()

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=" + registryCache,
	}

	if err := linkCargoRegistryCache(env); err != nil {
		t.Fatalf("linkCargoRegistryCache returned error: %v", err)
	}

	// Verify CARGO_HOME was created
	if _, err := os.Stat(cargoHome); err != nil {
		t.Fatalf("CARGO_HOME directory not created: %v", err)
	}

	// Verify symlink exists and points to the cache directory
	registryPath := filepath.Join(cargoHome, "registry")
	target, err := os.Readlink(registryPath)
	if err != nil {
		t.Fatalf("Expected symlink at %s: %v", registryPath, err)
	}
	if target != registryCache {
		t.Errorf("Symlink target = %q, want %q", target, registryCache)
	}
}

func TestLinkCargoRegistryCache_NoCargoHome(t *testing.T) {
	t.Parallel()

	registryCache := t.TempDir()
	env := []string{
		"TSUKU_CARGO_REGISTRY_CACHE=" + registryCache,
	}

	// Should be a no-op when CARGO_HOME is missing
	if err := linkCargoRegistryCache(env); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

func TestLinkCargoRegistryCache_NoRegistryCache(t *testing.T) {
	t.Parallel()

	cargoHome := filepath.Join(t.TempDir(), ".cargo-home")
	env := []string{
		"CARGO_HOME=" + cargoHome,
	}

	// Should be a no-op when TSUKU_CARGO_REGISTRY_CACHE is missing
	if err := linkCargoRegistryCache(env); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// CARGO_HOME should NOT be created (nothing to link)
	if _, err := os.Stat(cargoHome); !os.IsNotExist(err) {
		t.Error("CARGO_HOME should not be created when there is no registry cache to link")
	}
}

func TestLinkCargoRegistryCache_CacheDirDoesNotExist(t *testing.T) {
	t.Parallel()

	cargoHome := filepath.Join(t.TempDir(), ".cargo-home")
	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=/nonexistent/path",
	}

	// Should silently skip when the cache directory doesn't exist (mount not present)
	if err := linkCargoRegistryCache(env); err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// CARGO_HOME should NOT be created
	if _, err := os.Stat(cargoHome); !os.IsNotExist(err) {
		t.Error("CARGO_HOME should not be created when cache dir does not exist")
	}
}

func TestLinkCargoRegistryCache_ReplacesExistingRegistry(t *testing.T) {
	t.Parallel()

	cargoHome := filepath.Join(t.TempDir(), ".cargo-home")
	registryCache := t.TempDir()

	// Create an existing registry file (not directory) to verify it gets replaced
	if err := os.MkdirAll(cargoHome, 0755); err != nil {
		t.Fatal(err)
	}
	existingRegistry := filepath.Join(cargoHome, "registry")
	if err := os.WriteFile(existingRegistry, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=" + registryCache,
	}

	if err := linkCargoRegistryCache(env); err != nil {
		t.Fatalf("linkCargoRegistryCache returned error: %v", err)
	}

	// Verify symlink replaced the existing file
	target, err := os.Readlink(existingRegistry)
	if err != nil {
		t.Fatalf("Expected symlink at %s: %v", existingRegistry, err)
	}
	if target != registryCache {
		t.Errorf("Symlink target = %q, want %q", target, registryCache)
	}
}

func TestBuildDeterministicCargoEnv_WithDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()
	libsDir := t.TempDir()
	toolsDir := t.TempDir()

	// Create a library dependency with lib/pkgconfig, include, and lib directories
	opensslDir := filepath.Join(libsDir, "openssl-3.6.1")
	for _, sub := range []string{"lib/pkgconfig", "include", "bin"} {
		if err := os.MkdirAll(filepath.Join(opensslDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create a tool dependency with only a bin directory
	pkgConfigDir := filepath.Join(toolsDir, "pkg-config-0.29.2")
	if err := os.MkdirAll(filepath.Join(pkgConfigDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		LibsDir:  libsDir,
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{
				"openssl":    "3.6.1",
				"pkg-config": "0.29.2",
			},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, workDir, ctx)

	var pkgConfigPath, cIncludePath, libraryPath, pathValue string
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		} else if strings.HasPrefix(e, "C_INCLUDE_PATH=") {
			cIncludePath = strings.TrimPrefix(e, "C_INCLUDE_PATH=")
		} else if strings.HasPrefix(e, "LIBRARY_PATH=") {
			libraryPath = strings.TrimPrefix(e, "LIBRARY_PATH=")
		} else if strings.HasPrefix(e, "PATH=") {
			pathValue = strings.TrimPrefix(e, "PATH=")
		}
	}

	// PKG_CONFIG_PATH should include openssl's pkgconfig dir
	expectedPkgConfig := filepath.Join(opensslDir, "lib", "pkgconfig")
	if pkgConfigPath == "" {
		t.Error("PKG_CONFIG_PATH not set")
	} else if !strings.Contains(pkgConfigPath, expectedPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH = %q, want to contain %q", pkgConfigPath, expectedPkgConfig)
	}

	// C_INCLUDE_PATH should include openssl's include dir
	expectedInclude := filepath.Join(opensslDir, "include")
	if cIncludePath == "" {
		t.Error("C_INCLUDE_PATH not set")
	} else if !strings.Contains(cIncludePath, expectedInclude) {
		t.Errorf("C_INCLUDE_PATH = %q, want to contain %q", cIncludePath, expectedInclude)
	}

	// LIBRARY_PATH should include openssl's lib dir
	expectedLib := filepath.Join(opensslDir, "lib")
	if libraryPath == "" {
		t.Error("LIBRARY_PATH not set")
	} else if !strings.Contains(libraryPath, expectedLib) {
		t.Errorf("LIBRARY_PATH = %q, want to contain %q", libraryPath, expectedLib)
	}

	// PATH should include both openssl/bin and pkg-config/bin
	opensslBin := filepath.Join(opensslDir, "bin")
	pkgConfigBin := filepath.Join(pkgConfigDir, "bin")
	if !strings.Contains(pathValue, opensslBin) {
		t.Errorf("PATH should contain openssl bin dir %q, got %q", opensslBin, pathValue)
	}
	if !strings.Contains(pathValue, pkgConfigBin) {
		t.Errorf("PATH should contain pkg-config bin dir %q, got %q", pkgConfigBin, pathValue)
	}
}

func TestBuildDeterministicCargoEnv_NoDependencies(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: workDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, workDir, ctx)

	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			t.Error("PKG_CONFIG_PATH should not be set with no dependencies")
		}
		if strings.HasPrefix(e, "C_INCLUDE_PATH=") {
			t.Error("C_INCLUDE_PATH should not be set with no dependencies")
		}
		if strings.HasPrefix(e, "LIBRARY_PATH=") {
			t.Error("LIBRARY_PATH should not be set with no dependencies")
		}
	}
}

func TestBuildDeterministicCargoEnv_LibsDirPreferredOverToolsDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()
	libsDir := t.TempDir()
	toolsDir := t.TempDir()

	// Create the same dependency in both libs and tools directories
	libsDep := filepath.Join(libsDir, "zlib-1.3")
	toolsDep := filepath.Join(toolsDir, "zlib-1.3")
	for _, dir := range []string{libsDep, toolsDep} {
		if err := os.MkdirAll(filepath.Join(dir, "lib", "pkgconfig"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		LibsDir:  libsDir,
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"zlib": "1.3"},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, workDir, ctx)

	var pkgConfigPath string
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		}
	}

	// Should use libs directory, not tools directory
	expectedPkgConfig := filepath.Join(libsDep, "lib", "pkgconfig")
	unexpectedPkgConfig := filepath.Join(toolsDep, "lib", "pkgconfig")

	if !strings.Contains(pkgConfigPath, expectedPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH should use libs dir, got %q", pkgConfigPath)
	}
	if strings.Contains(pkgConfigPath, unexpectedPkgConfig) {
		t.Errorf("PKG_CONFIG_PATH should not use tools dir when libs dir exists, got %q", pkgConfigPath)
	}
}

func TestBuildDeterministicCargoEnv_FallsBackToToolsDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()
	libsDir := t.TempDir()
	toolsDir := t.TempDir()

	// Only create the dependency in tools directory (not libs)
	toolsDep := filepath.Join(toolsDir, "perl-5.38")
	if err := os.MkdirAll(filepath.Join(toolsDep, "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		LibsDir:  libsDir,
		ToolsDir: toolsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"perl": "5.38"},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, workDir, ctx)

	var pathValue string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathValue = strings.TrimPrefix(e, "PATH=")
		}
	}

	// Should find bin/ from tools directory
	expectedBin := filepath.Join(toolsDep, "bin")
	if !strings.Contains(pathValue, expectedBin) {
		t.Errorf("PATH should contain tools dep bin dir %q, got %q", expectedBin, pathValue)
	}
}

func TestBuildDeterministicCargoEnv_MissingSubdirectories(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "cargo")
	workDir := t.TempDir()
	libsDir := t.TempDir()

	// Create a library dependency with only a lib directory (no pkgconfig, no include, no bin)
	depDir := filepath.Join(libsDir, "zstd-1.5.6")
	if err := os.MkdirAll(filepath.Join(depDir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:  context.Background(),
		WorkDir:  workDir,
		LibsDir:  libsDir,
		ToolsDir: t.TempDir(),
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"zstd": "1.5.6"},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, workDir, ctx)

	var pkgConfigPath, cIncludePath, libraryPath string
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			pkgConfigPath = strings.TrimPrefix(e, "PKG_CONFIG_PATH=")
		} else if strings.HasPrefix(e, "C_INCLUDE_PATH=") {
			cIncludePath = strings.TrimPrefix(e, "C_INCLUDE_PATH=")
		} else if strings.HasPrefix(e, "LIBRARY_PATH=") {
			libraryPath = strings.TrimPrefix(e, "LIBRARY_PATH=")
		}
	}

	// Only LIBRARY_PATH should be set (lib/ exists but not lib/pkgconfig or include/)
	if pkgConfigPath != "" {
		t.Errorf("PKG_CONFIG_PATH should not be set when lib/pkgconfig doesn't exist, got %q", pkgConfigPath)
	}
	if cIncludePath != "" {
		t.Errorf("C_INCLUDE_PATH should not be set when include/ doesn't exist, got %q", cIncludePath)
	}
	expectedLib := filepath.Join(depDir, "lib")
	if !strings.Contains(libraryPath, expectedLib) {
		t.Errorf("LIBRARY_PATH should contain %q, got %q", expectedLib, libraryPath)
	}
}

func TestLinkCargoRegistryCache_EmptyEnv(t *testing.T) {
	t.Parallel()

	// Should be a no-op with empty env
	if err := linkCargoRegistryCache(nil); err != nil {
		t.Fatalf("Expected no error with nil env, got: %v", err)
	}
	if err := linkCargoRegistryCache([]string{}); err != nil {
		t.Fatalf("Expected no error with empty env, got: %v", err)
	}
}

// -- cargo_build.go: executeLockDataMode validation paths --

func TestCargoBuildAction_ExecuteLockDataMode_InvalidCrateName(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "!invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid crate") {
		t.Errorf("Expected invalid crate error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "!invalid",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "ripgrep",
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_MissingLockChecksum(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
		"crate":     "ripgrep",
	})
	if err == nil || !strings.Contains(err.Error(), "lock_checksum") {
		t.Errorf("Expected lock_checksum error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data":     "[dependencies]\n",
		"crate":         "ripgrep",
		"lock_checksum": "abc123",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestCargoBuildAction_ExecuteLockDataMode_InvalidExecutable(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data":     "[dependencies]\n",
		"crate":         "ripgrep",
		"lock_checksum": "abc123",
		"executables":   []any{"../evil"},
	})
	if err == nil || !strings.Contains(err.Error(), "path separator") {
		t.Errorf("Expected path separator error, got %v", err)
	}
}

// -- cargo_build.go: Execute source_dir mode with features that have path traversal --

func TestCargoBuildAction_Execute_WithPathTraversalFeature(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Feature with path traversal (.. is rejected by isValidFeatureName)
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
		"features":    []any{"..evil"},
	})
	if err == nil {
		t.Error("Expected error for feature with path traversal")
	}
}

// -- cargo_build.go: Execute source_dir mode with options --

func TestCargoBuildAction_Execute_WithOptions(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	tmpDir := t.TempDir()
	// Create Cargo.toml and Cargo.lock for locked build
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	// Request unlocked build (locked=false) so we skip the Cargo.lock check
	// Build will still fail at actual cargo invocation but will pass validation
	err := action.Execute(ctx, map[string]any{
		"source_dir":  tmpDir,
		"executables": []any{"tool"},
		"locked":      false,
	})
	// Should fail at cargo build, not at validation
	if err != nil && strings.Contains(err.Error(), "source_dir") {
		t.Errorf("Expected cargo build error, not validation error: %v", err)
	}
}

// -- cargo_build.go: Dependencies, RequiresNetwork --

func TestCargoBuildAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := CargoBuildAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "rust" {
		t.Errorf("Dependencies().InstallTime = %v, want [rust]", deps.InstallTime)
	}
}

func TestCargoBuildAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := CargoBuildAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- cargo_build.go: linkCargoRegistryCache --

func TestLinkCargoRegistryCache_NoEnvVars(t *testing.T) {
	t.Parallel()
	err := linkCargoRegistryCache([]string{"PATH=/usr/bin"})
	if err != nil {
		t.Errorf("linkCargoRegistryCache() with no env vars = %v, want nil", err)
	}
}

func TestLinkCargoRegistryCache_MissingMount(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoHome := filepath.Join(tmpDir, "cargo-home")

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=/nonexistent/path",
	}
	err := linkCargoRegistryCache(env)
	if err != nil {
		t.Errorf("linkCargoRegistryCache() with missing mount = %v, want nil", err)
	}
}

func TestLinkCargoRegistryCache_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoHome := filepath.Join(tmpDir, "cargo-home")
	registryCache := filepath.Join(tmpDir, "registry-cache")
	if err := os.MkdirAll(registryCache, 0755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=" + registryCache,
	}
	err := linkCargoRegistryCache(env)
	if err != nil {
		t.Fatalf("linkCargoRegistryCache() = %v", err)
	}

	registryPath := filepath.Join(cargoHome, "registry")
	target, err := os.Readlink(registryPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error: %v", registryPath, err)
	}
	if target != registryCache {
		t.Errorf("symlink target = %s, want %s", target, registryCache)
	}
}

// -- cargo_build.go: buildDeterministicCargoEnv --

func TestBuildDeterministicCargoEnv_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, nil)

	hasCargoHome := false
	hasCargoIncremental := false
	hasSourceDateEpoch := false
	for _, e := range env {
		if strings.HasPrefix(e, "CARGO_HOME=") {
			hasCargoHome = true
		}
		if e == "CARGO_INCREMENTAL=0" {
			hasCargoIncremental = true
		}
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSourceDateEpoch = true
		}
	}
	if !hasCargoHome {
		t.Error("Expected CARGO_HOME in env")
	}
	if !hasCargoIncremental {
		t.Error("Expected CARGO_INCREMENTAL=0 in env")
	}
	if !hasSourceDateEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0 in env")
	}
}

func TestBuildDeterministicCargoEnv_WithContext(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(tmpDir, "extra-bin")
	if err := os.MkdirAll(execPath, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:   context.Background(),
		WorkDir:   tmpDir,
		ExecPaths: []string{execPath},
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, ctx)

	hasExecInPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") && strings.Contains(e, execPath) {
			hasExecInPath = true
		}
	}
	if !hasExecInPath {
		t.Error("Expected ExecPaths in PATH")
	}
}

func TestBuildDeterministicCargoEnv_WithLibDeps(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a library dep with lib, include, lib/pkgconfig, and bin
	libsDir := filepath.Join(tmpDir, "libs")
	depDir := filepath.Join(libsDir, "openssl-3.0.0")
	for _, sub := range []string{"bin", "lib/pkgconfig", "include"} {
		if err := os.MkdirAll(filepath.Join(depDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		LibsDir: libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, ctx)

	hasPkgConfig := false
	hasCInclude := false
	hasLibrary := false
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") && strings.Contains(e, "openssl") {
			hasPkgConfig = true
		}
		if strings.HasPrefix(e, "C_INCLUDE_PATH=") && strings.Contains(e, "openssl") {
			hasCInclude = true
		}
		if strings.HasPrefix(e, "LIBRARY_PATH=") && strings.Contains(e, "openssl") {
			hasLibrary = true
		}
	}
	if !hasPkgConfig {
		t.Error("Expected PKG_CONFIG_PATH with openssl")
	}
	if !hasCInclude {
		t.Error("Expected C_INCLUDE_PATH with openssl")
	}
	if !hasLibrary {
		t.Error("Expected LIBRARY_PATH with openssl")
	}
}

// -- cargo_build.go: Execute with missing Cargo.toml --

func TestCargoBuildAction_Execute_MissingCargoToml(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
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
	if err == nil || !strings.Contains(err.Error(), "Cargo.toml") {
		t.Errorf("Expected Cargo.toml error, got %v", err)
	}
}

// -- cargo_build.go: Execute with lock_data mode --

func TestCargoBuildAction_Execute_LockDataMode_MissingCrate(t *testing.T) {
	t.Parallel()
	action := &CargoBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: filepath.Join(t.TempDir(), ".install"),
		Version:    "1.0.0",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"lock_data": "[dependencies]\n",
	})
	if err == nil {
		t.Error("Expected error for missing crate in lock_data mode")
	}
}
