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

// Name returns the action name
func (a *GemExecAction) Name() string {
	return "gem_exec"
}

// Execute runs a Bundler command with deterministic configuration.
//
// Parameters:
//   - source_dir (required): Directory containing Gemfile and Gemfile.lock
//   - command (required): Bundler command to run (e.g., "install", "exec rake build")
//   - use_lockfile (optional): Enforce Gemfile.lock with BUNDLE_FROZEN=true (default: true)
//   - ruby_version (optional): Required Ruby version (validates before execution)
//   - bundler_version (optional): Required Bundler version (validates before execution)
//   - executables (optional): List of executables to verify after installation
//   - environment_vars (optional): Additional environment variables for installation
//   - output_dir (optional): Installation target directory (defaults to source_dir/vendor/bundle)
//
// Environment Strategy:
//   - BUNDLE_FROZEN=true: Strict lockfile enforcement (when use_lockfile is true)
//   - GEM_HOME/GEM_PATH: Isolated gem installation
//   - BUNDLE_PATH: Installation target directory
//   - SOURCE_DATE_EPOCH: Reproducible timestamps
func (a *GemExecAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
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

	fmt.Printf("   Source dir: %s\n", sourceDir)
	fmt.Printf("   Command: bundle %s\n", command)
	fmt.Printf("   Using bundler: %s\n", bundlerPath)
	if useLockfile {
		fmt.Printf("   Lockfile enforcement: enabled\n")
	}

	// Build command arguments
	args := strings.Fields(command)

	// Add flags for install commands as per ecosystem_gem.md spec
	if args[0] == "install" {
		args = append(args, "--no-document") // Skip documentation generation
		args = append(args, "--standalone")  // Self-contained installation
		if outputDir != "" {
			args = append(args, "--path", outputDir)
		}
	}

	// Build environment
	env := a.buildEnvironment(sourceDir, outputDir, useLockfile, environmentVars)

	// Create and execute command
	cmd := exec.CommandContext(ctx.Context, bundlerPath, args...)
	cmd.Dir = sourceDir
	cmd.Env = env

	fmt.Printf("   Running: bundle %s\n", strings.Join(args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle %s failed: %w\nOutput: %s", command, err, string(output))
	}

	// Show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   bundle output:\n%s\n", outputStr)
	}

	fmt.Printf("   ✓ bundle %s completed successfully\n", args[0])

	// Verify executables exist after installation
	if len(executables) > 0 {
		binDir := filepath.Join(outputDir, "bin")
		for _, exe := range executables {
			exePath := filepath.Join(binDir, exe)
			if _, err := os.Stat(exePath); err != nil {
				return fmt.Errorf("expected executable %q not found at %s", exe, exePath)
			}
			fmt.Printf("   ✓ verified executable: %s\n", exe)
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

func init() {
	Register(&GemExecAction{})
}
