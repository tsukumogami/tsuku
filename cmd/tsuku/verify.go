package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/verify"
)

// Library verification flags
var (
	verifyIntegrityFlag  bool // --integrity: enable checksum verification for libraries
	verifySkipDlopenFlag bool // --skip-dlopen: skip load testing for libraries
)

// LibraryVerifyOptions controls library verification behavior
type LibraryVerifyOptions struct {
	CheckIntegrity           bool // Enable Level 4: checksum verification
	SkipDlopen               bool // Disable Level 3: dlopen load testing
	SkipDependencyValidation bool // Disable Level 2: dependency validation
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyIntegrityFlag, "integrity", false, "Enable checksum verification for libraries")
	verifyCmd.Flags().BoolVar(&verifySkipDlopenFlag, "skip-dlopen", false, "Skip dlopen load testing for libraries")
}

// displayDependencyResults formats and prints dependency validation results.
// Returns true if all dependencies passed, false if any failed.
func displayDependencyResults(results []verify.DepResult) bool {
	if len(results) == 0 {
		printInfo("    No dynamic dependencies (statically linked)\n")
		return true
	}

	allPassed := true
	for _, r := range results {
		var status string
		switch r.Category {
		case verify.DepTsukuManaged:
			if r.Status == verify.ValidationPass {
				status = fmt.Sprintf("OK (tsuku:%s@%s)", r.Recipe, r.Version)
			} else {
				status = fmt.Sprintf("FAIL (tsuku:%s@%s) - %s", r.Recipe, r.Version, r.Error)
				allPassed = false
			}
		case verify.DepExternallyManaged:
			if r.Status == verify.ValidationPass {
				status = fmt.Sprintf("OK (tsuku:%s@%s, external)", r.Recipe, r.Version)
			} else {
				status = fmt.Sprintf("FAIL (tsuku:%s@%s, external) - %s", r.Recipe, r.Version, r.Error)
				allPassed = false
			}
		case verify.DepPureSystem:
			if r.Status == verify.ValidationPass {
				status = "OK (system)"
			} else {
				status = fmt.Sprintf("FAIL (system) - %s", r.Error)
				allPassed = false
			}
		default:
			status = fmt.Sprintf("UNKNOWN - %s", r.Error)
			allPassed = false
		}
		printInfof("    %s: %s\n", r.Soname, status)

		// Display transitive dependencies (indented further)
		for _, t := range r.Transitive {
			var tStatus string
			switch t.Category {
			case verify.DepTsukuManaged:
				if t.Status == verify.ValidationPass {
					tStatus = fmt.Sprintf("OK (tsuku:%s@%s)", t.Recipe, t.Version)
				} else {
					tStatus = fmt.Sprintf("FAIL - %s", t.Error)
					allPassed = false
				}
			case verify.DepPureSystem:
				if t.Status == verify.ValidationPass {
					tStatus = "OK (system)"
				} else {
					tStatus = fmt.Sprintf("FAIL - %s", t.Error)
					allPassed = false
				}
			default:
				tStatus = fmt.Sprintf("UNKNOWN - %s", t.Error)
				allPassed = false
			}
			printInfof("      -> %s: %s\n", t.Soname, tStatus)
		}
	}

	printInfof("  Tier 2: %d dependencies validated\n", len(results))
	return allPassed
}

// findToolBinaries returns absolute paths to binary files for a tool.
// It looks in the bin/ subdirectory of the install directory.
func findToolBinaries(installDir string, binaries []string, toolName string) []string {
	// If no binaries recorded, fall back to tool name
	if len(binaries) == 0 {
		binaries = []string{toolName}
	}

	var paths []string
	binDir := filepath.Join(installDir, "bin")

	for _, binary := range binaries {
		// Handle both bare names and paths like "cargo/bin/cargo"
		name := filepath.Base(binary)
		path := filepath.Join(binDir, name)

		// Check if file exists
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}

	return paths
}

