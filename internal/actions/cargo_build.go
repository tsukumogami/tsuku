package actions

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CargoBuildAction builds Rust crates with deterministic configuration.
// This is an ecosystem primitive that cannot be decomposed further.
// It achieves determinism through cargo's --locked --offline flags and environment variables.
type CargoBuildAction struct{ BaseAction }

// Dependencies returns rust as an install-time dependency.
func (CargoBuildAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"rust"}}
}

// RequiresNetwork returns true because cargo_build fetches crates from crates.io.
func (CargoBuildAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *CargoBuildAction) Name() string {
	return "cargo_build"
}

// Execute builds a Rust crate with deterministic configuration
//
// Parameters:
//   - source_dir (required): Directory containing Cargo.toml
//   - executables (required): List of executable names to verify and install
//   - target (optional): Build target triple (defaults to host)
//   - features (optional): Cargo features to enable
//   - locked (optional): Use Cargo.lock for reproducibility (default: true)
//   - offline (optional): Build without network access after pre-fetch (default: true)
//   - no_default_features (optional): Disable default features (default: false)
//   - all_features (optional): Enable all features (default: false)
//   - rust_version (optional): Required Rust compiler version (e.g., "1.76.0")
//
// Deterministic Configuration:
//   - SOURCE_DATE_EPOCH: Set to Unix epoch (0) for reproducible embedded timestamps
//   - CARGO_INCREMENTAL=0: Disable incremental compilation for deterministic builds
//   - RUSTFLAGS="-C embed-bitcode=no": Smaller, more reproducible builds
//   - --locked: Require Cargo.lock to exist and be up-to-date
//   - --offline: Prevent network access during build (after pre-fetch)
//
// Security:
//   - Pre-fetches dependencies with cargo fetch --locked
//   - Builds with --offline to prevent network access (MITM protection)
//   - Uses isolated CARGO_HOME per build
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
func (a *CargoBuildAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Check for lock_data mode (decomposed cargo_install)
	lockData, hasLockData := GetString(params, "lock_data")
	if hasLockData && lockData != "" {
		return a.executeLockDataMode(ctx, params)
	}

	// Fall back to source_dir mode
	// Get source directory (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok || sourceDir == "" {
		return fmt.Errorf("cargo_build requires 'source_dir' parameter")
	}

	// Resolve source directory relative to work directory if not absolute
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify Cargo.toml exists
	cargoToml := filepath.Join(sourceDir, "Cargo.toml")
	if _, err := os.Stat(cargoToml); err != nil {
		return fmt.Errorf("Cargo.toml not found at %s: %w", cargoToml, err)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("cargo_build action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get optional parameters
	target, _ := GetString(params, "target")
	features, _ := GetStringSlice(params, "features")
	rustVersion, _ := GetString(params, "rust_version")

	// Boolean parameters with defaults (using GetBool for consistency)
	locked := true // Default to locked builds for reproducibility
	if val, ok := GetBool(params, "locked"); ok {
		locked = val
	}

	offline := true // Default to offline builds for security (prevents MITM)
	if val, ok := GetBool(params, "offline"); ok {
		offline = val
	}

	noDefaultFeatures := false
	if val, ok := GetBool(params, "no_default_features"); ok {
		noDefaultFeatures = val
	}

	allFeatures := false
	if val, ok := GetBool(params, "all_features"); ok {
		allFeatures = val
	}

	// Get cargo path
	cargoPath, _ := GetString(params, "cargo_path")
	if cargoPath == "" {
		cargoPath = ResolveCargo()
		if cargoPath == "" {
			cargoPath = "cargo"
		}
	}

	fmt.Printf("   Source: %s\n", sourceDir)
	fmt.Printf("   Executables: %v\n", executables)
	if target != "" {
		fmt.Printf("   Target: %s\n", target)
	}
	if len(features) > 0 {
		fmt.Printf("   Features: %v\n", features)
	}
	if noDefaultFeatures {
		fmt.Printf("   No default features: true\n")
	}
	if allFeatures {
		fmt.Printf("   All features: true\n")
	}
	fmt.Printf("   Locked: %v\n", locked)
	fmt.Printf("   Offline: %v\n", offline)
	fmt.Printf("   Using cargo: %s\n", cargoPath)

	// Set up deterministic environment with isolated CARGO_HOME
	env := buildDeterministicCargoEnv(cargoPath, ctx.WorkDir)

	// Validate Rust version if specified
	if rustVersion != "" {
		if err := validateRustVersion(ctx, cargoPath, rustVersion, env); err != nil {
			return err
		}
	}

	// Verify Cargo.lock exists when locked build is requested
	if locked {
		cargoLock := filepath.Join(sourceDir, "Cargo.lock")
		if _, err := os.Stat(cargoLock); err != nil {
			return fmt.Errorf("locked build requested but Cargo.lock not found in %s", sourceDir)
		}
	}

	// SECURITY: Validate target triple before any command execution
	if target != "" && !isValidTargetTriple(target) {
		return fmt.Errorf("invalid target triple '%s'", target)
	}

	// SECURITY: Validate feature names before any command execution
	for _, feature := range features {
		if !isValidFeatureName(feature) {
			return fmt.Errorf("invalid feature name '%s'", feature)
		}
	}

	// Pre-fetch dependencies if offline build is requested
	// This populates CARGO_HOME/registry with all required crates
	if offline && locked {
		fmt.Printf("   Pre-fetching dependencies...\n")
		fetchArgs := []string{"fetch", "--locked", "--manifest-path", filepath.Join(sourceDir, "Cargo.toml")}
		if target != "" {
			fetchArgs = append(fetchArgs, "--target", target)
		}

		fetchCmd := exec.CommandContext(ctx.Context, cargoPath, fetchArgs...)
		fetchCmd.Dir = sourceDir
		fetchCmd.Env = env
		fetchOutput, err := fetchCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cargo fetch failed: %w\nOutput: %s", err, string(fetchOutput))
		}
	}

	// Build arguments
	args := []string{"build", "--release"}

	if locked {
		args = append(args, "--locked")
	}

	if offline {
		args = append(args, "--offline")
	}

	if target != "" {
		args = append(args, "--target", target)
	}

	// Feature flags
	if noDefaultFeatures {
		args = append(args, "--no-default-features")
	}
	if allFeatures {
		args = append(args, "--all-features")
	}
	for _, feature := range features {
		args = append(args, "--features", feature)
	}

	fmt.Printf("   Building: cargo %s\n", strings.Join(args, " "))

	// Create bin directory in install dir
	binDir := filepath.Join(ctx.InstallDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Execute cargo build
	cmd := exec.CommandContext(ctx.Context, cargoPath, args...)
	cmd.Dir = sourceDir
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo build failed: %w\nOutput: %s", err, string(output))
	}

	// Show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   cargo output:\n%s\n", outputStr)
	}

	// Determine target directory for built binaries
	targetDir := filepath.Join(sourceDir, "target")
	if target != "" {
		targetDir = filepath.Join(targetDir, target)
	}
	releaseDir := filepath.Join(targetDir, "release")

	// Copy executables to install directory
	for _, exe := range executables {
		srcPath := filepath.Join(releaseDir, exe)
		dstPath := filepath.Join(binDir, exe)

		// Check if executable exists
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, srcPath)
		}

		// Copy the executable
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy executable %s: %w", exe, err)
		}

		// Make executable
		if err := os.Chmod(dstPath, 0755); err != nil {
			return fmt.Errorf("failed to chmod executable %s: %w", exe, err)
		}
	}

	fmt.Printf("   Crate built successfully\n")
	fmt.Printf("   Installed %d executable(s)\n", len(executables))

	return nil
}

