package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
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
	CheckIntegrity bool // Enable Level 4: checksum verification
	SkipDlopen     bool // Disable Level 3: dlopen load testing
}

func init() {
	verifyCmd.Flags().BoolVar(&verifyIntegrityFlag, "integrity", false, "Enable checksum verification for libraries")
	verifyCmd.Flags().BoolVar(&verifySkipDlopenFlag, "skip-dlopen", false, "Skip dlopen load testing for libraries")
}

// verifyBinaryIntegrity verifies the integrity of installed binaries using stored checksums.
// Returns true if verification passed, false if there were mismatches or errors.
// If no checksums are stored (pre-feature installation), prints a skip message and returns true.
func verifyBinaryIntegrity(toolDir string, versionState *install.VersionState) bool {
	if len(versionState.BinaryChecksums) == 0 {
		printInfo("  Integrity: SKIPPED (no stored checksums - pre-feature installation)\n")
		return true
	}

	printInfof("  Integrity: Verifying %d binaries...\n", len(versionState.BinaryChecksums))

	mismatches, err := install.VerifyBinaryChecksums(toolDir, versionState.BinaryChecksums)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Integrity: ERROR - %v\n", err)
		return false
	}

	if len(mismatches) == 0 {
		printInfof("  Integrity: OK (%d binaries verified)\n", len(versionState.BinaryChecksums))
		return true
	}

	// Report mismatches
	fmt.Fprintf(os.Stderr, "  Integrity: MODIFIED\n")
	for _, m := range mismatches {
		if m.Error != nil {
			fmt.Fprintf(os.Stderr, "    %s: ERROR - %v\n", m.Path, m.Error)
		} else {
			fmt.Fprintf(os.Stderr, "    %s: expected %s..., got %s...\n",
				m.Path, truncateChecksum(m.Expected), truncateChecksum(m.Actual))
		}
	}
	fmt.Fprintf(os.Stderr, "    WARNING: Binary may have been modified after installation.\n")
	fmt.Fprintf(os.Stderr, "    Run 'tsuku install <tool> --reinstall' to restore original.\n")
	return false
}

// truncateChecksum returns the first 12 characters of a checksum for display.
func truncateChecksum(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// verifyWithAbsolutePath verifies a hidden tool using absolute paths
func verifyWithAbsolutePath(r *recipe.Recipe, toolName, version, installDir string, versionState *install.VersionState) {
	command := r.Verify.Command
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)

	pattern := r.Verify.Pattern
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	printInfof("  Running: %s\n", command)

	cmdExec := exec.Command("sh", "-c", command)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Verification failed: %v\nOutput: %s\n", err, string(output))
		exitWithCode(ExitVerifyFailed)
	}

	outputStr := strings.TrimSpace(string(output))
	printInfof("  Output: %s\n", outputStr)

	if pattern != "" {
		if !strings.Contains(outputStr, pattern) {
			fmt.Fprintf(os.Stderr, "Output does not match expected pattern\n  Expected: %s\n  Got: %s\n", pattern, outputStr)
			exitWithCode(ExitVerifyFailed)
		}
		printInfof("  Pattern matched: %s\n", pattern)
	}

	// Binary integrity verification
	if !verifyBinaryIntegrity(installDir, versionState) {
		exitWithCode(ExitVerifyFailed)
	}
}

