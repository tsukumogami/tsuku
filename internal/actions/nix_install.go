package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// NixInstallAction installs packages from Nixpkgs using an isolated internal Nix store.
// This action is completely isolated from any user-installed Nix.
//
// IMPORTANT: nix-portable only supports Linux. This action will fail on macOS.
//
// Philosophy: Following the same pattern as cargo/pipx/npm/gem actions:
//   - tsuku controls the installation path
//   - tsuku controls the package manager (users cannot use nix to modify tsuku's installations)
//   - Complete isolation via NP_LOCATION environment variable
//
// Why Nix? For tools with complex dependencies that can't be handled by:
//   - Direct binary downloads (need specific glibc versions)
//   - cargo/gem/pipx with zig fallback (__cpu_model compatibility issues)
//
// Binaries from Nix cannot be executed directly - they require nix-portable's
// virtualization layer to resolve /nix/store paths. Wrapper scripts invoke
// binaries through `nix shell --profile`.
type NixInstallAction struct{}

// Name returns the action name
func (a *NixInstallAction) Name() string {
	return "nix_install"
}

// Execute installs a package from Nixpkgs
//
// Parameters:
//   - package (required): Nixpkgs attribute path (e.g., "hello", "python3Packages.pytorch")
//   - executables (required): List of executable names to expose
//
// Directory Structure Created:
//
//	<install_dir>/
//	  .nix-profile/           - Nix profile with installed package
//	  bin/<executable>        - Wrapper scripts that invoke through nix-portable
func (a *NixInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Check platform first - nix-portable only supports Linux
	if runtime.GOOS != "linux" {
		return fmt.Errorf("nix_install action only supports Linux (nix-portable does not support %s)", runtime.GOOS)
	}

	// Get package name (required)
	packageName, ok := GetString(params, "package")
	if !ok {
		return fmt.Errorf("nix_install action requires 'package' parameter")
	}

	// SECURITY: Validate package name to prevent command injection
	if !isValidNixPackage(packageName) {
		return fmt.Errorf("invalid nixpkgs package name '%s': must match nixpkgs attribute path rules", packageName)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("nix_install action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names (same pattern as gem_install)
	for _, exe := range executables {
		if err := validateExecutableName(exe); err != nil {
			return err
		}
	}

	fmt.Printf("   Package: nixpkgs#%s\n", packageName)
	fmt.Printf("   Executables: %v\n", executables)

	// Check if nix-portable needs to be bootstrapped
	if ResolveNixPortable() == "" {
		fmt.Printf("\n   First-time nix setup required:\n")
		fmt.Printf("     - Download nix-portable (~75MB)\n")
		fmt.Printf("     - Bootstrap nix store (~200MB on first package)\n")
		fmt.Printf("   This is a one-time operation.\n\n")
	}

	// Ensure nix-portable is available
	nixPortablePath, err := EnsureNixPortable()
	if err != nil {
		return fmt.Errorf("failed to ensure nix-portable: %w", err)
	}

	fmt.Printf("   Using nix-portable: %s\n", nixPortablePath)

	// Get internal nix directory for NP_LOCATION
	npLocation, err := GetNixInternalDir()
	if err != nil {
		return err
	}

	// Create install directory structure
	installDir := ctx.InstallDir
	profilePath := filepath.Join(installDir, ".nix-profile")
	binDir := filepath.Join(installDir, "bin")

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Build nix profile install command
	// Using --profile to install to a specific profile location
	fmt.Printf("   Installing: nix profile install nixpkgs#%s\n", packageName)

	cmd := exec.Command(nixPortablePath, "nix", "profile", "install",
		"--profile", profilePath,
		fmt.Sprintf("nixpkgs#%s", packageName))

	// Set NP_LOCATION to ensure complete isolation
	cmd.Env = append(os.Environ(), fmt.Sprintf("NP_LOCATION=%s", npLocation))

	// Run installation
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nix profile install failed: %w\nOutput: %s", err, string(output))
	}

	// Show output in debug mode
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   nix output:\n%s\n", outputStr)
	}

	// Detect if proot is being used (performance warning)
	// nix-portable falls back to proot when user namespaces are unavailable
	if detectProotFallback(nixPortablePath, npLocation) {
		fmt.Println("")
		fmt.Println("   Warning: Using proot (user namespaces unavailable)")
		fmt.Println("     Execution may be 10-100x slower than normal.")
		fmt.Println("     Consider enabling user namespaces for better performance.")
		fmt.Println("")
	}

	// Create wrapper scripts for each executable
	for _, exe := range executables {
		if err := createNixWrapper(exe, binDir, npLocation, packageName); err != nil {
			return fmt.Errorf("failed to create wrapper for %s: %w", exe, err)
		}
	}

	// Verify wrappers work
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("wrapper for %s not found at %s", exe, exePath)
		}
	}

	fmt.Printf("   Installed successfully\n")
	fmt.Printf("   Created %d wrapper(s)\n", len(executables))

	return nil
}

