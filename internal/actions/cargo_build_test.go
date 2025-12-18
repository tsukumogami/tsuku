package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	env := buildDeterministicCargoEnv(cargoPath, workDir)

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

	env := buildDeterministicCargoEnv(cargoPath, workDir)

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

	env := buildDeterministicCargoEnv(cargoPath, workDir)

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
	// This test verifies that locked defaults to true
	// We can't easily test the actual cargo invocation without cargo installed,
	// but we can verify the parameter handling
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

	// Don't specify locked - should default to true
	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
	}

	// The test will fail because cargo is not installed, but the output
	// should show "Locked: true" and it should pass the Cargo.lock check
	_ = action.Execute(ctx, params)
	// We can't easily capture stdout here, but the code path is exercised
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
	// This test verifies that offline defaults to true for security
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

	// Don't specify offline - should default to true
	params := map[string]interface{}{
		"source_dir":  workDir,
		"executables": []interface{}{"myapp"},
	}

	// The test will fail because cargo is not installed, but the output
	// should show "Offline: true" demonstrating the default value
	_ = action.Execute(ctx, params)
	// Code path is exercised even if cargo fails
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