// verifyVisibleTool performs comprehensive verification for visible tools
func verifyVisibleTool(r *recipe.Recipe, toolName string, toolState *install.ToolState, versionState *install.VersionState, installDir string, cfg *config.Config) {
	// Step 1: Verify installation via current/ symlink
	printInfo("  Step 1: Verifying installation via symlink...")

	command := r.Verify.Command
	pattern := r.Verify.Pattern

	// For visible tools, use the binary name directly (will resolve via current/)
	// But first verify the symlink works by using absolute path
	version := toolState.Version
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	printInfof("    Running: %s\n", command)
	cmdExec := exec.Command("sh", "-c", command)

	// For Step 1, add install directory bin/ to PATH so binaries can be found
	// This is needed for binary-only installs where verify command doesn't use {install_dir}
	env := os.Environ()
	binDir := filepath.Join(installDir, "bin")
	env = append(env, "PATH="+binDir+":"+os.Getenv("PATH"))
	cmdExec.Env = env

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Installation verification failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "    Output: %s\n", string(output))
		fmt.Fprintf(os.Stderr, "\nThe tool is installed but not working correctly.\n")
		exitWithCode(ExitVerifyFailed)
	}
	outputStr := strings.TrimSpace(string(output))
	printInfof("    Output: %s\n", outputStr)

	if pattern != "" && !strings.Contains(outputStr, pattern) {
		fmt.Fprintf(os.Stderr, "    Pattern mismatch\n")
		fmt.Fprintf(os.Stderr, "    Expected: %s\n", pattern)
		fmt.Fprintf(os.Stderr, "    Got: %s\n", outputStr)
		exitWithCode(ExitVerifyFailed)
	}
	printInfo("    Installation verified\n")

	// Step 2: Check if current/ is in PATH
	printInfof("  Step 2: Checking if %s is in PATH...\n", cfg.CurrentDir)
	pathEnv := os.Getenv("PATH")
	pathInPATH := false
	for _, dir := range strings.Split(pathEnv, ":") {
		if dir == cfg.CurrentDir {
			pathInPATH = true
			break
		}
	}

	if !pathInPATH {
		fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", cfg.CurrentDir)
		fmt.Fprintf(os.Stderr, "\nThe tool is installed correctly, but you need to add this to your shell profile:\n")
		fmt.Fprintf(os.Stderr, "  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)
		exitWithCode(ExitVerifyFailed)
	}
	printInfof("    %s is in PATH\n\n", cfg.CurrentDir)

	// Step 3: Verify tool binaries are accessible from PATH and check for conflicts
	printInfo("  Step 3: Checking PATH resolution for binaries...")

	// Check each binary provided by this tool
	binariesToCheck := toolState.Binaries
	if len(binariesToCheck) == 0 {
		// Fallback: assume tool name is the binary
		binariesToCheck = []string{toolName}
	}

	for _, binaryPath := range binariesToCheck {
		// Extract just the binary name (e.g., "cargo/bin/cargo" -> "cargo")
		binaryName := filepath.Base(binaryPath)

		whichPath, err := exec.Command("which", binaryName).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "    Binary '%s' not found in PATH\n", binaryName)
			fmt.Fprintf(os.Stderr, "\nThe tool is installed and current/ is in PATH, but '%s' cannot be found.\n", binaryName)
			fmt.Fprintf(os.Stderr, "This may indicate a broken symlink in %s\n", cfg.CurrentDir)
			exitWithCode(ExitVerifyFailed)
		}

		resolvedPath := strings.TrimSpace(string(whichPath))
		expectedPath := cfg.CurrentSymlink(binaryName)

		printInfof("    Binary '%s':\n", binaryName)
		printInfof("      Found: %s\n", resolvedPath)
		printInfof("      Expected: %s\n", expectedPath)

		if resolvedPath != expectedPath {
			fmt.Fprintf(os.Stderr, "      PATH conflict detected!\n")
			fmt.Fprintf(os.Stderr, "\nThe tool is installed, but another '%s' is earlier in your PATH:\n", binaryName)
			fmt.Fprintf(os.Stderr, "  Using: %s\n", resolvedPath)
			fmt.Fprintf(os.Stderr, "  Expected: %s\n", expectedPath)

			// Try to get version info from the conflicting tool
			versionCmd := exec.Command(binaryName, "--version")
			if versionOut, err := versionCmd.CombinedOutput(); err == nil {
				fmt.Fprintf(os.Stderr, "  Conflicting version output: %s\n", strings.TrimSpace(string(versionOut)))
			}
			exitWithCode(ExitVerifyFailed)
		}
		printInfo("      Correct binary is being used from PATH")
	}

	// Step 4: Binary integrity verification
	printInfo("\n  Step 4: Verifying binary integrity...")
	if !verifyBinaryIntegrity(installDir, versionState) {
		exitWithCode(ExitVerifyFailed)
	}
}