// isValidNixPackage validates Nixpkgs attribute paths
// Valid: hello, python3Packages.pytorch, nodePackages.typescript
// Invalid: anything with shell metacharacters or path traversal
func isValidNixPackage(pkg string) bool {
	if pkg == "" || len(pkg) > 256 {
		return false
	}

	// Must start with alphanumeric
	first := pkg[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') ||
		(first >= '0' && first <= '9')) {
		return false
	}

	// Nixpkgs attributes: alphanumeric, dots (for nested attrs), hyphens, underscores
	// Block ALL shell metacharacters
	for _, c := range pkg {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}

	// Block directory traversal patterns
	if strings.Contains(pkg, "..") {
		return false
	}

	return true
}

// validateExecutableName validates executable names to prevent path traversal and injection
// Same pattern as gem_install.go
func validateExecutableName(exe string) error {
	if len(exe) == 0 || len(exe) > 256 {
		return fmt.Errorf("invalid executable name length: %s", exe)
	}
	if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
		strings.Contains(exe, "..") || exe == "." {
		return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
	}
	// Check for control characters and null bytes
	for _, c := range exe {
		if c < 32 || c == 127 || c == 0 {
			return fmt.Errorf("invalid executable name '%s': contains control characters", exe)
		}
	}
	// Check for shell metacharacters
	if strings.ContainsAny(exe, "$`|;&<>()[]{}") {
		return fmt.Errorf("invalid executable name '%s': contains shell metacharacters", exe)
	}
	return nil
}

// createNixWrapper creates a wrapper script for a nix-installed executable
// The wrapper invokes the binary through nix-portable's virtualization layer
// because Nix binaries have RPATH pointing to /nix/store which only exists
// inside the virtualization.
func createNixWrapper(exe, binDir, npLocation, packageName string) error {
	exePath := filepath.Join(binDir, exe)

	// Create wrapper that uses nix shell to invoke the executable
	// This is simpler and more reliable than trying to run profile binaries directly
	// The package is already cached in the nix store from the profile install
	wrapperContent := fmt.Sprintf(`#!/bin/bash
# tsuku wrapper for %s (nix_install)
# Uses nix shell to invoke through nix-portable's virtualization layer
# because Nix binaries have RPATH pointing to /nix/store

# Set NP_LOCATION for tsuku's isolated internal nix
export NP_LOCATION="%s"

# Invoke through nix shell (package is already cached from installation)
exec "%s/nix-portable" nix shell "nixpkgs#%s" -c %s "$@"
`, exe, npLocation, npLocation, packageName, exe)

	if err := os.WriteFile(exePath, []byte(wrapperContent), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}

	return nil
}

// detectProotFallback checks if nix-portable is using proot (slower fallback)
// This happens when user namespaces are disabled
func detectProotFallback(nixPortablePath, npLocation string) bool {
	// Run a simple nix command and check for proot indicators
	cmd := exec.Command(nixPortablePath, "nix", "--version")
	cmd.Env = append(os.Environ(), fmt.Sprintf("NP_LOCATION=%s", npLocation))

	// Check stderr for proot messages
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// nix-portable prints messages about runtime selection
	return strings.Contains(outputStr, "proot") ||
		strings.Contains(outputStr, "PROOT")
}