// truncateChecksum returns the first 12 characters of a checksum for display.
func truncateChecksum(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// ToolVerifyOptions controls tool verification behavior
type ToolVerifyOptions struct {
	// Verbose enables detailed output during verification
	Verbose bool
	// SkipPATHChecks skips Steps 2-3 (PATH environment checks) for visible tools.
	// Use this in post-install context where the user may not have added tsuku to PATH yet.
	SkipPATHChecks bool
	// SkipDependencyValidation skips Tier 2 dependency validation for tools.
	// Use this in post-install context where dependency classification may have false positives.
	SkipDependencyValidation bool
}

// RunToolVerification performs verification for an installed tool.
// Returns nil on success, error on failure.
// This function is designed to be called from both the verify command and install flow.
func RunToolVerification(r *recipe.Recipe, toolName string, toolState *install.ToolState, cfg *config.Config, state *install.State, opts ToolVerifyOptions) error {
	version := toolState.Version
	if toolState.ActiveVersion != "" {
		version = toolState.ActiveVersion
	}

	installDir := filepath.Join(cfg.ToolsDir, fmt.Sprintf("%s-%s", toolName, version))

	// Get version state for integrity verification
	var versionState *install.VersionState
	if toolState.Versions != nil {
		if vs, ok := toolState.Versions[version]; ok {
			versionState = &vs
		}
	}
	if versionState == nil {
		// Fallback for legacy state without multi-version support
		versionState = &install.VersionState{
			Binaries: toolState.Binaries,
		}
	}

	if opts.Verbose {
		printInfof("Verifying %s (version %s)...\n", toolName, version)
	}

	// Determine verification strategy based on tool visibility
	if toolState.IsHidden {
		return runHiddenToolVerification(r, toolName, version, installDir, versionState, cfg, state, opts)
	}
	return runVisibleToolVerification(r, toolName, toolState, versionState, installDir, cfg, state, opts)
}

// makeVerifyEnv creates an environment for verification commands with proper PATH setup.
// It filters out existing PATH entries and prepends the install directories to ensure
// the installed tool's binaries are found before system binaries.
func makeVerifyEnv(installDir string, cfg *config.Config) []string {
	// Filter out existing PATH to avoid duplicate entries
	env := make([]string, 0)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PATH=") {
			env = append(env, e)
		}
	}

	// Build PATH with all possible bin directories:
	// 1. cfg.CurrentDir - symlinks to current tool versions
	// 2. installDir/bin - standard bin directory
	// 3. installDir/.gem/bin - gem-installed executables (via install_gem_direct)
	// 4. Original PATH
	binDir := filepath.Join(installDir, "bin")
	gemBinDir := filepath.Join(installDir, ".gem", "bin")
	pathValue := cfg.CurrentDir + ":" + binDir + ":" + gemBinDir + ":" + os.Getenv("PATH")
	env = append(env, "PATH="+pathValue)

	return env
}

// runHiddenToolVerification verifies a hidden tool using absolute paths.
// Returns nil on success, error on failure.
func runHiddenToolVerification(r *recipe.Recipe, toolName, version, installDir string, versionState *install.VersionState, cfg *config.Config, state *install.State, opts ToolVerifyOptions) error {
	command := r.Verify.Command
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)

	pattern := r.Verify.Pattern
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	if opts.Verbose {
		printInfof("  Running: %s\n", command)
	}

	cmdExec := exec.Command("sh", "-c", command)
	cmdExec.Env = makeVerifyEnv(installDir, cfg)

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verification command failed: %w\nOutput: %s", err, string(output))
	}

	outputStr := strings.TrimSpace(string(output))
	if opts.Verbose {
		printInfof("  Output: %s\n", outputStr)
	}

	if pattern != "" {
		if !strings.Contains(outputStr, pattern) {
			return fmt.Errorf("output does not match expected pattern\n  Expected: %s\n  Got: %s", pattern, outputStr)
		}
		if opts.Verbose {
			printInfof("  Pattern matched: %s\n", pattern)
		}
	}

	// Binary integrity verification
	if err := verifyBinaryIntegrityInternal(installDir, versionState, opts.Verbose); err != nil {
		return err
	}

	// Tier 2: Dependency validation (skipped in post-install context)
	if !opts.SkipDependencyValidation {
		if opts.Verbose {
			printInfo("  Tier 2: Validating dependencies...\n")
		}
		binaries := findToolBinaries(installDir, nil, toolName)
		if len(binaries) == 0 {
			// Try the tool name directly in install dir
			binPath := filepath.Join(installDir, "bin", toolName)
			if _, err := os.Stat(binPath); err == nil {
				binaries = []string{binPath}
			}
		}

		var allResults []verify.DepResult
		for _, binPath := range binaries {
			results, err := verify.ValidateDependenciesSimple(binPath, state, cfg.HomeDir)
			if err != nil {
				return fmt.Errorf("dependency validation failed for %s: %w", filepath.Base(binPath), err)
			}
			allResults = append(allResults, results...)
		}

		if !checkDependencyResults(allResults, opts.Verbose) {
			return fmt.Errorf("dependency validation failed")
		}
	} else if opts.Verbose {
		printInfo("  Tier 2: Skipping dependency validation (post-install verification)\n")
	}

	return nil
}

