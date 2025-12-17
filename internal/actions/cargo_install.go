package actions

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ensure CargoInstallAction implements Decomposable
var _ Decomposable = (*CargoInstallAction)(nil)

// CargoInstallAction installs Rust crates using cargo install with --root isolation
type CargoInstallAction struct{ BaseAction }

// Dependencies returns rust as eval-time, install-time, and runtime dependency.
// EvalTime is needed because Decompose() runs cargo to generate Cargo.lock.
func (CargoInstallAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"rust"},
		EvalTime:    []string{"rust"},
	}
}

// RequiresNetwork returns true because cargo_install fetches crates from crates.io.
func (CargoInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *CargoInstallAction) Name() string {
	return "cargo_install"
}

// Execute installs a Rust crate to the install directory
//
// Parameters:
//   - crate (required): crate name on crates.io
//   - executables (required): List of executable names to verify
//   - cargo_path (optional): Path to cargo (defaults to system cargo or tsuku's rust)
//
// Installation:
//
//	cargo install --root <install_dir> <crate>@<version>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
//	  .crates.toml         - Cargo metadata
//	  .crates2.json        - Cargo metadata
func (a *CargoInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get crate name (required)
	crateName, ok := GetString(params, "crate")
	if !ok {
		return fmt.Errorf("cargo_install action requires 'crate' parameter")
	}

	// SECURITY: Validate crate name to prevent command injection
	if !isValidCrateName(crateName) {
		return fmt.Errorf("invalid crate name '%s': must match crates.io naming rules", crateName)
	}

	// SECURITY: Validate version string
	if !isValidCargoVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match semver format", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("cargo_install action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get cargo path (optional, defaults to "cargo" from PATH or tsuku's rust)
	cargoPath, _ := GetString(params, "cargo_path")
	if cargoPath == "" {
		// Try to resolve cargo from tsuku's tools directory first
		cargoPath = ResolveCargo()
		if cargoPath == "" {
			// Fallback to cargo in PATH
			cargoPath = "cargo"
		}
	}

	fmt.Printf("   Crate: %s@%s\n", crateName, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using cargo: %s\n", cargoPath)

	// Install crate with --root for isolation
	installDir := ctx.InstallDir
	crateSpec := fmt.Sprintf("%s@%s", crateName, ctx.Version)

	fmt.Printf("   Installing: cargo install --root=%s %s\n", installDir, crateSpec)

	// Use CommandContext for cancellation support
	cmd := exec.CommandContext(ctx.Context, cargoPath, "install", "--root", installDir, crateSpec)

	// Set up environment: add cargo's bin directory to PATH
	// With the proper install.sh setup, cargo and rustc are both in bin/
	cargoDir := filepath.Dir(cargoPath)
	env := os.Environ()
	env = append(env, fmt.Sprintf("PATH=%s:%s", cargoDir, os.Getenv("PATH")))

	// Set up C compiler for crates with native dependencies
	// Prefer system compiler (gcc) when available because it has better compatibility
	// Fall back to zig if no system compiler is found
	if !hasSystemCompiler() {
		if newEnv, found := SetupCCompilerEnv(env); found {
			env = newEnv
			fmt.Printf("   Using zig as C compiler for native dependencies\n")
		}
	}
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo install failed: %w\nOutput: %s", err, string(output))
	}

	// cargo is verbose, only show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   cargo output:\n%s\n", outputStr)
	}

	// Verify executables exist
	binDir := filepath.Join(installDir, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}
	}

	fmt.Printf("   ✓ Crate installed successfully\n")
	fmt.Printf("   ✓ Verified %d executable(s)\n", len(executables))

	return nil
}

// Decompose converts a cargo_install composite action into a cargo_build primitive step.
// This is called during plan generation to capture the Cargo.lock at eval time.
//
// The decomposition:
//  1. Creates a temporary directory with a minimal Cargo.toml
//  2. Runs `cargo generate-lockfile` to create Cargo.lock
//  3. Optionally runs `cargo fetch --locked` to verify checksums
//  4. Returns a cargo_build step with the captured lock_data
func (a *CargoInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get crate name (required)
	crateName, ok := GetString(params, "crate")
	if !ok {
		return nil, fmt.Errorf("cargo_install action requires 'crate' parameter")
	}

	// Validate crate name
	if !isValidCrateName(crateName) {
		return nil, fmt.Errorf("invalid crate name '%s'", crateName)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return nil, fmt.Errorf("cargo_install action requires 'executables' parameter with at least one executable")
	}

	// Use Version from context
	version := ctx.Version
	if version == "" {
		return nil, fmt.Errorf("cargo_install decomposition requires a resolved version")
	}

	// Validate version format
	if !isValidCargoVersion(version) {
		return nil, fmt.Errorf("invalid version format '%s'", version)
	}

	// Find cargo command
	cargoPath := findCargoForEval()
	if cargoPath == "" {
		return nil, fmt.Errorf("cargo not found: install Rust first (tsuku install rust)")
	}

	// Create temp directory for lockfile generation
	tempDir, err := os.MkdirTemp("", "tsuku-cargo-decompose-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate Cargo.lock
	lockData, err := generateCargoLock(ctx, cargoPath, crateName, version, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Cargo.lock: %w", err)
	}

	// Compute Cargo.lock checksum
	lockSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(lockData)))

	// Get Rust version for metadata
	rustVersion := getRustVersionForCargo(cargoPath)

	// Build cargo_build params
	cargoBuildParams := map[string]interface{}{
		"crate":         crateName,
		"version":       version,
		"executables":   executables,
		"lock_data":     lockData,
		"lock_checksum": lockSHA,
	}

	// Add Rust version info if available
	if rustVersion != "" {
		cargoBuildParams["rust_version"] = rustVersion
	}

	return []Step{
		{
			Action: "cargo_build",
			Params: cargoBuildParams,
		},
	}, nil
}

