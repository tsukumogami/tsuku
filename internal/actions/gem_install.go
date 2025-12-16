package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ensure GemInstallAction implements Decomposable
var _ Decomposable = (*GemInstallAction)(nil)

// GemInstallAction installs Ruby gems with GEM_HOME isolation
type GemInstallAction struct{ BaseAction }

// Dependencies returns ruby as eval-time, install-time, and runtime dependency.
// EvalTime is needed because Decompose() runs bundler to generate Gemfile.lock.
func (GemInstallAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"ruby"},
		Runtime:     []string{"ruby"},
		EvalTime:    []string{"ruby"}, // bundler comes with ruby
	}
}

// RequiresNetwork returns true because gem_install fetches gems from RubyGems.org.
func (GemInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *GemInstallAction) Name() string {
	return "gem_install"
}

// Execute installs a Ruby gem to the install directory
//
// Parameters:
//   - gem (required): gem name on RubyGems.org
//   - executables (required): List of executable names to verify
//   - gem_path (optional): Path to gem executable (defaults to system gem or tsuku's ruby)
//
// Environment Strategy:
//
//	GEM_HOME=<install_dir>        - Where gems are installed
//	GEM_PATH=<install_dir>        - Where gems are searched (isolation)
//	PATH=<gem_bin_dir>:$PATH      - For finding ruby
//
// Installation:
//
//	gem install <gem> --version <version> --no-document --install-dir <install_dir>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>           - Wrapper scripts
//	  gems/<gem>-<version>/      - Gem files
//	  specifications/            - Gemspecs
//	  cache/                     - Downloaded .gem files
func (a *GemInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get gem name (required)
	gemName, ok := GetString(params, "gem")
	if !ok {
		return fmt.Errorf("gem_install action requires 'gem' parameter")
	}

	// SECURITY: Validate gem name to prevent command injection
	if !isValidGemName(gemName) {
		return fmt.Errorf("invalid gem name '%s': must match RubyGems naming rules", gemName)
	}

	// SECURITY: Validate version string
	if !isValidGemVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match RubyGems version format", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("gem_install action requires 'executables' parameter with at least one executable")
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

	// Get gem path (optional, defaults to system gem or tsuku's ruby)
	gemPath, _ := GetString(params, "gem_path")
	if gemPath == "" {
		// Try to resolve gem from tsuku's tools directory first
		gemPath = ResolveGem()
		if gemPath == "" {
			// Fallback to gem in PATH
			gemPath = "gem"
		}
	}

	fmt.Printf("   Gem: %s@%s\n", gemName, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using gem: %s\n", gemPath)

	// Install gem with --install-dir for isolation
	installDir := ctx.InstallDir

	fmt.Printf("   Installing: gem install %s --version %s --install-dir %s\n",
		gemName, ctx.Version, installDir)

	// Build command: gem install <gem> --version <version> --no-document --install-dir <dir>
	// Use CommandContext for cancellation support
	cmd := exec.CommandContext(ctx.Context, gemPath, "install", gemName,
		"--version", ctx.Version,
		"--no-document",             // Skip documentation
		"--install-dir", installDir, // Install to our directory
		"--bindir", filepath.Join(installDir, "bin"), // Put executables in bin/
	)

	// Set environment for isolated installation
	// GEM_HOME and GEM_PATH ensure gems are installed and found only in our directory
	env := os.Environ()
	env = append(env, fmt.Sprintf("GEM_HOME=%s", installDir))
	env = append(env, fmt.Sprintf("GEM_PATH=%s", installDir))

	// Build PATH incrementally - start with gem's directory
	gemDir := filepath.Dir(gemPath)
	pathValue := fmt.Sprintf("%s:%s", gemDir, os.Getenv("PATH"))

	// Set up C compiler for native extensions
	// Prefer system compiler (gcc) when available because it has better compatibility
	// Fall back to zig if no system compiler is found
	if !hasSystemCompiler() {
		if zigPath := ResolveZig(); zigPath != "" {
			wrapperDir := filepath.Join(ctx.ToolsDir, "zig-cc-wrapper")
			if err := setupZigWrappers(zigPath, wrapperDir); err == nil {
				// Prepend wrapper directory to PATH and set CC/CXX
				pathValue = fmt.Sprintf("%s:%s", wrapperDir, pathValue)
				env = append(env, fmt.Sprintf("CC=%s", filepath.Join(wrapperDir, "cc")))
				env = append(env, fmt.Sprintf("CXX=%s", filepath.Join(wrapperDir, "c++")))
				fmt.Printf("   Using zig as C compiler for native extensions\n")
			}
		}
	}

	// Update PATH in env (remove existing PATH entries first)
	newEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			newEnv = append(newEnv, e)
		}
	}
	newEnv = append(newEnv, fmt.Sprintf("PATH=%s", pathValue))
	env = newEnv
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem install failed: %w\nOutput: %s", err, string(output))
	}

	// gem is verbose, only show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   gem output:\n%s\n", outputStr)
	}

	// Verify executables exist and create self-contained wrappers
	// The gem-generated wrappers need GEM_HOME/GEM_PATH at runtime,
	// so we create wrapper scripts that set these before calling the original
	binDir := filepath.Join(installDir, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}

		// Read the original gem wrapper
		originalContent, err := os.ReadFile(exePath)
		if err != nil {
			return fmt.Errorf("failed to read gem wrapper %s: %w", exe, err)
		}

		// Rename original wrapper to .gem suffix
		gemWrapperPath := exePath + ".gem"
		if err := os.Rename(exePath, gemWrapperPath); err != nil {
			return fmt.Errorf("failed to rename gem wrapper: %w", err)
		}

		// Create new wrapper that sets GEM_HOME/GEM_PATH and adds Ruby to PATH
		// Uses SCRIPT_DIR to make wrapper relocatable (works after install dir is moved)
		// gemDir contains the Ruby bin directory (where gem and ruby executables are)
		wrapperContent := fmt.Sprintf(`#!/bin/bash
# tsuku wrapper for %s (sets GEM_HOME/GEM_PATH for isolated gem)
SCRIPT_PATH="${BASH_SOURCE[0]}"
# Resolve symlinks to get the actual script location
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"
export GEM_HOME="$INSTALL_DIR"
export GEM_PATH="$INSTALL_DIR"
# Add Ruby to PATH and explicitly call ruby (don't rely on shebang)
export PATH="%s:$PATH"
exec ruby "$SCRIPT_DIR/%s.gem" "$@"
`, exe, gemDir, exe)

		if err := os.WriteFile(exePath, []byte(wrapperContent), 0755); err != nil {
			// Restore original on failure
			_ = os.Rename(gemWrapperPath, exePath)
			return fmt.Errorf("failed to create wrapper script: %w", err)
		}

		// Log debug info if enabled
		if os.Getenv("TSUKU_DEBUG") != "" {
			fmt.Printf("   Created wrapper for %s (original at %s)\n", exe, gemWrapperPath)
			fmt.Printf("   Original wrapper content:\n%s\n", string(originalContent)[:min(200, len(originalContent))])
		}
	}

	fmt.Printf("   ✓ Gem installed successfully\n")
	fmt.Printf("   ✓ Created %d self-contained wrapper(s)\n", len(executables))

	return nil
}