// runVisibleToolVerification performs comprehensive verification for visible tools.
// Returns nil on success, error on failure.
func runVisibleToolVerification(r *recipe.Recipe, toolName string, toolState *install.ToolState, versionState *install.VersionState, installDir string, cfg *config.Config, state *install.State, opts ToolVerifyOptions) error {
	// Step 1: Verify installation via current/ symlink
	if opts.Verbose {
		printInfo("  Step 1: Verifying installation via symlink...")
	}

	command := r.Verify.Command
	pattern := r.Verify.Pattern

	version := toolState.Version
	if toolState.ActiveVersion != "" {
		version = toolState.ActiveVersion
	}
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	if opts.Verbose {
		printInfof("    Running: %s\n", command)
	}
	cmdExec := exec.Command("sh", "-c", command)
	cmdExec.Env = makeVerifyEnv(installDir, cfg)

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installation verification failed: %w\nOutput: %s", err, string(output))
	}
	outputStr := strings.TrimSpace(string(output))
	if opts.Verbose {
		printInfof("    Output: %s\n", outputStr)
	}

	if pattern != "" && !strings.Contains(outputStr, pattern) {
		return fmt.Errorf("pattern mismatch\n  Expected: %s\n  Got: %s", pattern, outputStr)
	}
	if opts.Verbose {
		printInfo("    Installation verified\n")
	}

	// Steps 2-3: PATH environment checks (skipped in post-install context)
	if !opts.SkipPATHChecks {
		// Step 2: Check if current/ is in PATH
		if opts.Verbose {
			printInfof("  Step 2: Checking if %s is in PATH...\n", cfg.CurrentDir)
		}
		pathEnv := os.Getenv("PATH")
		pathInPATH := false
		for _, dir := range strings.Split(pathEnv, ":") {
			if dir == cfg.CurrentDir {
				pathInPATH = true
				break
			}
		}

		if !pathInPATH {
			return fmt.Errorf("%s is not in your PATH\n\nThe tool is installed correctly, but you need to add this to your shell profile:\n  export PATH=\"%s:$PATH\"", cfg.CurrentDir, cfg.CurrentDir)
		}
		if opts.Verbose {
			printInfof("    %s is in PATH\n\n", cfg.CurrentDir)
		}

		// Step 3: Verify tool binaries are accessible from PATH and check for conflicts
		if opts.Verbose {
			printInfo("  Step 3: Checking PATH resolution for binaries...")
		}

		binariesToCheck := toolState.Binaries
		if len(binariesToCheck) == 0 {
			binariesToCheck = []string{toolName}
		}

		for _, binaryPath := range binariesToCheck {
			binaryName := filepath.Base(binaryPath)

			whichPath, err := exec.Command("which", binaryName).Output()
			if err != nil {
				return fmt.Errorf("binary '%s' not found in PATH\n\nThe tool is installed and current/ is in PATH, but '%s' cannot be found.\nThis may indicate a broken symlink in %s", binaryName, binaryName, cfg.CurrentDir)
			}

			resolvedPath := strings.TrimSpace(string(whichPath))
			expectedPath := cfg.CurrentSymlink(binaryName)

			if opts.Verbose {
				printInfof("    Binary '%s':\n", binaryName)
				printInfof("      Found: %s\n", resolvedPath)
				printInfof("      Expected: %s\n", expectedPath)
			}

			if resolvedPath != expectedPath {
				return fmt.Errorf("PATH conflict detected for '%s'\n\nThe tool is installed, but another '%s' is earlier in your PATH:\n  Using: %s\n  Expected: %s", binaryName, binaryName, resolvedPath, expectedPath)
			}
			if opts.Verbose {
				printInfo("      Correct binary is being used from PATH")
			}
		}
	} else if opts.Verbose {
		printInfo("  Steps 2-3: Skipping PATH checks (post-install verification)\n")
	}

	// Step 4: Binary integrity verification
	if opts.Verbose {
		printInfo("\n  Step 4: Verifying binary integrity...")
	}
	if err := verifyBinaryIntegrityInternal(installDir, versionState, opts.Verbose); err != nil {
		return err
	}

	// Step 5: Tier 2 dependency validation (skipped in post-install context)
	if !opts.SkipDependencyValidation {
		if opts.Verbose {
			printInfo("\n  Step 5: Validating dependencies...")
		}
		binaries := findToolBinaries(installDir, toolState.Binaries, toolName)
		if len(binaries) == 0 {
			if opts.Verbose {
				printInfo("\n    No binaries found to validate\n")
			}
		} else {
			var allResults []verify.DepResult
			for _, binPath := range binaries {
				results, err := verify.ValidateDependenciesSimple(binPath, state, cfg.HomeDir)
				if err != nil {
					return fmt.Errorf("dependency validation failed for %s: %w", filepath.Base(binPath), err)
				}
				allResults = append(allResults, results...)
			}

			if opts.Verbose {
				printInfo("\n")
			}
			if !checkDependencyResults(allResults, opts.Verbose) {
				return fmt.Errorf("dependency validation failed")
			}
		}
	} else if opts.Verbose {
		printInfo("\n  Step 5: Skipping dependency validation (post-install verification)\n")
	}

	return nil
}

