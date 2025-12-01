package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CpanInstallAction installs Perl distributions with local::lib isolation
type CpanInstallAction struct{}

// Name returns the action name
func (a *CpanInstallAction) Name() string {
	return "cpan_install"
}

// Execute installs a CPAN distribution to the install directory
//
// Parameters:
//   - distribution (required): CPAN distribution name (e.g., "App-Ack")
//   - executables (required): List of executable names to verify
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

	fmt.Printf("   Distribution: %s@%s\n", distribution, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using perl: %s\n", perlPath)
	fmt.Printf("   Using cpanm: %s\n", cpanmPath)

	installDir := ctx.InstallDir

	// Build install target
	target := distribution
	if ctx.Version != "" {
		target = distribution + "@" + ctx.Version
	}

	fmt.Printf("   Installing: cpanm --local-lib %s --notest %s\n", installDir, target)

	// Build command: cpanm --local-lib <dir> --notest <distribution>@<version>
	cmd := exec.CommandContext(ctx.Context, cpanmPath,
		"--local-lib", installDir,
		"--notest", // Skip tests for faster installation
		target,
	)

	// SECURITY: Clear ALL PERL* environment variables to prevent contamination
	// This ensures isolation from any system Perl configuration
	cleanEnv := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PERL") {
			cleanEnv = append(cleanEnv, e)
		}
	}

	// Add perl's directory to PATH so cpanm can find the right perl
	perlDir := filepath.Dir(perlPath)
	pathUpdated := false
	for i, e := range cleanEnv {
		if strings.HasPrefix(e, "PATH=") {
			cleanEnv[i] = fmt.Sprintf("PATH=%s:%s", perlDir, e[5:])
			pathUpdated = true
			break
		}
	}
	if !pathUpdated {
		cleanEnv = append(cleanEnv, fmt.Sprintf("PATH=%s:%s", perlDir, os.Getenv("PATH")))
	}

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
