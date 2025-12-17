package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestIsValidCrateName tests crate name validation
func TestIsValidCrateName(t *testing.T) {
	tests := []struct {
		name      string
		crateName string
		wantValid bool
	}{
		// Valid crate names
		{"simple lowercase", "serde", true},
		{"with hyphen", "cargo-audit", true},
		{"with underscore", "tokio_util", true},
		{"mixed case", "TokioUtil", true},
		{"with numbers", "serde2", true},
		{"single letter", "a", true},

		// Invalid crate names
		{"empty string", "", false},
		{"starts with number", "123abc", false},
		{"starts with hyphen", "-cargo", false},
		{"starts with underscore", "_cargo", false},
		{"contains space", "cargo audit", false},
		{"contains special char @", "cargo@audit", false},
		{"contains special char .", "cargo.audit", false},
		{"contains slash", "cargo/audit", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false}, // 65 chars
		{"shell metachar semicolon", "cargo;rm", false},
		{"shell metachar dollar", "cargo$var", false},
		{"shell metachar backtick", "cargo`cmd`", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCrateName(tt.crateName)
			if got != tt.wantValid {
				t.Errorf("isValidCrateName(%q) = %v, want %v", tt.crateName, got, tt.wantValid)
			}
		})
	}
}

// TestIsValidCargoVersion tests version string validation
func TestIsValidCargoVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantValid bool
	}{
		// Valid versions
		{"simple semver", "1.0.0", true},
		{"two part", "1.0", true},
		{"single digit", "1", true},
		{"with prerelease alpha", "1.0.0-alpha", true},
		{"with prerelease beta", "1.0.0-beta1", true},
		{"with prerelease rc", "1.0.0-rc1", true},
		{"with build metadata", "1.0.0+build", true},
		{"complex prerelease", "1.0.0-alpha.1", true},
		{"large version", "100.200.300", true},

		// Invalid versions
		{"empty string", "", false},
		{"starts with letter", "v1.0.0", false},
		{"starts with hyphen", "-1.0.0", false},
		{"contains space", "1.0.0 ", false},
		{"shell metachar semicolon", "1.0.0;rm", false},
		{"shell metachar dollar", "1.0.0$var", false},
		{"shell metachar backtick", "1.0.0`cmd`", false},
		{"shell metachar pipe", "1.0.0|cat", false},
		{"shell metachar ampersand", "1.0.0&cmd", false},
		{"path traversal", "1.0.0/../etc", false},
		{"too long", "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", false}, // > 50 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCargoVersion(tt.version)
			if got != tt.wantValid {
				t.Errorf("isValidCargoVersion(%q) = %v, want %v", tt.version, got, tt.wantValid)
			}
		})
	}
}

// TestExecutableNameValidation tests that invalid executable names are rejected
func TestExecutableNameValidation(t *testing.T) {
	// This test verifies the validation logic in Execute() would reject these names
	invalidExeNames := []string{
		"../bin/cargo",
		"/etc/passwd",
		"cargo/../etc",
		".",
		"",
		"cargo\\audit",
	}

	for _, exe := range invalidExeNames {
		t.Run(exe, func(t *testing.T) {
			// Check the validation conditions from Execute()
			isInvalid := exe == "" || exe == "." ||
				containsSubstr(exe, "/") || containsSubstr(exe, "\\") || containsSubstr(exe, "..")

			if !isInvalid {
				t.Errorf("executable name %q should be rejected but validation passes", exe)
			}
		})
	}
}

// Helper function to check if string contains substring
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestCargoInstallAction_Execute_MissingCrate tests that Execute fails without crate parameter
func TestCargoInstallAction_Execute_MissingCrate(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		// Missing "crate" parameter
		"executables": []interface{}{"test-exe"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'crate' parameter is missing")
	}
	if err != nil && !containsSubstr(err.Error(), "crate") {
		t.Errorf("Error message should mention 'crate', got: %v", err)
	}
}

// TestCargoInstallAction_Execute_MissingExecutables tests that Execute fails without executables
func TestCargoInstallAction_Execute_MissingExecutables(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"crate": "cargo-audit",
		// Missing "executables" parameter
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' parameter is missing")
	}
	if err != nil && !containsSubstr(err.Error(), "executables") {
		t.Errorf("Error message should mention 'executables', got: %v", err)
	}
}

// TestCargoInstallAction_Execute_EmptyExecutables tests that Execute fails with empty executables
func TestCargoInstallAction_Execute_EmptyExecutables(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"crate":       "cargo-audit",
		"executables": []interface{}{}, // Empty list
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail when 'executables' is empty")
	}
}