// verifyBinaryIntegrityInternal verifies binary integrity and returns an error on failure.
func verifyBinaryIntegrityInternal(toolDir string, versionState *install.VersionState, verbose bool) error {
	if len(versionState.BinaryChecksums) == 0 {
		if verbose {
			printInfo("  Integrity: SKIPPED (no stored checksums - pre-feature installation)\n")
		}
		return nil
	}

	if verbose {
		printInfof("  Integrity: Verifying %d binaries...\n", len(versionState.BinaryChecksums))
	}

	mismatches, err := install.VerifyBinaryChecksums(toolDir, versionState.BinaryChecksums)
	if err != nil {
		return fmt.Errorf("integrity check error: %w", err)
	}

	if len(mismatches) == 0 {
		if verbose {
			printInfof("  Integrity: OK (%d binaries verified)\n", len(versionState.BinaryChecksums))
		}
		return nil
	}

	// Build mismatch error message
	var sb strings.Builder
	sb.WriteString("integrity verification failed - binary may have been modified after installation\n")
	for _, m := range mismatches {
		if m.Error != nil {
			sb.WriteString(fmt.Sprintf("  %s: ERROR - %v\n", m.Path, m.Error))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: expected %s..., got %s...\n", m.Path, truncateChecksum(m.Expected), truncateChecksum(m.Actual)))
		}
	}
	sb.WriteString("Run 'tsuku install <tool> --reinstall' to restore original.")
	return fmt.Errorf("%s", sb.String())
}

