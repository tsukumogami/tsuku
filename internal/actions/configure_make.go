package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConfigureMakeAction builds software using autotools (./configure && make install).
// This is an ecosystem primitive that cannot be decomposed further.
type ConfigureMakeAction struct{}

// Name returns the action name
func (a *ConfigureMakeAction) Name() string {
	return "configure_make"
}

// Execute builds software using autotools
//
// Parameters:
//   - source_dir (required): Directory containing configure script
//   - configure_args (optional): Arguments to pass to ./configure
//   - make_targets (optional): Make targets to run (default: ["", "install"])
//   - executables (required): List of executable names to verify
//   - prefix (optional): Installation prefix (default: install_dir)
//
// The action runs:
//  1. ./configure --prefix=<install_dir> [configure_args...]
//  2. make [make_targets[0]]
//  3. make install (or make_targets[1:])
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
//	  lib/                 - Libraries (if any)
//	  include/             - Headers (if any)
func (a *ConfigureMakeAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get source directory (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok {
		return fmt.Errorf("configure_make action requires 'source_dir' parameter")
	}

	// Resolve source directory relative to work directory if not absolute
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify configure script exists
	configureScript := filepath.Join(sourceDir, "configure")
	if _, err := os.Stat(configureScript); err != nil {
		return fmt.Errorf("configure script not found at %s: %w", configureScript, err)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("configure_make action requires 'executables' parameter with at least one executable")
	}

	// Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get optional parameters
	configureArgs, _ := GetStringSlice(params, "configure_args")
	makeTargets, _ := GetStringSlice(params, "make_targets")

	// Validate configure args for security before any output
	for _, arg := range configureArgs {
		if !isValidConfigureArg(arg) {
			return fmt.Errorf("invalid configure argument '%s'", arg)
		}
	}

	// Default make targets: build (empty target) then install
	if len(makeTargets) == 0 {
		makeTargets = []string{"", "install"}
	}

	// Get prefix (defaults to install_dir)
	prefix := ctx.InstallDir
	if p, ok := GetString(params, "prefix"); ok {
		prefix = p
	}

	fmt.Printf("   Source: %s\n", sourceDir)
	fmt.Printf("   Prefix: %s\n", prefix)
	fmt.Printf("   Executables: %v\n", executables)
	if len(configureArgs) > 0 {
		fmt.Printf("   Configure args: %v\n", configureArgs)
	}

	// Build environment
	env := buildAutotoolsEnv()

	// Step 1: Run ./configure
	fmt.Printf("   Running: ./configure --prefix=%s\n", prefix)
	args := []string{"--prefix=" + prefix}
	args = append(args, configureArgs...)

	configCmd := exec.CommandContext(ctx.Context, configureScript, args...)
	configCmd.Dir = sourceDir
	configCmd.Env = env

	configOutput, err := configCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("configure failed: %w\nOutput: %s", err, string(configOutput))
	}

	// Step 2: Run make targets
	makePath := "make"

	for _, target := range makeTargets {
		var makeArgs []string
		if target != "" {
			makeArgs = []string{target}
			fmt.Printf("   Running: make %s\n", target)
		} else {
			fmt.Printf("   Running: make\n")
		}

		makeCmd := exec.CommandContext(ctx.Context, makePath, makeArgs...)
		makeCmd.Dir = sourceDir
		makeCmd.Env = env

		makeOutput, err := makeCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("make %s failed: %w\nOutput: %s", target, err, string(makeOutput))
		}
	}

	// Step 3: Verify executables exist
	binDir := filepath.Join(prefix, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}
	}

	fmt.Printf("   Build completed successfully\n")
	fmt.Printf("   Installed %d executable(s)\n", len(executables))

	return nil
}

// buildAutotoolsEnv creates an environment for autotools builds.
func buildAutotoolsEnv() []string {
	env := os.Environ()

	// Set deterministic build variables
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		// Filter variables that could cause non-determinism
		if !strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Set SOURCE_DATE_EPOCH for reproducible builds
	filteredEnv = append(filteredEnv, "SOURCE_DATE_EPOCH=0")

	// Set up C compiler if not using system compiler
	if !hasSystemCompiler() {
		if newEnv, found := SetupCCompilerEnv(filteredEnv); found {
			filteredEnv = newEnv
		}
	}

	return filteredEnv
}

// isValidConfigureArg validates configure arguments for security.
// Allows common argument patterns while rejecting shell metacharacters.
func isValidConfigureArg(arg string) bool {
	if arg == "" || len(arg) > 500 {
		return false
	}

	// Must not contain shell metacharacters
	shellChars := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "<", ">", "\n", "\r"}
	for _, char := range shellChars {
		if strings.Contains(arg, char) {
			return false
		}
	}

	return true
}