// verifyLibrary performs verification for library recipes.
// Implements Tier 1 header validation: validates that library files are valid
// shared libraries for the current platform. Additional tiers (dependency checking,
// dlopen testing, integrity) will be implemented in subsequent issues.
func verifyLibrary(name string, state *install.State, cfg *config.Config, opts LibraryVerifyOptions) error {
	// Look up library in state.Libs (not state.Installed)
	libVersions, ok := state.Libs[name]
	if !ok {
		return fmt.Errorf("library '%s' is not installed", name)
	}

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

	if len(libFiles) == 0 {
		printInfo("    No shared library files found (may be header-only)\n")
	} else {
		var validated, skipped int
		for _, libFile := range libFiles {
			relPath, _ := filepath.Rel(libDir, libFile)
			info, err := verify.ValidateHeader(libFile)
			if err != nil {
				// Check if it's a wrong architecture error - this is acceptable for cross-platform recipes
				if verr, ok := err.(*verify.ValidationError); ok {
					if verr.Category == verify.ErrWrongArch {
						printInfof("    %s: SKIPPED (%s)\n", relPath, verr.Message)
						skipped++
						continue
					}
				}
				return fmt.Errorf("header validation failed for %s: %w", relPath, err)
			}
			printInfof("    %s: OK (%s %s, %s)\n", relPath, info.Format, info.Type, info.Architecture)
			validated++
		}
		printInfof("  Tier 1: %d validated", validated)
		if skipped > 0 {
			printInfof(", %d skipped (wrong arch)", skipped)
		}
		printInfo("\n")
	}

	// Tier 4: Integrity verification (--integrity flag)
	// Note: This is a basic implementation for CI validation. Production-grade
	// verification with detailed reporting will be implemented in issue #950.
	if opts.CheckIntegrity {
		if err := verifyLibraryIntegrity(libDir, &libState); err != nil {
			return err
		}
	}

	// Log options for future tiers
	if !opts.SkipDlopen {
		printInfo("  Tier 3 (dlopen): not yet implemented\n")
	}

	return nil
}

// verifyLibraryIntegrity verifies the integrity of installed library files using stored checksums.
// Note: This is a basic implementation for CI validation. Production-grade verification
// with detailed reporting will be implemented in issue #950.
func verifyLibraryIntegrity(libDir string, libState *install.LibraryVersionState) error {
	if len(libState.Checksums) == 0 {
		printInfo("  Integrity: SKIPPED (no stored checksums - pre-feature installation)\n")
		return nil
	}

	printInfof("  Integrity: Verifying %d files...\n", len(libState.Checksums))

	mismatches, err := install.VerifyLibraryChecksums(libDir, libState.Checksums)
	if err != nil {
		return fmt.Errorf("integrity verification error: %w", err)
	}

	if len(mismatches) == 0 {
		printInfof("  Integrity: OK (%d files verified)\n", len(libState.Checksums))
		return nil
	}

	// Report mismatches
	fmt.Fprintf(os.Stderr, "  Integrity: MODIFIED\n")
	for _, m := range mismatches {
		if m.Error != nil {
			fmt.Fprintf(os.Stderr, "    %s: ERROR - %v\n", m.Path, m.Error)
		} else {
			fmt.Fprintf(os.Stderr, "    %s: expected %s..., got %s...\n",
				m.Path, truncateChecksum(m.Expected), truncateChecksum(m.Actual))
		}
	}
	fmt.Fprintf(os.Stderr, "    WARNING: Library files may have been modified after installation.\n")
	fmt.Fprintf(os.Stderr, "    Run 'tsuku install <library> --reinstall' to restore original.\n")
	return fmt.Errorf("integrity verification failed: %d file(s) modified", len(mismatches))
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
  Tier 2: Dependency checking (not yet implemented)
  Tier 3: dlopen load testing (not yet implemented)
  Tier 4: Integrity verification (not yet implemented)

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
		r, err := loader.Get(name)
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

		installDir := filepath.Join(cfg.ToolsDir, fmt.Sprintf("%s-%s", name, toolState.Version))
		printInfof("Verifying %s (version %s)...\n", name, toolState.Version)

		// Get version state for integrity verification
		var versionState *install.VersionState
		if toolState.Versions != nil {
			if vs, ok := toolState.Versions[toolState.Version]; ok {
				versionState = &vs
			}
		}
		if versionState == nil {
			// Fallback for legacy state without multi-version support
			versionState = &install.VersionState{
				Binaries: toolState.Binaries,
			}
		}

		// Determine verification strategy based on tool visibility
		if toolState.IsHidden {
			// Hidden tools: verify with absolute path
			printInfo("  Tool is hidden (not in PATH)")
			verifyWithAbsolutePath(r, name, toolState.Version, installDir, versionState)
		} else {
			// Visible tools: comprehensive verification
			verifyVisibleTool(r, name, &toolState, versionState, installDir, cfg)
		}

		printInfof("%s is working correctly\n", name)
	},
}