// checkDependencyResults checks dependency validation results and returns true if all passed.
func checkDependencyResults(results []verify.DepResult, verbose bool) bool {
	if len(results) == 0 {
		if verbose {
			printInfo("    No dynamic dependencies (statically linked)\n")
		}
		return true
	}

	allPassed := true
	for _, r := range results {
		var status string
		switch r.Category {
		case verify.DepTsukuManaged:
			if r.Status == verify.ValidationPass {
				status = fmt.Sprintf("OK (tsuku:%s@%s)", r.Recipe, r.Version)
			} else {
				status = fmt.Sprintf("FAIL (tsuku:%s@%s) - %s", r.Recipe, r.Version, r.Error)
				allPassed = false
			}
		case verify.DepExternallyManaged:
			if r.Status == verify.ValidationPass {
				status = fmt.Sprintf("OK (tsuku:%s@%s, external)", r.Recipe, r.Version)
			} else {
				status = fmt.Sprintf("FAIL (tsuku:%s@%s, external) - %s", r.Recipe, r.Version, r.Error)
				allPassed = false
			}
		case verify.DepPureSystem:
			if r.Status == verify.ValidationPass {
				status = "OK (system)"
			} else {
				status = fmt.Sprintf("FAIL (system) - %s", r.Error)
				allPassed = false
			}
		default:
			status = fmt.Sprintf("UNKNOWN - %s", r.Error)
			allPassed = false
		}
		if verbose {
			printInfof("    %s: %s\n", r.Soname, status)
		}

		// Display transitive dependencies (indented further)
		for _, t := range r.Transitive {
			var tStatus string
			switch t.Category {
			case verify.DepTsukuManaged:
				if t.Status == verify.ValidationPass {
					tStatus = fmt.Sprintf("OK (tsuku:%s@%s)", t.Recipe, t.Version)
				} else {
					tStatus = fmt.Sprintf("FAIL - %s", t.Error)
					allPassed = false
				}
			case verify.DepPureSystem:
				if t.Status == verify.ValidationPass {
					tStatus = "OK (system)"
				} else {
					tStatus = fmt.Sprintf("FAIL - %s", t.Error)
					allPassed = false
				}
			default:
				tStatus = fmt.Sprintf("UNKNOWN - %s", t.Error)
				allPassed = false
			}
			if verbose {
				printInfof("      -> %s: %s\n", t.Soname, tStatus)
			}
		}
	}

	if verbose {
		printInfof("  Tier 2: %d dependencies validated\n", len(results))
	}
	return allPassed
}

// verifyLibrary performs verification for library recipes.
// Implements Tier 1 header validation: validates that library files are valid
// shared libraries for the current platform. Additional tiers (dependency checking,
// dlopen testing, integrity) will be implemented in subsequent issues.
//
// For externally-managed libraries (installed via system package managers like apk, apt),
// the function discovers library files from the package manager rather than looking
// in $TSUKU_HOME/libs.
func verifyLibrary(name string, state *install.State, cfg *config.Config, opts LibraryVerifyOptions) error {
	// Look up library in state.Libs (tsuku-managed libraries)
	libVersions, ok := state.Libs[name]
	if ok {
		return verifyTsukuManagedLibrary(name, libVersions, state, cfg, opts)
	}

	// Not in state.Libs - check if it's an externally-managed library
	r, err := loader.Get(name, recipe.LoaderOptions{})
	if err != nil {
		return fmt.Errorf("library '%s' is not installed (not in state and recipe not found)", name)
	}

	if !r.IsLibrary() {
		return fmt.Errorf("'%s' is not a library recipe", name)
	}

	// Detect current platform
	target, err := platform.DetectTarget()
	if err != nil {
		return fmt.Errorf("failed to detect platform: %w", err)
	}

	// Check if library is provided by system packages
	extInfo, err := verify.CheckExternalLibrary(r, target)
	if err != nil {
		return fmt.Errorf("failed to check external library: %w", err)
	}

	if extInfo == nil {
		return fmt.Errorf("library '%s' is not installed", name)
	}

	return verifyExternalLibrary(name, extInfo, state, cfg, opts)
}

