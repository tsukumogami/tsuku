package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MesonBuildAction builds software using Meson.
// This is an ecosystem primitive that cannot be decomposed further.
type MesonBuildAction struct{ BaseAction }

// IsDeterministic returns false because meson builds depend on system compilers.

// Dependencies declares the install-time dependencies for this action.
func (MesonBuildAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"meson", "make", "zig", "pkg-config"}}
}

// Name returns the action name
func (a *MesonBuildAction) Name() string {
	return "meson_build"
}

// Execute builds software using Meson
//
// Parameters:
//   - source_dir (required): Directory containing meson.build
//   - meson_args (optional): Arguments to pass to meson setup
//   - executables (required): List of executable names to verify
//   - buildtype (optional): Build type (default: release)
//   - wrap_mode (optional): Dependency wrapping behavior (default: nofallback)
//
// The action runs:
//  1. meson setup <build_dir> <source_dir> --prefix=<install_dir> [meson_args...]
//  2. meson compile -C <build_dir>
//  3. meson install -C <build_dir>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
//	  lib/                 - Libraries (if any)
//	  include/             - Headers (if any)
func (a *MesonBuildAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get source directory (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok {
		return fmt.Errorf("meson_build action requires 'source_dir' parameter")
	}

	// Resolve source directory relative to work directory if not absolute
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify meson.build exists
	mesonBuild := filepath.Join(sourceDir, "meson.build")
	if _, err := os.Stat(mesonBuild); err != nil {
		return fmt.Errorf("meson.build not found at %s: %w", mesonBuild, err)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("meson_build action requires 'executables' parameter with at least one executable")
	}

	// Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get optional parameters
	mesonArgs, _ := GetStringSlice(params, "meson_args")
	buildtype := "release"
	if bt, ok := GetString(params, "buildtype"); ok {
		buildtype = bt
	}

	wrapMode := "nofallback"
	if wm, ok := GetString(params, "wrap_mode"); ok {
		wrapMode = wm
	}

	// Validate meson args for security before any output
	for _, arg := range mesonArgs {
		if !isValidMesonArg(arg) {
			return fmt.Errorf("invalid meson argument '%s'", arg)
		}
	}

	// Validate buildtype
	if !isValidBuildtype(buildtype) {
		return fmt.Errorf("invalid buildtype '%s': must be release, debug, plain, or debugoptimized", buildtype)
	}

	// Build directory
	buildDir := filepath.Join(ctx.WorkDir, "build")

	fmt.Printf("   Source: %s\n", sourceDir)
	fmt.Printf("   Build: %s\n", buildDir)
	fmt.Printf("   Install: %s\n", ctx.InstallDir)
	fmt.Printf("   Build type: %s\n", buildtype)
	fmt.Printf("   Wrap mode: %s\n", wrapMode)
	fmt.Printf("   Executables: %v\n", executables)
	if len(mesonArgs) > 0 {
		fmt.Printf("   Meson args: %v\n", mesonArgs)
	}

	// Build environment
	env := buildMesonEnv()

	// Find meson
	mesonPath := "meson"

	// Step 1: Setup with meson
	setupArgs := []string{
		"setup",
		buildDir,
		sourceDir,
		"--prefix=" + ctx.InstallDir,
		"--buildtype=" + buildtype,
		"--wrap-mode=" + wrapMode,
	}
	setupArgs = append(setupArgs, mesonArgs...)

	fmt.Printf("   Running: meson %s\n", strings.Join(setupArgs, " "))

	setupCmd := exec.CommandContext(ctx.Context, mesonPath, setupArgs...)
	setupCmd.Dir = ctx.WorkDir
	setupCmd.Env = env

	setupOutput, err := setupCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("meson setup failed: %w\nOutput: %s", err, string(setupOutput))
	}

	// Step 2: Compile
	compileArgs := []string{"compile", "-C", buildDir}

	fmt.Printf("   Running: meson %s\n", strings.Join(compileArgs, " "))

	compileCmd := exec.CommandContext(ctx.Context, mesonPath, compileArgs...)
	compileCmd.Dir = ctx.WorkDir
	compileCmd.Env = env

	compileOutput, err := compileCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("meson compile failed: %w\nOutput: %s", err, string(compileOutput))
	}

	// Step 3: Install
	installArgs := []string{"install", "-C", buildDir}

	fmt.Printf("   Running: meson %s\n", strings.Join(installArgs, " "))

	installCmd := exec.CommandContext(ctx.Context, mesonPath, installArgs...)
	installCmd.Dir = ctx.WorkDir
	installCmd.Env = env

	installOutput, err := installCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("meson install failed: %w\nOutput: %s", err, string(installOutput))
	}

	// Step 4: Verify executables exist
	binDir := filepath.Join(ctx.InstallDir, "bin")
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

// buildMesonEnv creates an environment for Meson builds.
func buildMesonEnv() []string {
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

// isValidMesonArg validates Meson arguments for security.
// Allows common argument patterns while rejecting shell metacharacters.
func isValidMesonArg(arg string) bool {
	if arg == "" || len(arg) > 500 {
		return false
	}

	// Must not contain shell metacharacters including command substitution
	shellChars := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "\n", "\r"}
	for _, char := range shellChars {
		if strings.Contains(arg, char) {
			return false
		}
	}

	return true
}

// isValidBuildtype validates Meson buildtype parameter.
// Only allows known Meson buildtype values.
func isValidBuildtype(buildtype string) bool {
	validTypes := map[string]bool{
		"release":        true,
		"debug":          true,
		"plain":          true,
		"debugoptimized": true,
	}
	return validTypes[buildtype]
}