// executeLockDataMode handles building from lock_data parameter.
// This is the mode used when cargo_install is decomposed.
func (a *CargoBuildAction) executeLockDataMode(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get crate name (required)
	crateName, ok := GetString(params, "crate")
	if !ok || crateName == "" {
		return fmt.Errorf("cargo_build lock_data mode requires 'crate' parameter")
	}

	// SECURITY: Validate crate name
	if !isValidCrateName(crateName) {
		return fmt.Errorf("invalid crate name '%s': must match crates.io naming rules", crateName)
	}

	// Get version (required)
	version, ok := GetString(params, "version")
	if !ok || version == "" {
		version = ctx.Version
	}
	if version == "" {
		return fmt.Errorf("cargo_build lock_data mode requires 'version' parameter")
	}

	// SECURITY: Validate version
	if !isValidCargoVersion(version) {
		return fmt.Errorf("invalid cargo version '%s'", version)
	}

	// Get lock_data (required - already validated in Execute)
	lockData, _ := GetString(params, "lock_data")

	// Get lock checksum (required for verification)
	lockChecksum, ok := GetString(params, "lock_checksum")
	if !ok || lockChecksum == "" {
		return fmt.Errorf("cargo_build lock_data mode requires 'lock_checksum' parameter")
	}

	// Get executables (required for verification)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("cargo_build lock_data mode requires 'executables' parameter")
	}

	// SECURITY: Validate executable names
	for _, exe := range executables {
		if len(exe) == 0 || len(exe) > 256 {
			return fmt.Errorf("invalid executable name length: %s", exe)
		}
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get optional parameters
	rustVersion, _ := GetString(params, "rust_version")

	fmt.Printf("   Crate: %s@%s\n", crateName, version)
	fmt.Printf("   Executables: %v\n", executables)

	// Get cargo path
	cargoPath := ResolveCargo()
	if cargoPath == "" {
		cargoPath = "cargo"
	}

	// Validate Rust version if specified
	if rustVersion != "" {
		// Build temporary environment for version check
		tempEnv := buildDeterministicCargoEnv(cargoPath, ctx.WorkDir)
		if err := validateRustVersion(ctx, cargoPath, rustVersion, tempEnv); err != nil {
			fmt.Printf("   Warning: Rust version validation failed: %v\n", err)
		}
	}

	fmt.Printf("   Using cargo: %s\n", cargoPath)

	// Create temporary workspace for building
	tempDir, err := os.MkdirTemp("", "tsuku-cargo-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create src directory and minimal main.rs (required for valid Cargo package)
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}

	mainRsPath := filepath.Join(srcDir, "main.rs")
	mainRsContent := "fn main() {}\n"
	if err := os.WriteFile(mainRsPath, []byte(mainRsContent), 0644); err != nil {
		return fmt.Errorf("failed to write main.rs: %w", err)
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
		return fmt.Errorf("failed to write Cargo.toml: %w", err)
	}

	// Write Cargo.lock
	lockPath := filepath.Join(tempDir, "Cargo.lock")
	if err := os.WriteFile(lockPath, []byte(lockData), 0644); err != nil {
		return fmt.Errorf("failed to write Cargo.lock: %w", err)
	}

	// Verify Cargo.lock checksum
	computedChecksum := fmt.Sprintf("%x", sha256.Sum256([]byte(lockData)))
	if computedChecksum != lockChecksum {
		return fmt.Errorf("Cargo.lock checksum mismatch\n  Expected: %s\n  Got:      %s\n\nThis may indicate plan file tampering",
			lockChecksum, computedChecksum)
	}

	fmt.Printf("   Building crate with lockfile enforcement\n")

	// Build deterministic environment
	env := buildDeterministicCargoEnv(cargoPath, tempDir)

	// Pre-fetch dependencies to populate CARGO_HOME
	fmt.Printf("   Pre-fetching dependencies...\n")
	fetchArgs := []string{"fetch", "--locked", "--manifest-path", cargoTomlPath}
	fetchCmd := exec.CommandContext(ctx.Context, cargoPath, fetchArgs...)
	fetchCmd.Dir = tempDir
	fetchCmd.Env = env
	fetchOutput, err := fetchCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo fetch failed: %w\nOutput: %s", err, string(fetchOutput))
	}

	// Build with --locked --offline
	buildArgs := []string{"build", "--release", "--locked", "--offline", "--manifest-path", cargoTomlPath}
	fmt.Printf("   Running: cargo %s\n", strings.Join(buildArgs, " "))

	buildCmd := exec.CommandContext(ctx.Context, cargoPath, buildArgs...)
	buildCmd.Dir = tempDir
	buildCmd.Env = env

	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cargo build failed: %w\nOutput: %s", err, string(buildOutput))
	}

	// Show output if debugging
	outputStr := strings.TrimSpace(string(buildOutput))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   cargo output:\n%s\n", outputStr)
	}

	// Find built executables in target/release
	releaseDir := filepath.Join(tempDir, "target", "release")

	// Create bin directory in install dir
	binDir := filepath.Join(ctx.InstallDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Copy executables to install directory
	for _, exe := range executables {
		srcPath := filepath.Join(releaseDir, exe)
		dstPath := filepath.Join(binDir, exe)

		// Check if executable exists
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, srcPath)
		}

		// Copy the executable
		if err := copyFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to copy executable %s: %w", exe, err)
		}

		// Make executable
		if err := os.Chmod(dstPath, 0755); err != nil {
			return fmt.Errorf("failed to chmod executable %s: %w", exe, err)
		}
	}

	fmt.Printf("   Crate built successfully\n")
	fmt.Printf("   Verified %d executable(s)\n", len(executables))

	return nil
}