// verifyTsukuManagedLibrary verifies a library installed by tsuku into $TSUKU_HOME/libs.
func verifyTsukuManagedLibrary(name string, libVersions map[string]install.LibraryVersionState, state *install.State, cfg *config.Config, opts LibraryVerifyOptions) error {
	// Get the first version (libraries typically have one active version)
	var version string
	var libState install.LibraryVersionState
	for v, ls := range libVersions {
		version = v
		libState = ls
		break
	}

	libDir := cfg.LibDir(name, version)

	printInfof("Verifying library %s (version %s)...\n", name, version)

	// Verify directory exists
	if _, err := os.Stat(libDir); os.IsNotExist(err) {
		return fmt.Errorf("library directory not found: %s", libDir)
	}

	printInfo("  Directory: exists\n")

	// Tier 1: Header validation - validate all shared library files
	printInfo("  Tier 1: Header validation...\n")

	libFiles, err := findLibraryFiles(libDir)
	if err != nil {
		return fmt.Errorf("failed to scan library directory: %w", err)
	}

	if err := runTier1Validation(libFiles, libDir); err != nil {
		return err
	}

	// Tier 2: Dependency validation
	if opts.SkipDependencyValidation {
		printInfo("  Tier 2: Skipping dependency validation (post-install verification)\n")
	} else {
		if err := runTier2Validation(libFiles, state, cfg); err != nil {
			return err
		}
	}

	// Tier 3: dlopen load testing
	if err := runTier3Validation(libFiles, libDir, cfg, opts); err != nil {
		return err
	}

	// Tier 4: Integrity verification (--integrity flag)
	if opts.CheckIntegrity {
		result, err := verify.VerifyIntegrity(libDir, libState.Checksums)
		if err != nil {
			return fmt.Errorf("integrity verification error: %w", err)
		}

		if result.Skipped {
			printInfof("  Integrity: SKIPPED (%s)\n", result.Reason)
		} else if len(result.Missing) > 0 || len(result.Mismatches) > 0 {
			fmt.Fprintf(os.Stderr, "  Integrity: MODIFIED\n")
			for _, m := range result.Mismatches {
				fmt.Fprintf(os.Stderr, "    %s: expected %s..., got %s...\n",
					m.Path, truncateChecksum(m.Expected), truncateChecksum(m.Actual))
			}
			for _, path := range result.Missing {
				fmt.Fprintf(os.Stderr, "    %s: MISSING\n", path)
			}
			fmt.Fprintf(os.Stderr, "    WARNING: Library files may have been modified after installation.\n")
			fmt.Fprintf(os.Stderr, "    Run 'tsuku install <library> --reinstall' to restore original.\n")
			return fmt.Errorf("integrity verification failed: %d file(s) modified, %d file(s) missing",
				len(result.Mismatches), len(result.Missing))
		} else {
			printInfof("  Integrity: OK (%d files verified)\n", result.Verified)
		}
	}

	return nil
}

// verifyExternalLibrary verifies a library installed via system package manager.
func verifyExternalLibrary(name string, extInfo *verify.ExternalLibraryInfo, state *install.State, cfg *config.Config, opts LibraryVerifyOptions) error {
	printInfof("Verifying library %s (external: %s packages %v)...\n", name, extInfo.Family, extInfo.Packages)
	printInfo("  Source: system package manager\n")

	libFiles := extInfo.LibraryFiles

	if len(libFiles) == 0 {
		printInfo("  No shared library files found in packages\n")
		return nil
	}

	// Tier 1: Header validation
	if err := runTier1Validation(libFiles, ""); err != nil {
		return err
	}

	// Tier 2: Dependency validation
	// For externally-managed libraries, we skip dependency validation because:
	// 1. The system package manager (apk, apt, etc.) handles dependencies
	// 2. Dependencies are in system paths that tsuku's validator doesn't track
	// 3. If Tier 3 (dlopen) passes, dependencies are implicitly satisfied
	printInfo("  Tier 2: Dependency validation...\n")
	printInfo("    Skipped (system package manager handles dependencies)\n")

	// Tier 3: dlopen load testing
	// For external libraries, we test the actual system paths
	if err := runTier3ValidationDirect(libFiles, cfg, opts); err != nil {
		return err
	}

	// Tier 4: Integrity - not applicable for externally-managed libraries
	// System packages have their own integrity verification mechanisms

	return nil
}

// runTier1Validation performs header validation on library files.
func runTier1Validation(libFiles []string, baseDir string) error {
	printInfo("  Tier 1: Header validation...\n")

	if len(libFiles) == 0 {
		printInfo("    No shared library files found (may be header-only)\n")
		return nil
	}

	var validated, skipped int
	for _, libFile := range libFiles {
		displayPath := libFile
		if baseDir != "" {
			if rel, err := filepath.Rel(baseDir, libFile); err == nil {
				displayPath = rel
			}
		} else {
			displayPath = filepath.Base(libFile)
		}

		info, err := verify.ValidateHeader(libFile)
		if err != nil {
			// Check if it's a wrong architecture error - this is acceptable for cross-platform recipes
			if verr, ok := err.(*verify.ValidationError); ok {
				if verr.Category == verify.ErrWrongArch {
					printInfof("    %s: SKIPPED (%s)\n", displayPath, verr.Message)
					skipped++
					continue
				}
			}
			return fmt.Errorf("header validation failed for %s: %w", displayPath, err)
		}
		printInfof("    %s: OK (%s %s, %s)\n", displayPath, info.Format, info.Type, info.Architecture)
		validated++
	}
	printInfof("  Tier 1: %d validated", validated)
	if skipped > 0 {
		printInfof(", %d skipped (wrong arch)", skipped)
	}
	printInfo("\n")

	return nil
}

