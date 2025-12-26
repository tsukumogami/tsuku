package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CpanInstallAction is an ecosystem primitive that installs Perl distributions
// with deterministic configuration. It achieves determinism through environment
// isolation, version validation, and optional offline/mirror-only modes.
type CpanInstallAction struct{ BaseAction }

// Dependencies returns perl as both install-time and runtime dependency.
func (CpanInstallAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"perl"}, Runtime: []string{"perl"}}
}

// RequiresNetwork returns true because cpan_install fetches distributions from CPAN.
func (CpanInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *CpanInstallAction) Name() string {
	return "cpan_install"
}

// Preflight validates parameters without side effects.
func (a *CpanInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "distribution"); !ok {
		result.AddError("cpan_install action requires 'distribution' parameter")
	}
	if _, ok := GetStringSlice(params, "executables"); !ok {
		result.AddError("cpan_install action requires 'executables' parameter")
	}
	return result
}

// Execute installs a CPAN distribution to the install directory
//
// Parameters:
//   - distribution (required): CPAN distribution name (e.g., "App-Ack")
//   - module (optional): Module name to install (e.g., "App::Ack")
//     When provided, this is used directly with cpanm instead of converting
//     distribution name. Use this when the distribution name doesn't follow
//     standard naming convention (e.g., "ack" distribution contains "App::Ack").
//   - executables (required): List of executable names to verify
//   - perl_version (optional): Required Perl version (e.g., "5.38.0")
//   - cpanfile (optional): Path to cpanfile for dependency installation
//   - mirror (optional): CPAN mirror URL (e.g., "https://cpan.metacpan.org/")
//   - mirror_only (optional): Only use specified mirror, no fallback (default: false)
//   - offline (optional): Build without network after pre-fetch (default: false)
//
// Deterministic Configuration:
//   - SOURCE_DATE_EPOCH: Set to 0 for reproducible embedded timestamps
//   - All PERL* env vars cleared during installation
//   - --mirror-only: When enabled, prevents fallback to other CPAN sources
//
// Environment Strategy:
//
//	PERL5LIB is set at runtime via wrapper scripts
//	All PERL* env vars are cleared during cpanm execution
//
// Installation:
//
//	cpanm --local-lib <install_dir> --notest <distribution>@<version>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>           - Wrapper scripts
//	  bin/<executable>.cpanm     - Original cpanm scripts
//	  lib/perl5/                 - Installed modules
//	  man/                       - Man pages
func (a *CpanInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get distribution name (required)
	distribution, ok := GetString(params, "distribution")
	if !ok {
		return fmt.Errorf("cpan_install action requires 'distribution' parameter")
	}

	// SECURITY: Validate distribution name to prevent command injection
	if !isValidDistribution(distribution) {
		return fmt.Errorf("invalid distribution name '%s': must match CPAN naming rules (letters, numbers, hyphens)", distribution)
	}

	// Get optional module name (for non-standard distributions like "ack" -> "App::Ack")
	moduleName, hasModule := GetString(params, "module")
	if hasModule {
		// SECURITY: Validate module name to prevent command injection
		if !isValidModuleName(moduleName) {
			return fmt.Errorf("invalid module name '%s': must match CPAN module naming rules", moduleName)
		}
	}

	// SECURITY: Validate version string
	if !isValidCpanVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match CPAN version format", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("cpan_install action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal and injection
	for _, exe := range executables {
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
	}

	// Get optional parameters
	perlVersion, _ := GetString(params, "perl_version")
	cpanfile, _ := GetString(params, "cpanfile")
	mirror, _ := GetString(params, "mirror")

	// Boolean parameters with defaults (using GetBool for consistency with other primitives)
	mirrorOnly := false
	if val, ok := GetBool(params, "mirror_only"); ok {
		mirrorOnly = val
	}

	offline := false
	if val, ok := GetBool(params, "offline"); ok {
		offline = val
	}

	// Verify /bin/bash exists before creating wrappers
	if _, err := os.Stat("/bin/bash"); os.IsNotExist(err) {
		return fmt.Errorf("/bin/bash not found: required for wrapper scripts")
	}

	// Find perl and cpanm from tsuku's tools directory
	perlPath := ResolvePerl()
	if perlPath == "" {
		return fmt.Errorf("perl not found: install perl first (tsuku install perl)")
	}

	cpanmPath := ResolveCpanm()
	if cpanmPath == "" {
		return fmt.Errorf("cpanm not found: install perl first (tsuku install perl)")
	}

	// Validate Perl version if specified
	if perlVersion != "" {
		if err := validatePerlVersion(ctx, perlPath, perlVersion); err != nil {
			return err
		}
	}

	fmt.Printf("   Distribution: %s@%s\n", distribution, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	if mirror != "" {
		fmt.Printf("   Mirror: %s\n", mirror)
	}
	if mirrorOnly {
		fmt.Printf("   Mirror only: true\n")
	}
	if offline {
		fmt.Printf("   Offline: true\n")
	}
	fmt.Printf("   Using perl: %s\n", perlPath)
	fmt.Printf("   Using cpanm: %s\n", cpanmPath)

	installDir := ctx.InstallDir

	// Build install target
	// Use explicit module name if provided, otherwise convert distribution name
	// cpanm only understands module names or full tarball paths
	if !hasModule {
		moduleName = distributionToModule(distribution)
	}
	target := moduleName
	if ctx.Version != "" {
		target = moduleName + "@" + ctx.Version
	}

	// Build cpanm arguments
	args := []string{
		"--local-lib", installDir,
		"--notest", // Skip tests for faster installation
	}

	// Add mirror options for deterministic builds
	if mirror != "" {
		args = append(args, "--mirror", mirror)
	}
	if mirrorOnly {
		args = append(args, "--mirror-only")
	}

	// Handle cpanfile vs module installation
	if cpanfile != "" {
		// Resolve cpanfile path relative to work directory if not absolute
		if !filepath.IsAbs(cpanfile) {
			cpanfile = filepath.Join(ctx.WorkDir, cpanfile)
		}
		// Verify cpanfile exists
		if _, err := os.Stat(cpanfile); err != nil {
			return fmt.Errorf("cpanfile not found: %s", cpanfile)
		}
		args = append(args, "--installdeps", filepath.Dir(cpanfile))
		fmt.Printf("   Installing dependencies from: %s\n", cpanfile)
	} else {
		args = append(args, target)
		fmt.Printf("   Installing: cpanm %s\n", strings.Join(args, " "))
	}

	// Build command
	cmd := exec.CommandContext(ctx.Context, cpanmPath, args...)

	// Get perl directory for wrapper scripts and PATH
	perlDir := filepath.Dir(perlPath)

	// Set up deterministic environment
	cleanEnv := buildDeterministicPerlEnv(perlPath, offline)

	cmd.Env = cleanEnv

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cpanm install failed: %w\nOutput: %s", err, string(output))
	}

	// cpanm is verbose, only show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   cpanm output:\n%s\n", outputStr)
	}

	// Verify executables exist and create self-contained wrappers
	// The cpanm-generated scripts need PERL5LIB at runtime,
	// so we create wrapper scripts that set this before calling the original
	binDir := filepath.Join(installDir, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}

		// Rename original cpanm-generated script to .cpanm suffix
		cpanmWrapperPath := exePath + ".cpanm"
		if err := os.Rename(exePath, cpanmWrapperPath); err != nil {
			return fmt.Errorf("failed to rename cpanm script: %w", err)
		}

		// Create new wrapper that sets PERL5LIB and adds Perl to PATH
		// Uses SCRIPT_DIR to make wrapper relocatable (works after install dir is moved)
		wrapperContent := fmt.Sprintf(`#!/bin/bash
# tsuku wrapper for %s (sets PERL5LIB for isolated cpan distribution)
SCRIPT_PATH="${BASH_SOURCE[0]}"
# Resolve symlinks to get the actual script location
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"

export PERL5LIB="$INSTALL_DIR/lib/perl5"
export PATH="%s:$PATH"
exec perl "$SCRIPT_DIR/%s.cpanm" "$@"
`, exe, perlDir, exe)

		if err := os.WriteFile(exePath, []byte(wrapperContent), 0755); err != nil {
			// Restore original on failure
			_ = os.Rename(cpanmWrapperPath, exePath)
			return fmt.Errorf("failed to create wrapper script: %w", err)
		}

		// Log debug info if enabled
		if os.Getenv("TSUKU_DEBUG") != "" {
			fmt.Printf("   Created wrapper for %s (original at %s)\n", exe, cpanmWrapperPath)
		}
	}

	fmt.Printf("   Installed successfully\n")
	fmt.Printf("   Created %d self-contained wrapper(s)\n", len(executables))

	return nil
}

