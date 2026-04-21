package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GemExecAction implements the gem_exec primitive for deterministic Ruby gem execution.
// This is an ecosystem primitive that cannot be decomposed further within tsuku.
// Determinism is achieved through Bundler's frozen lockfile enforcement.
type GemExecAction struct{ BaseAction }

// Dependencies returns ruby as both install-time and runtime dependency.
func (GemExecAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"ruby"}, Runtime: []string{"ruby"}}
}

// RequiresNetwork returns true because gem_exec fetches gems from RubyGems.org.
func (GemExecAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *GemExecAction) Name() string {
	return "gem_exec"
}

// IsDeterministic returns false because gem installation has residual non-determinism.
// While lockfile enforcement ensures identical gem versions, native extension compilation
// and platform-specific gem selection introduce variance.
func (a *GemExecAction) IsDeterministic() bool {
	return false
}

// Execute runs a Bundler command with deterministic configuration.
//
// The action supports two modes:
//
// Mode 1: lock_data mode (for decomposed gem_install)
//   - gem (required): Gem name for Gemfile generation
//   - version (required): Gem version for Gemfile generation
//   - lock_data (required): Complete Gemfile.lock content
//   - executables (required): List of executables to verify and symlink
//
// Mode 2: source_dir mode (for existing Gemfile/Gemfile.lock)
//   - source_dir (required): Directory containing Gemfile and Gemfile.lock
//   - command (required): Bundler command to run (e.g., "install", "exec rake build")
//
// Common parameters:
//   - use_lockfile (optional): Enforce Gemfile.lock with BUNDLE_FROZEN=true (default: true)
//   - ruby_version (optional): Required Ruby version (validates before execution)
//   - bundler_version (optional): Required Bundler version (validates before execution)
//   - environment_vars (optional): Additional environment variables for installation
//   - output_dir (optional): Installation target directory
//
// Environment Strategy:
//   - BUNDLE_FROZEN=true: Strict lockfile enforcement (when use_lockfile is true)
//   - GEM_HOME/GEM_PATH: Isolated gem installation
//   - BUNDLE_PATH: Installation target directory
//   - SOURCE_DATE_EPOCH: Reproducible timestamps
func (a *GemExecAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Check for lock_data mode (decomposed gem_install)
	lockData, hasLockData := GetString(params, "lock_data")
	if hasLockData && lockData != "" {
		return a.executeLockDataMode(ctx, params)
	}

	// Fall back to source_dir mode
	// Get source directory (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok || sourceDir == "" {
		return fmt.Errorf("gem_exec requires 'source_dir' parameter")
	}

	// Expand source_dir relative to work directory
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify Gemfile exists
	gemfilePath := filepath.Join(sourceDir, "Gemfile")
	if _, err := os.Stat(gemfilePath); err != nil {
		return fmt.Errorf("Gemfile not found in source_dir: %s", sourceDir)
	}

	// Get command (required)
	command, ok := GetString(params, "command")
	if !ok || command == "" {
		return fmt.Errorf("gem_exec requires 'command' parameter")
	}

	// SECURITY: Validate command doesn't contain shell metacharacters for injection
	if strings.ContainsAny(command, ";|&$`\\") {
		return fmt.Errorf("invalid command: contains shell metacharacters")
	}

	// Get optional parameters with defaults
	useLockfile := true
	if val, ok := params["use_lockfile"].(bool); ok {
		useLockfile = val
	}

	rubyVersion, _ := GetString(params, "ruby_version")
	bundlerVersion, _ := GetString(params, "bundler_version")
	outputDir, _ := GetString(params, "output_dir")
	executables, _ := GetStringSlice(params, "executables")
	environmentVars, _ := GetMapStringString(params, "environment_vars")

	// Default output_dir to source_dir/vendor/bundle
	if outputDir == "" {
		outputDir = filepath.Join(sourceDir, "vendor", "bundle")
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(ctx.WorkDir, outputDir)
	}

	// Verify Gemfile.lock exists if use_lockfile is true
	if useLockfile {
		lockPath := filepath.Join(sourceDir, "Gemfile.lock")
		if _, err := os.Stat(lockPath); err != nil {
			return fmt.Errorf("Gemfile.lock not found but use_lockfile is true: %s", sourceDir)
		}
	}

	// Validate Ruby version if specified
	if rubyVersion != "" {
		if err := a.validateRubyVersion(rubyVersion); err != nil {
			return fmt.Errorf("ruby version validation failed: %w", err)
		}
	}

	// Validate Bundler version if specified
	if bundlerVersion != "" {
		if err := a.validateBundlerVersion(bundlerVersion); err != nil {
			return fmt.Errorf("bundler version validation failed: %w", err)
		}
	}

	// Find bundler executable
	bundlerPath := a.findBundler(ctx)
	if bundlerPath == "" {
		return fmt.Errorf("bundler not found: install Ruby with bundler or ensure it's in PATH")
	}

	reporter := ctx.GetReporter()
	reporter.Log("   Source dir: %s", sourceDir)
	reporter.Log("   Command: bundle %s", command)
	reporter.Log("   Using bundler: %s", bundlerPath)
	if useLockfile {
		reporter.Log("   Lockfile enforcement: enabled")
	}

	// Build command arguments
	args := strings.Fields(command)

	// Build environment
	env := a.buildEnvironment(sourceDir, outputDir, useLockfile, environmentVars)

	// Add flags for install commands as per ecosystem_gem.md spec
	if args[0] == "install" {
		args = append(args, "--standalone") // Self-contained installation
		if outputDir != "" {
			// Use bundle config instead of --path flag (compatible with bundler 3.x+)
			configCmd := exec.CommandContext(ctx.Context, bundlerPath, "config", "set", "--local", "path", outputDir)
			configCmd.Dir = sourceDir
			configCmd.Env = env
			if output, err := configCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("bundle config failed: %w\nOutput: %s", err, string(output))
			}
		}
	}

	// Create and execute command
	cmd := exec.CommandContext(ctx.Context, bundlerPath, args...)
	cmd.Dir = sourceDir
	cmd.Env = env

	reporter.Log("   Running: bundle %s", strings.Join(args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle %s failed: %w\nOutput: %s", command, err, string(output))
	}

	// Show output if debugging (TSUKU_DEBUG env var)
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		reporter.Log("   bundle output:\n%s", outputStr)
	}

	reporter.Log("   bundle %s completed successfully", args[0])

	// Verify executables exist after installation
	if len(executables) > 0 {
		binDir := filepath.Join(outputDir, "bin")
		for _, exe := range executables {
			exePath := filepath.Join(binDir, exe)
			if _, err := os.Stat(exePath); err != nil {
				return fmt.Errorf("expected executable %q not found at %s", exe, exePath)
			}
			reporter.Log("   verified executable: %s", exe)
		}
	}

	return nil
}