// isValidGemName validates gem names to prevent command injection
// Gem names on RubyGems.org: start with letter, alphanumeric + hyphens + underscores
// Max length: 100 characters (reasonable limit)
func isValidGemName(name string) bool {
	if name == "" || len(name) > 100 {
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

// isValidGemVersion validates RubyGems version strings
// Valid: 1.0.0, 1.2.3.pre, 1.2.3.rc1, 1.2.3.beta.2, 1.0.0-pre.1
// Invalid: anything with shell metacharacters
func isValidGemVersion(version string) bool {
	if version == "" || len(version) > 50 {
		return false
	}

	// Must start with a digit
	if version[0] < '0' || version[0] > '9' {
		return false
	}

	// Allow RubyGems version characters: digits, dots, letters (for pre/rc/beta), hyphens
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-') {
			return false
		}
	}

	return true
}

// Decompose converts a gem_install composite action into a gem_exec primitive step.
// This is called during plan generation to capture the Gemfile.lock at eval time.
//
// The decomposition:
//  1. Creates a temporary directory with a minimal Gemfile
//  2. Runs `bundle lock --add-checksums` to generate Gemfile.lock
//  3. Returns a gem_exec step with the captured lock_data
func (a *GemInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get gem name (required)
	gemName, ok := GetString(params, "gem")
	if !ok {
		return nil, fmt.Errorf("gem_install action requires 'gem' parameter")
	}

	// Validate gem name
	if !isValidGemName(gemName) {
		return nil, fmt.Errorf("invalid gem name '%s'", gemName)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return nil, fmt.Errorf("gem_install action requires 'executables' parameter with at least one executable")
	}

	// Use Version from context
	version := ctx.Version
	if version == "" {
		return nil, fmt.Errorf("gem_install decomposition requires a resolved version")
	}

	// Validate version format
	if !isValidGemVersion(version) {
		return nil, fmt.Errorf("invalid version format '%s'", version)
	}

	// Find bundler from ruby installation
	bundlerPath := findBundlerForEval()
	if bundlerPath == "" {
		return nil, fmt.Errorf("bundler not found: install Ruby with bundler first (tsuku install ruby)")
	}

	// Create temp directory for lockfile generation
	tempDir, err := os.MkdirTemp("", "tsuku-gem-decompose-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate Gemfile.lock using bundle lock
	lockData, err := generateGemfileLock(ctx, bundlerPath, gemName, version, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Gemfile.lock: %w", err)
	}

	// Get Ruby version for metadata (optional)
	rubyVersion := getRubyVersionForGem()

	// Build gem_exec params
	gemExecParams := map[string]interface{}{
		"gem":         gemName,
		"version":     version,
		"executables": executables,
		"lock_data":   lockData,
	}

	// Add Ruby version info if available
	if rubyVersion != "" {
		gemExecParams["ruby_version"] = rubyVersion
	}

	return []Step{
		{
			Action: "gem_exec",
			Params: gemExecParams,
		},
	}, nil
}

// findBundlerForEval finds the bundler executable for eval-time decomposition.
func findBundlerForEval() string {
	// Try tsuku's installed Ruby first
	tsukuHome := os.Getenv("TSUKU_HOME")
	if tsukuHome == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			tsukuHome = filepath.Join(homeDir, ".tsuku")
		}
	}

	if tsukuHome != "" {
		// Look for ruby installation with bundler
		patterns := []string{
			filepath.Join(tsukuHome, "tools", "ruby-*", "bin", "bundle"),
			filepath.Join(tsukuHome, "tools", "current", "bin", "bundle"),
		}
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				return matches[0]
			}
		}

		// Check if bundler symlink exists in bin
		bundlerBin := filepath.Join(tsukuHome, "bin", "bundle")
		if _, err := os.Stat(bundlerBin); err == nil {
			return bundlerBin
		}
	}

	// Try system bundler
	path, err := exec.LookPath("bundle")
	if err == nil {
		return path
	}

	return ""
}