// isValidDistribution validates CPAN distribution names to prevent command injection
// Distribution names: start with letter, alphanumeric + hyphens + underscores
// Max length: 128 characters (reasonable limit for CPAN)
// Rejects module names containing "::" - those need conversion first
func isValidDistribution(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Reject module names (contain ::)
	if strings.Contains(name, "::") {
		return false
	}

	// Must start with a letter
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}

	// Check allowed characters: alphanumeric, hyphens, underscores
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

// isValidModuleName validates CPAN module names to prevent command injection
// Module names: alphanumeric + underscores, separated by ::
// Max length: 128 characters (reasonable limit for CPAN)
// Examples: App::Ack, Perl::Critic, File::Slurp::Tiny
func isValidModuleName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Split by :: and validate each part
	parts := strings.Split(name, "::")
	for _, part := range parts {
		if part == "" {
			return false // Empty part (e.g., "Foo::" or "::Bar" or "Foo::::Bar")
		}
		// Each part must start with a letter
		first := part[0]
		if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
			return false
		}
		// Check allowed characters: alphanumeric and underscores
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}

	return true
}

// distributionToModule converts a CPAN distribution name to a module name
// Distribution names use hyphens (Perl-Critic), module names use :: (Perl::Critic)
// cpanm only understands module names, not distribution names
func distributionToModule(distribution string) string {
	return strings.ReplaceAll(distribution, "-", "::")
}