// runTier2Validation performs dependency validation on library files.
func runTier2Validation(libFiles []string, state *install.State, cfg *config.Config) error {
	printInfo("  Tier 2: Dependency validation...\n")
	if len(libFiles) == 0 {
		printInfo("    No library files to validate\n")
		return nil
	}

	var allResults []verify.DepResult
	for _, libFile := range libFiles {
		results, err := verify.ValidateDependenciesSimple(libFile, state, cfg.HomeDir)
		if err != nil {
			return fmt.Errorf("dependency validation failed for %s: %w", filepath.Base(libFile), err)
		}
		allResults = append(allResults, results...)
	}

	if !displayDependencyResults(allResults) {
		return fmt.Errorf("dependency validation failed: one or more dependencies could not be verified")
	}

	return nil
}

// runTier3Validation performs dlopen load testing for tsuku-managed libraries.
func runTier3Validation(libFiles []string, libDir string, cfg *config.Config, opts LibraryVerifyOptions) error {
	if opts.SkipDlopen || len(libFiles) == 0 {
		return nil
	}

	printInfo("  Tier 3: dlopen load testing...\n")
	result, err := verify.RunDlopenVerification(
		context.Background(),
		cfg,
		libFiles,
		false,
	)
	if err != nil {
		return fmt.Errorf("dlopen verification failed: %w", err)
	}
	if result.Warning != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", result.Warning)
	}
	if !result.Skipped {
		passed, failed := 0, 0
		for _, r := range result.Results {
			if r.OK {
				passed++
			} else {
				failed++
				relPath, _ := filepath.Rel(libDir, r.Path)
				if relPath == "" {
					relPath = filepath.Base(r.Path)
				}
				printInfof("    %s: FAIL - %s\n", relPath, r.Error)
			}
		}
		if failed > 0 {
			return fmt.Errorf("dlopen failed for %d of %d libraries", failed, passed+failed)
		}
		printInfof("  Tier 3: %d libraries loaded successfully\n", passed)
	}

	return nil
}

// runTier3ValidationDirect performs dlopen load testing using the dltest helper directly.
// This is used for externally-managed libraries where paths are outside $TSUKU_HOME/libs.
func runTier3ValidationDirect(libFiles []string, cfg *config.Config, opts LibraryVerifyOptions) error {
	if opts.SkipDlopen || len(libFiles) == 0 {
		return nil
	}

	printInfo("  Tier 3: dlopen load testing...\n")

	// Get the dltest helper path
	helperPath, err := verify.EnsureDltest(cfg)
	if err != nil {
		// Helper unavailable - skip with warning
		fmt.Fprintf(os.Stderr, "  Warning: tsuku-dltest helper not available, skipping load test\n")
		fmt.Fprintf(os.Stderr, "    Run 'tsuku install tsuku-dltest' to enable full verification\n")
		return nil
	}

	// Invoke dltest directly (bypassing path validation since these are system paths)
	results, err := invokeDltestDirect(helperPath, libFiles)
	if err != nil {
		return fmt.Errorf("dlopen verification failed: %w", err)
	}

	passed, failed := 0, 0
	for _, r := range results {
		if r.OK {
			passed++
		} else {
			failed++
			printInfof("    %s: FAIL - %s\n", filepath.Base(r.Path), r.Error)
		}
	}
	if failed > 0 {
		return fmt.Errorf("dlopen failed for %d of %d libraries", failed, passed+failed)
	}
	printInfof("  Tier 3: %d libraries loaded successfully\n", passed)

	return nil
}

// invokeDltestDirect calls the dltest helper directly without path validation.
// This is needed for external libraries that live outside $TSUKU_HOME/libs.
func invokeDltestDirect(helperPath string, paths []string) ([]verify.DlopenResult, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), verify.BatchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, helperPath, paths...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("dltest timed out after %v", verify.BatchTimeout)
	}

	// Parse JSON output
	var results []verify.DlopenResult
	if parseErr := parseJSONOutput(stdout.String(), &results); parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("dltest failed: %v (stderr: %s)", runErr, stderr.String())
		}
		return nil, fmt.Errorf("failed to parse dltest output: %w", parseErr)
	}

	return results, nil
}