// findBundler locates the bundler executable
func (a *GemExecAction) findBundler(ctx *ExecutionContext) string {
	// Try tsuku's installed Ruby first
	if ctx.ToolsDir != "" {
		// Look for ruby installation with bundler
		rubyDirs, _ := filepath.Glob(filepath.Join(ctx.ToolsDir, "ruby-*", "bin", "bundle"))
		if len(rubyDirs) > 0 {
			return rubyDirs[0]
		}
	}

	// Try system bundler
	path, err := exec.LookPath("bundle")
	if err == nil {
		return path
	}

	return ""
}

// validateRubyVersion checks if the current Ruby version matches the requirement
func (a *GemExecAction) validateRubyVersion(required string) error {
	// Run ruby --version and parse output
	cmd := exec.Command("ruby", "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check ruby version: %w", err)
	}

	// Parse "ruby X.Y.Z..." format
	outputStr := string(output)
	parts := strings.Fields(outputStr)
	if len(parts) < 2 || parts[0] != "ruby" {
		return fmt.Errorf("unexpected ruby --version output: %s", outputStr)
	}

	// Extract version (may have suffix like "p123" or "-preview")
	currentVersion := parts[1]

	// Simple prefix match for major.minor.patch
	if !strings.HasPrefix(currentVersion, required) {
		return fmt.Errorf("ruby version mismatch: required %s, got %s", required, currentVersion)
	}

	return nil
}

// validateBundlerVersion checks if the current Bundler version matches the requirement
func (a *GemExecAction) validateBundlerVersion(required string) error {
	// Run bundle --version and parse output
	cmd := exec.Command("bundle", "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check bundler version: %w", err)
	}

	// Parse "Bundler version X.Y.Z" format
	outputStr := string(output)
	parts := strings.Fields(outputStr)
	if len(parts) < 3 || parts[0] != "Bundler" {
		return fmt.Errorf("unexpected bundle --version output: %s", outputStr)
	}

	currentVersion := parts[2]

	// Simple prefix match for major.minor.patch
	if !strings.HasPrefix(currentVersion, required) {
		return fmt.Errorf("bundler version mismatch: required %s, got %s", required, currentVersion)
	}

	return nil
}