// buildDeterministicCargoEnv creates an environment with deterministic build settings.
// workDir is used to create an isolated CARGO_HOME for reproducible builds.
func buildDeterministicCargoEnv(cargoPath, workDir string) []string {
	cargoDir := filepath.Dir(cargoPath)
	env := os.Environ()

	// Filter existing variables that might affect determinism
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		// Keep most variables but filter some that could cause non-determinism
		if !strings.HasPrefix(e, "CARGO_INCREMENTAL=") &&
			!strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") &&
			!strings.HasPrefix(e, "CARGO_HOME=") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Add cargo's bin directory to PATH
	pathUpdated := false
	for i, e := range filteredEnv {
		if strings.HasPrefix(e, "PATH=") {
			filteredEnv[i] = fmt.Sprintf("PATH=%s:%s", cargoDir, e[5:])
			pathUpdated = true
			break
		}
	}
	if !pathUpdated {
		filteredEnv = append(filteredEnv, fmt.Sprintf("PATH=%s:%s", cargoDir, os.Getenv("PATH")))
	}

	// Set isolated CARGO_HOME to prevent cross-contamination between builds
	cargoHome := filepath.Join(workDir, ".cargo-home")
	filteredEnv = append(filteredEnv, "CARGO_HOME="+cargoHome)

	// Set deterministic build environment variables
	filteredEnv = append(filteredEnv,
		// Disable incremental compilation for deterministic builds
		"CARGO_INCREMENTAL=0",
		// Set SOURCE_DATE_EPOCH to Unix epoch (0) for reproducible timestamps
		"SOURCE_DATE_EPOCH=0",
	)

	// Set RUSTFLAGS for more reproducible builds
	// -C embed-bitcode=no: Don't embed LLVM bitcode (smaller, more reproducible)
	existingRustflags := ""
	for _, e := range filteredEnv {
		if strings.HasPrefix(e, "RUSTFLAGS=") {
			existingRustflags = e[10:]
			break
		}
	}

	rustflags := "-C embed-bitcode=no"
	if existingRustflags != "" {
		rustflags = existingRustflags + " " + rustflags
	}

	// Update or add RUSTFLAGS
	rustflagsSet := false
	for i, e := range filteredEnv {
		if strings.HasPrefix(e, "RUSTFLAGS=") {
			filteredEnv[i] = "RUSTFLAGS=" + rustflags
			rustflagsSet = true
			break
		}
	}
	if !rustflagsSet {
		filteredEnv = append(filteredEnv, "RUSTFLAGS="+rustflags)
	}

	// Set up C compiler for crates with native dependencies
	if !hasSystemCompiler() {
		if newEnv, found := SetupCCompilerEnv(filteredEnv); found {
			filteredEnv = newEnv
		}
	}

	return filteredEnv
}

