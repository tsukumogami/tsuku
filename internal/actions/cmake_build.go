package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CMakeBuildAction builds software using CMake.
// This is an ecosystem primitive that cannot be decomposed further.
type CMakeBuildAction struct{ BaseAction }

// IsDeterministic returns false because cmake builds depend on system compilers.

// Name returns the action name
func (a *CMakeBuildAction) Name() string {
	return "cmake_build"
}

// Execute builds software using CMake
//
// Parameters:
//   - source_dir (required): Directory containing CMakeLists.txt
//   - cmake_args (optional): Arguments to pass to cmake
//   - executables (required): List of executable names to verify
//   - build_type (optional): CMAKE_BUILD_TYPE (default: Release)
//
// The action runs:
//  1. cmake -S <source_dir> -B <build_dir> -DCMAKE_INSTALL_PREFIX=<install_dir> [cmake_args...]
//  2. cmake --build <build_dir>
//  3. cmake --install <build_dir>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
//	  lib/                 - Libraries (if any)
//	  include/             - Headers (if any)
func (a *CMakeBuildAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get source directory (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok {
		return fmt.Errorf("cmake_build action requires 'source_dir' parameter")
	}

	// Resolve source directory relative to work directory if not absolute
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify CMakeLists.txt exists
	cmakeLists := filepath.Join(sourceDir, "CMakeLists.txt")
	if _, err := os.Stat(cmakeLists); err != nil {
		return fmt.Errorf("CMakeLists.txt not found at %s: %w", cmakeLists, err)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("cmake_build action requires 'executables' parameter with at least one executable")
	}

	// Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get optional parameters
	cmakeArgs, _ := GetStringSlice(params, "cmake_args")
	buildType := "Release"
	if bt, ok := GetString(params, "build_type"); ok {
		buildType = bt
	}

	// Validate cmake args for security before any output
	for _, arg := range cmakeArgs {
		if !isValidCMakeArg(arg) {
			return fmt.Errorf("invalid cmake argument '%s'", arg)
		}
	}

	// Build directory
	buildDir := filepath.Join(ctx.WorkDir, "build")

	fmt.Printf("   Source: %s\n", sourceDir)
	fmt.Printf("   Build: %s\n", buildDir)
	fmt.Printf("   Install: %s\n", ctx.InstallDir)
	fmt.Printf("   Build type: %s\n", buildType)
	fmt.Printf("   Executables: %v\n", executables)
	if len(cmakeArgs) > 0 {
		fmt.Printf("   CMake args: %v\n", cmakeArgs)
	}

	// Build environment
	env := buildCMakeEnv()

	// Create build directory
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	// Find cmake
	cmakePath := "cmake"

	// Step 1: Configure with cmake
	configArgs := []string{
		"-S", sourceDir,
		"-B", buildDir,
		"-DCMAKE_INSTALL_PREFIX=" + ctx.InstallDir,
		"-DCMAKE_BUILD_TYPE=" + buildType,
	}
	configArgs = append(configArgs, cmakeArgs...)

	fmt.Printf("   Running: cmake %s\n", strings.Join(configArgs, " "))

	configCmd := exec.CommandContext(ctx.Context, cmakePath, configArgs...)
	configCmd.Dir = ctx.WorkDir
	configCmd.Env = env

	configOutput, err := configCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cmake configure failed: %w\nOutput: %s", err, string(configOutput))
	}

	// Step 2: Build
	buildArgs := []string{"--build", buildDir, "--config", buildType}

	fmt.Printf("   Running: cmake %s\n", strings.Join(buildArgs, " "))

	buildCmd := exec.CommandContext(ctx.Context, cmakePath, buildArgs...)
	buildCmd.Dir = ctx.WorkDir
	buildCmd.Env = env

	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cmake build failed: %w\nOutput: %s", err, string(buildOutput))
	}

	// Step 3: Install
	installArgs := []string{"--install", buildDir}

	fmt.Printf("   Running: cmake %s\n", strings.Join(installArgs, " "))

	installCmd := exec.CommandContext(ctx.Context, cmakePath, installArgs...)
	installCmd.Dir = ctx.WorkDir
	installCmd.Env = env

	installOutput, err := installCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cmake install failed: %w\nOutput: %s", err, string(installOutput))
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

// buildCMakeEnv creates an environment for CMake builds.
func buildCMakeEnv() []string {
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

// isValidCMakeArg validates CMake arguments for security.
// Allows common argument patterns while rejecting shell metacharacters.
func isValidCMakeArg(arg string) bool {
	if arg == "" || len(arg) > 500 {
		return false
	}

	// Must not contain shell metacharacters including command substitution
	// Note: CMake generator expressions use $<...> not $(), so blocking $() is safe
	shellChars := []string{";", "&", "|", "`", "$", "(", ")", "\n", "\r"}
	for _, char := range shellChars {
		if strings.Contains(arg, char) {
			return false
		}
	}

	return true
}