// findCargoForEval finds the cargo executable for eval-time decomposition.
func findCargoForEval() string {
	// Try tsuku's installed Rust first
	tsukuHome := os.Getenv("TSUKU_HOME")
	if tsukuHome == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			tsukuHome = filepath.Join(homeDir, ".tsuku")
		}
	}

	if tsukuHome != "" {
		// Look for rust installation with cargo
		patterns := []string{
			filepath.Join(tsukuHome, "tools", "rust-*", "bin", "cargo"),
			filepath.Join(tsukuHome, "tools", "current", "bin", "cargo"),
		}
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return matches[0]
			}
		}

		// Check if cargo symlink exists in bin
		cargoBin := filepath.Join(tsukuHome, "bin", "cargo")
		if _, err := os.Stat(cargoBin); err == nil {
			return cargoBin
		}
	}

	// Try system cargo
	path, err := exec.LookPath("cargo")
	if err == nil {
		return path
	}

	return ""
}

// generateCargoLock generates a Cargo.lock using cargo generate-lockfile.
func generateCargoLock(ctx *EvalContext, cargoPath, crateName, version, tempDir string) (string, error) {
	// Create src directory and minimal main.rs (required for valid Cargo package)
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create src directory: %w", err)
	}

	mainRsPath := filepath.Join(srcDir, "main.rs")
	mainRsContent := "fn main() {}\n"
	if err := os.WriteFile(mainRsPath, []byte(mainRsContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write main.rs: %w", err)
	}

	// Write minimal Cargo.toml
	cargoTomlPath := filepath.Join(tempDir, "Cargo.toml")
	cargoTomlContent := fmt.Sprintf(`[package]
name = "tsuku-temp"
version = "0.0.0"
edition = "2021"

[dependencies]
%s = "=%s"
`, crateName, version)

	if err := os.WriteFile(cargoTomlPath, []byte(cargoTomlContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write Cargo.toml: %w", err)
	}

	// Generate Cargo.lock
	cmd := exec.CommandContext(ctx.Context, cargoPath, "generate-lockfile", "--manifest-path", cargoTomlPath)
	cmd.Dir = tempDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cargo generate-lockfile failed: %w\nOutput: %s", err, string(output))
	}

	// Read generated Cargo.lock
	lockPath := filepath.Join(tempDir, "Cargo.lock")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		return "", fmt.Errorf("failed to read Cargo.lock: %w", err)
	}

	return string(lockData), nil
}

// getRustVersionForCargo returns the Rust compiler version for metadata.
func getRustVersionForCargo(cargoPath string) string {
	// Get rustc from same directory as cargo
	rustcPath := filepath.Join(filepath.Dir(cargoPath), "rustc")
	if _, err := os.Stat(rustcPath); err != nil {
		// Try system rustc
		var lookupErr error
		rustcPath, lookupErr = exec.LookPath("rustc")
		if lookupErr != nil {
			return ""
		}
	}

	cmd := exec.Command(rustcPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse "rustc 1.76.0 (07dca489a 2024-02-04)"
	outputStr := string(output)
	parts := strings.Fields(outputStr)
	if len(parts) >= 2 && parts[0] == "rustc" {
		return parts[1]
	}

	return ""
}

// isValidCrateName validates crate names to prevent command injection
// Crate names on crates.io: alphanumeric, hyphens, underscores
// Max length: 64 characters, must start with letter
func isValidCrateName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}

	// Must start with a letter
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}

	// Check allowed characters
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

// isValidCargoVersion validates semver version strings
// Valid: 0.1.0, 1.2.3, 1.2.3-alpha, 1.2.3+build
// Invalid: anything with shell metacharacters
func isValidCargoVersion(version string) bool {
	if version == "" || len(version) > 50 {
		return false
	}

	// Must start with a digit
	if version[0] < '0' || version[0] > '9' {
		return false
	}

	// Allow semver characters: digits, dots, hyphens, plus, alphanumeric
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '+') {
			return false
		}
	}

	return true
}