// TestCargoInstallAction_Execute_InvalidCrateName tests that Execute fails with invalid crate name
func TestCargoInstallAction_Execute_InvalidCrateName(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"crate":       "cargo;rm -rf /", // Injection attempt
		"executables": []interface{}{"cargo"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in crate name")
	}
	if err != nil && !containsSubstr(err.Error(), "invalid crate name") {
		t.Errorf("Error message should mention 'invalid crate name', got: %v", err)
	}
}

// TestCargoInstallAction_Execute_InvalidVersion tests that Execute fails with invalid version
func TestCargoInstallAction_Execute_InvalidVersion(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0;rm -rf /", // Injection attempt
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"crate":       "cargo-audit",
		"executables": []interface{}{"cargo-audit"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with command injection in version")
	}
	if err != nil && !containsSubstr(err.Error(), "invalid version") {
		t.Errorf("Error message should mention 'invalid version', got: %v", err)
	}
}

// TestCargoInstallAction_Execute_InvalidExecutableName tests path traversal in executable names
func TestCargoInstallAction_Execute_InvalidExecutableName(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
		WorkDir:    t.TempDir(),
	}

	params := map[string]interface{}{
		"crate":       "cargo-audit",
		"executables": []interface{}{"../../../etc/passwd"}, // Path traversal
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Execute() should fail with path traversal in executable name")
	}
	if err != nil && !containsSubstr(err.Error(), "invalid executable name") {
		t.Errorf("Error message should mention 'invalid executable name', got: %v", err)
	}
}

// TestResolveCargo_NoRustInstalled tests ResolveCargo when no rust is installed
func TestResolveCargo_NoRustInstalled(t *testing.T) {
	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create empty .tsuku/tools directory (no rust)
	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("Failed to create tools dir: %v", err)
	}

	result := ResolveCargo()
	if result != "" {
		t.Errorf("ResolveCargo() should return empty string when no rust installed, got: %s", result)
	}
}

// TestResolveCargo_SingleRustVersion tests ResolveCargo with one rust version
func TestResolveCargo_SingleRustVersion(t *testing.T) {
	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create rust installation structure
	rustDir := filepath.Join(tmpHome, ".tsuku", "tools", "rust-1.70.0", "bin")
	if err := os.MkdirAll(rustDir, 0755); err != nil {
		t.Fatalf("Failed to create rust dir: %v", err)
	}

	// Create mock cargo executable
	cargoPath := filepath.Join(rustDir, "cargo")
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\necho mock"), 0755); err != nil {
		t.Fatalf("Failed to create mock cargo: %v", err)
	}

	result := ResolveCargo()
	if result != cargoPath {
		t.Errorf("ResolveCargo() = %q, want %q", result, cargoPath)
	}
}

// TestResolveCargo_MultipleVersions tests that ResolveCargo selects the latest version
func TestResolveCargo_MultipleVersions(t *testing.T) {
	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create multiple rust versions
	versions := []string{"rust-1.68.0", "rust-1.70.0", "rust-1.69.0"}
	for _, ver := range versions {
		rustDir := filepath.Join(tmpHome, ".tsuku", "tools", ver, "bin")
		if err := os.MkdirAll(rustDir, 0755); err != nil {
			t.Fatalf("Failed to create rust dir: %v", err)
		}
		cargoPath := filepath.Join(rustDir, "cargo")
		if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\necho mock"), 0755); err != nil {
			t.Fatalf("Failed to create mock cargo: %v", err)
		}
	}

	result := ResolveCargo()
	// Lexicographic sort means rust-1.70.0 comes last
	expectedPath := filepath.Join(tmpHome, ".tsuku", "tools", "rust-1.70.0", "bin", "cargo")
	if result != expectedPath {
		t.Errorf("ResolveCargo() = %q, want %q (latest version)", result, expectedPath)
	}
}

// TestResolveCargo_LegacyLocation tests ResolveCargo falls back to legacy cargo/bin/cargo
func TestResolveCargo_LegacyLocation(t *testing.T) {
	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create rust installation with legacy structure (cargo/bin/cargo)
	legacyDir := filepath.Join(tmpHome, ".tsuku", "tools", "rust-1.70.0", "cargo", "bin")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatalf("Failed to create legacy dir: %v", err)
	}

	// Create mock cargo in legacy location
	cargoPath := filepath.Join(legacyDir, "cargo")
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\necho mock"), 0755); err != nil {
		t.Fatalf("Failed to create mock cargo: %v", err)
	}

	result := ResolveCargo()
	if result != cargoPath {
		t.Errorf("ResolveCargo() = %q, want legacy path %q", result, cargoPath)
	}
}