// isValidTargetTriple validates Rust target triples
// Format: <arch><sub>-<vendor>-<sys>-<abi>
// Examples: x86_64-unknown-linux-gnu, aarch64-apple-darwin
func isValidTargetTriple(target string) bool {
	if target == "" || len(target) > 100 {
		return false
	}

	// Must contain at least two hyphens (arch-vendor-sys or arch-vendor-sys-abi)
	parts := strings.Split(target, "-")
	if len(parts) < 3 {
		return false
	}

	// Check allowed characters: alphanumeric and hyphens
	for _, c := range target {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

// isValidFeatureName validates Cargo feature names
// Features can be alphanumeric with hyphens, underscores, and slashes (for namespaced features)
func isValidFeatureName(feature string) bool {
	if feature == "" || len(feature) > 100 {
		return false
	}

	// Security: reject path traversal and absolute paths
	if strings.Contains(feature, "..") || strings.HasPrefix(feature, "/") {
		return false
	}

	// Limit slash depth to prevent abuse (namespaced features typically have one slash)
	slashCount := strings.Count(feature, "/")
	if slashCount > 2 {
		return false
	}

	for _, c := range feature {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '/') {
			return false
		}
	}

	return true
}

// validateRustVersion verifies the installed Rust compiler matches the required version.
// Version format: major.minor.patch (e.g., "1.76.0")
func validateRustVersion(ctx *ExecutionContext, cargoPath, requiredVersion string, env []string) error {
	// Get rustc path from same directory as cargo
	rustcPath := filepath.Join(filepath.Dir(cargoPath), "rustc")
	if _, err := os.Stat(rustcPath); err != nil {
		// Fall back to PATH lookup
		rustcPath = "rustc"
	}

	cmd := exec.CommandContext(ctx.Context, rustcPath, "--version")
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get rustc version: %w", err)
	}

	// Parse version from output like "rustc 1.76.0 (07dca489a 2024-02-04)"
	versionOutput := strings.TrimSpace(string(output))
	parts := strings.Fields(versionOutput)
	if len(parts) < 2 {
		return fmt.Errorf("unexpected rustc version output: %s", versionOutput)
	}

	installedVersion := parts[1]

	// Check if installed version matches required version
	if !strings.HasPrefix(installedVersion, requiredVersion) {
		return fmt.Errorf("Rust compiler version mismatch\n  Required: rustc %s\n  Found:    rustc %s\n\n  Install the required version:\n    rustup install %s\n    rustup default %s",
			requiredVersion, installedVersion, requiredVersion, requiredVersion)
	}

	fmt.Printf("   Rust version: %s (matches required %s)\n", installedVersion, requiredVersion)
	return nil
}