// generateGemfileLock generates a Gemfile.lock using bundle lock.
func generateGemfileLock(ctx *EvalContext, bundlerPath, gemName, version, tempDir string) (string, error) {
	// Write minimal Gemfile
	gemfilePath := filepath.Join(tempDir, "Gemfile")
	gemfileContent := fmt.Sprintf("source 'https://rubygems.org'\ngem '%s', '= %s'\n", gemName, version)
	if err := os.WriteFile(gemfilePath, []byte(gemfileContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write Gemfile: %w", err)
	}

	// Run bundle lock with checksums
	args := []string{"lock", "--add-checksums"}

	cmd := exec.CommandContext(ctx.Context, bundlerPath, args...)
	cmd.Dir = tempDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bundle lock failed: %w\nOutput: %s", err, string(output))
	}

	// Read generated Gemfile.lock
	lockPath := filepath.Join(tempDir, "Gemfile.lock")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		return "", fmt.Errorf("failed to read Gemfile.lock: %w", err)
	}

	return string(lockData), nil
}

// getRubyVersionForGem returns the Ruby version for metadata.
func getRubyVersionForGem() string {
	rubyPath, err := exec.LookPath("ruby")
	if err != nil {
		// Try tsuku's ruby
		tsukuHome := os.Getenv("TSUKU_HOME")
		if tsukuHome == "" {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				tsukuHome = filepath.Join(homeDir, ".tsuku")
			}
		}
		if tsukuHome != "" {
			patterns := []string{
				filepath.Join(tsukuHome, "tools", "ruby-*", "bin", "ruby"),
				filepath.Join(tsukuHome, "bin", "ruby"),
			}
			for _, pattern := range patterns {
				matches, _ := filepath.Glob(pattern)
				if len(matches) > 0 {
					rubyPath = matches[0]
					break
				}
			}
		}
	}

	if rubyPath == "" {
		return ""
	}

	cmd := exec.Command(rubyPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse "ruby X.Y.Z..." format
	outputStr := string(output)
	parts := strings.Fields(outputStr)
	if len(parts) >= 2 && parts[0] == "ruby" {
		// Extract just the version number (before any suffix)
		version := parts[1]
		// Remove suffix like "p123"
		if idx := strings.Index(version, "p"); idx > 0 {
			version = version[:idx]
		}
		return version
	}

	return ""
}