// TestResolveCargo_NonExecutable tests that non-executable cargo is skipped
func TestResolveCargo_NonExecutable(t *testing.T) {
	// Create a temp directory as HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create rust installation structure
	rustDir := filepath.Join(tmpHome, ".tsuku", "tools", "rust-1.70.0", "bin")
	if err := os.MkdirAll(rustDir, 0755); err != nil {
		t.Fatalf("Failed to create rust dir: %v", err)
	}

	// Create non-executable cargo file (mode 0644, not 0755)
	cargoPath := filepath.Join(rustDir, "cargo")
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh\necho mock"), 0644); err != nil {
		t.Fatalf("Failed to create mock cargo: %v", err)
	}

	result := ResolveCargo()
	if result != "" {
		t.Errorf("ResolveCargo() should skip non-executable cargo, got: %s", result)
	}
}

// TestCargoInstallAction_Decompose tests the Decompose method
func TestCargoInstallAction_Decompose(t *testing.T) {
	action := &CargoInstallAction{}

	// Create eval context
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "14.1.0",
		OS:      "linux",
		Arch:    "amd64",
	}

	params := map[string]interface{}{
		"crate":       "ripgrep",
		"executables": []interface{}{"rg"},
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() failed: %v", err)
	}

	// Verify we got exactly one step
	if len(steps) != 1 {
		t.Fatalf("Decompose() should return 1 step, got %d", len(steps))
	}

	step := steps[0]

	// Verify the action is cargo_build
	if step.Action != "cargo_build" {
		t.Errorf("Decompose() action = %q, want %q", step.Action, "cargo_build")
	}

	// Verify required parameters are present
	requiredParams := []string{"crate", "version", "executables", "lock_data", "lock_checksum"}
	for _, param := range requiredParams {
		if _, ok := step.Params[param]; !ok {
			t.Errorf("Decompose() step missing required parameter: %q", param)
		}
	}

	// Verify crate name is correct
	if crate, ok := GetString(step.Params, "crate"); !ok || crate != "ripgrep" {
		t.Errorf("Decompose() crate = %q, want %q", crate, "ripgrep")
	}

	// Verify version is correct
	if version, ok := GetString(step.Params, "version"); !ok || version != "14.1.0" {
		t.Errorf("Decompose() version = %q, want %q", version, "14.1.0")
	}

	// Verify executables are correct
	executables, ok := GetStringSlice(step.Params, "executables")
	if !ok || len(executables) != 1 || executables[0] != "rg" {
		t.Errorf("Decompose() executables = %v, want [%q]", executables, "rg")
	}

	// Verify lock_data is not empty
	lockData, ok := GetString(step.Params, "lock_data")
	if !ok || lockData == "" {
		t.Error("Decompose() lock_data should not be empty")
	}

	// Verify lock_checksum is not empty and is a valid hex string
	lockChecksum, ok := GetString(step.Params, "lock_checksum")
	if !ok || lockChecksum == "" {
		t.Error("Decompose() lock_checksum should not be empty")
	}
	if len(lockChecksum) != 64 { // SHA256 hex is 64 characters
		t.Errorf("Decompose() lock_checksum length = %d, want 64 (SHA256)", len(lockChecksum))
	}
}

// TestCargoInstallAction_Decompose_MissingCrate tests Decompose with missing crate
func TestCargoInstallAction_Decompose_MissingCrate(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"executables": []interface{}{"test"},
		// Missing "crate"
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail when crate parameter is missing")
	}
	if err != nil && !containsSubstr(err.Error(), "crate") {
		t.Errorf("Error message should mention 'crate', got: %v", err)
	}
}

// TestCargoInstallAction_Decompose_MissingVersion tests Decompose with missing version
func TestCargoInstallAction_Decompose_MissingVersion(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		// Missing Version
	}

	params := map[string]interface{}{
		"crate":       "ripgrep",
		"executables": []interface{}{"rg"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail when version is not provided in context")
	}
	if err != nil && !containsSubstr(err.Error(), "version") {
		t.Errorf("Error message should mention 'version', got: %v", err)
	}
}

// TestCargoInstallAction_Decompose_InvalidCrateName tests Decompose with invalid crate name
func TestCargoInstallAction_Decompose_InvalidCrateName(t *testing.T) {
	action := &CargoInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}

	params := map[string]interface{}{
		"crate":       "cargo;rm -rf /", // Injection attempt
		"executables": []interface{}{"test"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail with invalid crate name")
	}
	if err != nil && !containsSubstr(err.Error(), "invalid crate name") {
		t.Errorf("Error message should mention 'invalid crate name', got: %v", err)
	}
}

// TestCargoInstallIsDecomposable tests that cargo_install implements Decomposable
func TestCargoInstallIsDecomposable(t *testing.T) {
	action := &CargoInstallAction{}
	_, ok := interface{}(action).(Decomposable)
	if !ok {
		t.Error("CargoInstallAction should implement Decomposable interface")
	}
}
