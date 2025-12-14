package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CargoInstallAction installs Rust crates using cargo install with --root isolation
type CargoInstallAction struct{ BaseAction }

// Dependencies returns rust as an install-time dependency.
func (CargoInstallAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"rust"}}
}

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