// parseJSONOutput parses the JSON output from dltest.
func parseJSONOutput(output string, results *[]verify.DlopenResult) error {
	reader := strings.NewReader(output)
	decoder := json.NewDecoder(reader)
	return decoder.Decode(results)
}

// findLibraryFiles walks a directory and returns paths to shared library files.
// It identifies shared libraries by extension (.so, .dylib) and follows symlinks.
func findLibraryFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check for shared library extensions
		name := d.Name()
		if isSharedLibrary(name) {
			// Resolve symlinks to avoid validating the same file twice
			realPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil // Skip broken symlinks
			}

			// Only add if it's the real file (not a symlink)
			if realPath == path {
				files = append(files, path)
			}
		}

		return nil
	})

	return files, err
}

// isSharedLibrary returns true if the filename indicates a shared library.
func isSharedLibrary(name string) bool {
	// macOS dynamic libraries
	if strings.HasSuffix(name, ".dylib") {
		return true
	}

	// Linux shared objects: libfoo.so, libfoo.so.1, libfoo.so.1.2.3
	if strings.Contains(name, ".so") {
		// Ensure .so is followed by nothing or a version number
		idx := strings.Index(name, ".so")
		suffix := name[idx+3:]
		if suffix == "" {
			return true
		}
		// Check if suffix is version-like: .1, .1.2, .1.2.3, etc.
		if len(suffix) > 0 && suffix[0] == '.' {
			// All remaining chars should be digits or dots
			for _, c := range suffix[1:] {
				if c != '.' && (c < '0' || c > '9') {
					return false
				}
			}
			return true
		}
	}

	return false
}

var verifyCmd = &cobra.Command{
	Use:   "verify <tool>",
	Short: "Verify an installed tool or library",
	Long: `Verify that an installed tool or library is working correctly.

For visible tools, verification includes:
  1. Running the recipe's verification command
  2. Checking that the tool's bin directory is in PATH
  3. Verifying PATH resolution finds the correct binary
  4. Checking binary integrity against stored checksums

For hidden tools (execution dependencies), only the verification command is run.

For libraries, verification is tiered:
  Tier 1: Header validation - validates that library files are valid
          shared libraries (ELF or Mach-O) for the current platform
  Tier 2: Dependency checking - validates dynamic library dependencies
          are satisfied (system libs, tsuku-managed, or externally-managed)
  Tier 3: dlopen load testing - loads the library with dlopen() to verify
          it can be dynamically loaded and all dependencies are satisfied
  Tier 4: Integrity verification - compares current SHA256 checksums
          against those stored at installation time

  Flags:
    --integrity     Enable checksum verification (Tier 4)
    --skip-dlopen   Skip dlopen load testing (Tier 3)

Binary integrity verification detects post-installation tampering by comparing
current SHA256 checksums against those stored at installation time. Tools
installed before this feature will show "Integrity: SKIPPED".`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		// Get config and manager
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)

		// Load state
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		// Load recipe to determine type
		r, err := loader.Get(name, recipe.LoaderOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load recipe: %v\n", err)
			exitWithCode(ExitRecipeNotFound)
		}

		// Route based on recipe type
		if r.IsLibrary() {
			// Library verification
			opts := LibraryVerifyOptions{
				CheckIntegrity: verifyIntegrityFlag,
				SkipDlopen:     verifySkipDlopenFlag,
			}
			if err := verifyLibrary(name, state, cfg, opts); err != nil {
				fmt.Fprintf(os.Stderr, "Library verification failed: %v\n", err)
				exitWithCode(ExitVerifyFailed)
			}
			printInfof("%s is working correctly\n", name)
			return
		}

		// Tool verification
		toolState, ok := state.Installed[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "Tool '%s' is not installed\n", name)
			exitWithCode(ExitGeneral)
		}

		// Check if recipe has verification
		if r.Verify.Command == "" {
			fmt.Fprintf(os.Stderr, "Recipe for '%s' does not define verification\n", name)
			exitWithCode(ExitGeneral)
		}

		// Use shared verification function
		opts := ToolVerifyOptions{Verbose: true}
		if err := RunToolVerification(r, name, &toolState, cfg, state, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			exitWithCode(ExitVerifyFailed)
		}

		printInfof("%s is working correctly\n", name)
	},
}