// buildEnvironment constructs the environment variables for deterministic execution
func (a *GemExecAction) buildEnvironment(sourceDir, outputDir string, useLockfile bool, customEnv map[string]string) []string {
	env := os.Environ()

	// Add Bundler-specific environment variables
	gemfileEnv := fmt.Sprintf("BUNDLE_GEMFILE=%s", filepath.Join(sourceDir, "Gemfile"))
	env = append(env, gemfileEnv)

	// Strict lockfile enforcement
	if useLockfile {
		env = append(env, "BUNDLE_FROZEN=true")
	}

	// Set GEM_HOME and GEM_PATH for isolation
	if outputDir != "" {
		env = append(env, fmt.Sprintf("GEM_HOME=%s", outputDir))
		env = append(env, fmt.Sprintf("GEM_PATH=%s", outputDir))
		env = append(env, fmt.Sprintf("BUNDLE_PATH=%s", outputDir))
	}

	// Set SOURCE_DATE_EPOCH for reproducible builds (RubyGems 3.6+)
	// Use the canonical epoch: 1980-01-01 00:00:00 UTC
	env = append(env, "SOURCE_DATE_EPOCH=315619200")

	// Add custom environment variables
	for k, v := range customEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env
}

// executeLockDataMode handles installation from lock_data parameter.
// This is the mode used when gem_install is decomposed.
func (a *GemExecAction) executeLockDataMode(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get gem name (required)
	gemName, ok := GetString(params, "gem")
	if !ok || gemName == "" {
		return fmt.Errorf("gem_exec lock_data mode requires 'gem' parameter")
	}

	// SECURITY: Validate gem name
	if !isValidGemName(gemName) {
		return fmt.Errorf("invalid gem name '%s': must match RubyGems naming rules", gemName)
	}

	// Get version (required)
	version, ok := GetString(params, "version")
	if !ok || version == "" {
		version = ctx.Version
	}
	if version == "" {
		return fmt.Errorf("gem_exec lock_data mode requires 'version' parameter")
	}

	// SECURITY: Validate version
	if !isValidGemVersion(version) {
		return fmt.Errorf("invalid gem version '%s'", version)
	}

	// Get lock_data (required - already validated in Execute)
	lockData, _ := GetString(params, "lock_data")

	// Get executables (required for verification)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("gem_exec lock_data mode requires 'executables' parameter")
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
		// Check for control characters and null bytes
		for _, c := range exe {
			if c < 32 || c == 127 || c == 0 {
				return fmt.Errorf("invalid executable name '%s': contains control characters", exe)
			}
		}
		if strings.ContainsAny(exe, "$`|;&<>()[]{}") {
			return fmt.Errorf("invalid executable name '%s': contains shell metacharacters", exe)
		}
	}

	// Get optional parameters
	rubyVersion, _ := GetString(params, "ruby_version")
	environmentVars, _ := GetMapStringString(params, "environment_vars")

	// Set up installation directory
	installDir := ctx.InstallDir

	reporter := ctx.GetReporter()
	reporter.Log("   Gem: %s@%s", gemName, version)
	reporter.Log("   Executables: %v", executables)

	// Validate Ruby version if specified
	if rubyVersion != "" {
		if err := a.validateRubyVersion(rubyVersion); err != nil {
			reporter.Warn("   Ruby version validation failed: %v", err)
		}
	}

	// Find bundler executable (must be tsuku-managed for wrapper relocatability)
	bundlerPath := a.findBundler(ctx)
	if bundlerPath == "" {
		return fmt.Errorf("bundler not found: install Ruby with bundler (tsuku install ruby)")
	}

	// Guard: wrapper scripts need a tsuku-managed ruby path for relocatability.
	// System bundler (e.g., /usr/bin/bundle) would hardcode a non-relocatable path.
	rubyBinDir := filepath.Dir(bundlerPath)
	if ctx.ToolsDir != "" && !strings.HasPrefix(bundlerPath, ctx.ToolsDir) {
		return fmt.Errorf("gem_exec lock_data mode requires tsuku-managed ruby (found system bundler at %s)", bundlerPath)
	}

	reporter.Log("   Using bundler: %s", bundlerPath)

	// Write Gemfile
	gemfilePath := filepath.Join(installDir, "Gemfile")
	gemfileContent := fmt.Sprintf("source 'https://rubygems.org'\ngem '%s', '= %s'\n", gemName, version)
	if err := os.WriteFile(gemfilePath, []byte(gemfileContent), 0644); err != nil {
		return fmt.Errorf("failed to write Gemfile: %w", err)
	}

	// Write Gemfile.lock
	lockPath := filepath.Join(installDir, "Gemfile.lock")
	if err := os.WriteFile(lockPath, []byte(lockData), 0644); err != nil {
		return fmt.Errorf("failed to write Gemfile.lock: %w", err)
	}

	// Count gems in lockfile for progress reporting
	gemCount := countLockfileGems(lockData)
	reporter.Log("   Installing %d gem(s) with lockfile enforcement", gemCount)

	// Extract bundler version from lockfile to prevent auto-upgrade
	bundlerVersion := extractBundlerVersion(lockData)
	if bundlerVersion != "" {
		if environmentVars == nil {
			environmentVars = make(map[string]string)
		}
		// Set BUNDLER_VERSION to prevent bundler from auto-installing different version
		environmentVars["BUNDLER_VERSION"] = bundlerVersion
		reporter.Log("   Lockfile bundler version: %s", bundlerVersion)
	}

	// Build environment
	env := a.buildEnvironment(installDir, installDir, true, environmentVars)

	// Configure path using bundle config (compatible with bundler 3.x+)
	// The --path flag was removed in bundler 3.0+
	configCmd := exec.CommandContext(ctx.Context, bundlerPath, "config", "set", "--local", "path", installDir)
	configCmd.Dir = installDir
	configCmd.Env = env
	if output, err := configCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bundle config failed: %w\nOutput: %s", err, string(output))
	}

	// Build install command with flags for deterministic installation
	args := []string{"install"}

	// Create and execute command
	cmd := exec.CommandContext(ctx.Context, bundlerPath, args...)
	cmd.Dir = installDir
	cmd.Env = env

	reporter.Log("   Running: bundle config set --local path %s && bundle %s", installDir, strings.Join(args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle install failed: %w\nOutput: %s", err, string(output))
	}

	// Show output if debugging (TSUKU_DEBUG env var)
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		reporter.Log("   bundle output:\n%s", outputStr)
	}

	// Verify executables exist
	// Bundler installs gems to <installDir>/ruby/<version>/bin/ when using --path
	// We need to find where bundler put the executables
	binDir := a.findBundlerBinDir(installDir)
	if binDir == "" {
		// Fallback: check standard locations
		binDir = filepath.Join(installDir, "bin")
	}

	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			// Try alternate location
			altPath := filepath.Join(installDir, "ruby", "*", "bin", exe)
			matches, _ := filepath.Glob(altPath)
			if len(matches) == 0 {
				return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
			}
			binDir = filepath.Dir(matches[0])
		}
	}

	// Create self-contained wrapper scripts at install root for executables.
	// Wrappers set GEM_HOME/GEM_PATH/PATH so gems work with tsuku's managed ruby,
	// matching the approach used by gem_install's direct path.
	//
	// Bundler installs gems into a versioned subdirectory (ruby/<ver>/), so
	// GEM_HOME must point there rather than the install root. The gem home
	// is the parent of the bundler bin directory.
	gemHomeDir := filepath.Dir(binDir)
	gemHomeRel, err := filepath.Rel(ctx.InstallDir, gemHomeDir)
	if err != nil {
		return fmt.Errorf("failed to compute relative gem home path: %w", err)
	}

	rootBinDir := filepath.Join(ctx.InstallDir, "bin")
	if err := os.MkdirAll(rootBinDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	for _, exe := range executables {
		srcScript := filepath.Join(binDir, exe)
		if err := createGemWrapper(srcScript, rootBinDir, exe, rubyBinDir, gemHomeRel); err != nil {
			return fmt.Errorf("failed to create wrapper for %s: %w", exe, err)
		}
	}

	reporter.Log("   Gem installed successfully")
	reporter.Log("   Created %d self-contained wrapper(s)", len(executables))

	return nil
}