// isValidCpanVersion validates CPAN version strings
// Valid: 1.0.0, 1.2.3_01, 1.2.3-TRIAL, v1.2.3
// Invalid: anything with shell metacharacters
func isValidCpanVersion(version string) bool {
	if version == "" {
		// Empty version is valid (means latest)
		return true
	}

	if len(version) > 50 {
		return false
	}

	// CPAN versions can start with 'v' or digit
	first := version[0]
	if first != 'v' && (first < '0' || first > '9') {
		return false
	}

	// Allow CPAN version characters: digits, dots, letters (for -TRIAL, _01), hyphens, underscores
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

// buildDeterministicPerlEnv creates an environment with deterministic build settings.
// Clears all PERL* variables and sets SOURCE_DATE_EPOCH for reproducibility.
func buildDeterministicPerlEnv(perlPath string, offline bool) []string {
	env := os.Environ()

	// Filter out PERL* and SOURCE_DATE_EPOCH variables
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PERL") && !strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Add perl's directory to PATH
	perlDir := filepath.Dir(perlPath)
	pathUpdated := false
	for i, e := range filteredEnv {
		if strings.HasPrefix(e, "PATH=") {
			filteredEnv[i] = fmt.Sprintf("PATH=%s:%s", perlDir, e[5:])
			pathUpdated = true
			break
		}
	}
	if !pathUpdated {
		filteredEnv = append(filteredEnv, fmt.Sprintf("PATH=%s:%s", perlDir, os.Getenv("PATH")))
	}

	// Set SOURCE_DATE_EPOCH for reproducible timestamps (Unix epoch 0)
	filteredEnv = append(filteredEnv, "SOURCE_DATE_EPOCH=0")

	// If offline mode, set environment to prevent network access
	// cpanm respects HTTP_PROXY/HTTPS_PROXY - we could block these, but
	// --mirror-only with a local mirror is the recommended approach
	_ = offline // Currently informational; --mirror-only handles isolation

	return filteredEnv
}

// validatePerlVersion verifies the installed Perl version matches the required version.
// Version format: major.minor.patch (e.g., "5.38.0") or just major.minor (e.g., "5.38")
func validatePerlVersion(ctx *ExecutionContext, perlPath, requiredVersion string) error {
	cmd := exec.CommandContext(ctx.Context, perlPath, "-v")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get Perl version: %w", err)
	}

	// Parse version from output like:
	// "This is perl 5, version 38, subversion 0 (v5.38.0) built for x86_64-linux"
	// or "perl 5.38.0"
	versionOutput := string(output)

	// Try to find v5.XX.X pattern first
	var installedVersion string
	if idx := strings.Index(versionOutput, "(v"); idx != -1 {
		end := strings.Index(versionOutput[idx:], ")")
		if end != -1 {
			installedVersion = versionOutput[idx+2 : idx+end]
		}
	}

	// Fallback: look for "perl X.X.X" or "version X"
	if installedVersion == "" {
		parts := strings.Fields(versionOutput)
		for i, p := range parts {
			if p == "version" && i+1 < len(parts) {
				// Extract from "version X, subversion Y"
				// This is tricky, try the (vX.X.X) approach above first
				break
			}
		}
	}

	if installedVersion == "" {
		return fmt.Errorf("unable to parse Perl version from: %s", versionOutput[:min(100, len(versionOutput))])
	}

	// Check if installed version matches required version (prefix match)
	if !strings.HasPrefix(installedVersion, requiredVersion) {
		return fmt.Errorf("Perl version mismatch\n  Required: %s\n  Found:    %s\n\n  Install the required version:\n    tsuku install perl@%s",
			requiredVersion, installedVersion, requiredVersion)
	}

	fmt.Printf("   Perl version: %s (matches required %s)\n", installedVersion, requiredVersion)
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