// findBundlerBinDir finds the bin directory where bundler installed executables.
func (a *GemExecAction) findBundlerBinDir(installDir string) string {
	// Check common bundler installation paths
	// bundler installs gems differently when using --path
	patterns := []string{
		filepath.Join(installDir, "ruby", "*", "bin"),              // Standard --path location
		filepath.Join(installDir, "bin"),                           // Direct bin
		filepath.Join(installDir, "ruby", "*", "gems", "*", "exe"), // Gem exe directory
		filepath.Join(installDir, "gems", "*", "exe"),              // Alternative gem exe
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			// Return the first match that has files
			for _, match := range matches {
				entries, _ := os.ReadDir(match)
				if len(entries) > 0 {
					return match
				}
			}
		}
	}

	return ""
}

// extractBundlerVersion extracts the bundler version from BUNDLED WITH section in lockfile
func extractBundlerVersion(lockData string) string {
	lines := strings.Split(lockData, "\n")
	foundBundledWith := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "BUNDLED WITH" {
			foundBundledWith = true
			continue
		}
		if foundBundledWith && strings.TrimSpace(line) != "" {
			// Next non-empty line after "BUNDLED WITH" is the version
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// countLockfileGems counts the number of gems in a Gemfile.lock
func countLockfileGems(lockData string) int {
	count := 0
	inSpecs := false
	for _, line := range strings.Split(lockData, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "specs:" {
			inSpecs = true
			continue
		}
		if inSpecs {
			// Gem entries are indented with spaces and have version in parentheses
			if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
				if strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") {
					count++
				}
			}
			// End of specs section
			if trimmed != "" && !strings.HasPrefix(line, " ") {
				inSpecs = false
			}
		}
	}
	return count
}

func init() {
	Register(&GemExecAction{})
}
