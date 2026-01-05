package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MesonBuildAction builds software using Meson.
// This is an ecosystem primitive that cannot be decomposed further.
type MesonBuildAction struct{ BaseAction }

// IsDeterministic returns false because meson builds depend on system compilers.

// Dependencies declares the install-time dependencies for this action.
// Patchelf is only needed on Linux for RPATH fixup; macOS uses install_name_tool (system-provided).
func (MesonBuildAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime:      []string{"meson", "make", "zig"},
		LinuxInstallTime: []string{"patchelf"},
	}
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

	// Find meson - must search ExecPaths since exec.Command uses parent process's PATH
	mesonPath := LookPathInDirs("meson", ctx.ExecPaths)

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

	// Step 4: Fix RPATH on installed executables if lib/ directory exists
	// Meson builds often link executables to shared libraries in lib/
	// The RPATH gets set to the absolute staging directory, which breaks after relocation
	libDir := filepath.Join(ctx.InstallDir, "lib")
	fmt.Printf("   Checking for lib directory: %s\n", libDir)
	if stat, err := os.Stat(libDir); err == nil && stat.IsDir() {
		fmt.Printf("   Found lib/ directory, fixing RPATH for shared library dependencies\n")

		// Find where .so files are actually located (could be in subdirectories)
		libPaths := findLibraryDirectories(libDir)
		if len(libPaths) == 0 {
			fmt.Printf("   No shared libraries found in lib/\n")
		} else {
			fmt.Printf("   Found libraries in: %v\n", libPaths)
		}

		binDir := filepath.Join(ctx.InstallDir, "bin")
		for _, exe := range executables {
			exePath := filepath.Join(binDir, exe)
			fmt.Printf("   Processing %s\n", exe)

			// Detect binary format
			format, err := detectBinaryFormat(exePath)
			if err != nil {
				return fmt.Errorf("failed to detect binary format for %s: %w", exe, err)
			}
			fmt.Printf("   Binary format: %s\n", format)

			// Build RPATH from found library directories
			// Convert absolute paths to $ORIGIN-relative paths
			rpath := buildRpathFromLibDirs(libPaths, ctx.InstallDir)
			if rpath == "" {
				// Fallback to standard lib path
				rpath = "$ORIGIN/../lib"
			}
			fmt.Printf("   RPATH: %s\n", rpath)

			// Set RPATH for relative library lookup
			var rpathErr error
			switch format {
			case "elf":
				fmt.Printf("   Setting RPATH with patchelf\n")
				rpathErr = setRpathLinux(ctx, exePath, rpath)
			case "macho":
				fmt.Printf("   Setting RPATH with install_name_tool\n")
				rpathErr = setRpathMacOS(exePath, rpath)
				if rpathErr == nil {
					// Also fix library load commands to use @rpath
					rpathErr = fixMachoLibraryPaths(exePath, ctx.InstallDir)
				}
			default:
				// Unknown format, skip RPATH fix
				fmt.Printf("   Skipping RPATH fix (unknown format)\n")
				continue
			}

			if rpathErr != nil {
				return fmt.Errorf("failed to set RPATH for %s: %w", exe, rpathErr)
			}
			fmt.Printf("   Successfully set RPATH for %s\n", exe)
		}
	} else {
		fmt.Printf("   No lib/ directory found or stat failed: %v\n", err)
	}

	// Step 5: Verify executables exist
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

// fixMachoLibraryPaths fixes library load commands in macOS executables.
// Meson builds on macOS link executables with absolute library paths that point
// to the staging directory. This function changes those paths to use @rpath.
func fixMachoLibraryPaths(binaryPath, installDir string) error {
	// Check if install_name_tool and otool are available
	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		return fmt.Errorf("install_name_tool not found")
	}

	otool, err := exec.LookPath("otool")
	if err != nil {
		return fmt.Errorf("otool not found")
	}

	// List library dependencies
	otoolCmd := exec.Command(otool, "-L", binaryPath)
	output, err := otoolCmd.Output()
	if err != nil {
		return fmt.Errorf("otool -L failed: %w", err)
	}

	// Parse library paths and fix those pointing to staging directory
	lines := strings.Split(string(output), "\n")
	for _, line := range lines[1:] { // Skip first line (the binary itself)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Extract library path (format: "/path/to/lib.dylib (compatibility version...)")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		libPath := parts[0]

		// Change library references that point to the staging directory
		if strings.Contains(libPath, installDir) {
			// Extract basename and use @rpath
			libBasename := filepath.Base(libPath)
			newLibRef := "@rpath/" + libBasename

			fmt.Printf("   Changing %s -> %s\n", libBasename, newLibRef)
			changeCmd := exec.Command(installNameTool, "-change", libPath, newLibRef, binaryPath)
			if output, err := changeCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("install_name_tool -change failed for %s: %s: %w",
					libBasename, strings.TrimSpace(string(output)), err)
			}
		}
	}

	// Re-sign the binary (required on Apple Silicon)
	if runtime.GOARCH == "arm64" {
		codesign, err := exec.LookPath("codesign")
		if err == nil {
			signCmd := exec.Command(codesign, "-f", "-s", "-", binaryPath)
			if err := signCmd.Run(); err != nil {
				fmt.Printf("   Warning: codesign failed for %s: %v\n", filepath.Base(binaryPath), err)
			}
		}
	}

	return nil
}

// findLibraryDirectories recursively searches for directories containing shared libraries.
// Returns absolute paths to directories containing .so files (Linux) or .dylib files (macOS).
func findLibraryDirectories(libDir string) []string {
	var dirs []string
	seen := make(map[string]bool)

	// Walk the lib directory tree
	_ = filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Check if this is a shared library file
		if !info.IsDir() {
			ext := filepath.Ext(path)
			isLibrary := ext == ".so" || ext == ".dylib" || strings.HasPrefix(ext, ".so.")

			if isLibrary {
				// Add the parent directory (not the file itself)
				dir := filepath.Dir(path)
				if !seen[dir] {
					seen[dir] = true
					dirs = append(dirs, dir)
				}
			}
		}

		return nil
	})

	return dirs
}

// buildRpathFromLibDirs converts absolute library directory paths to $ORIGIN-relative paths.
// Returns a single RPATH string with relative paths to all library directories.
func buildRpathFromLibDirs(libPaths []string, installDir string) string {
	if len(libPaths) == 0 {
		return ""
	}

	// For each library directory, compute relative path from bin/ to that directory
	binDir := filepath.Join(installDir, "bin")
	var rpathParts []string

	for _, libPath := range libPaths {
		// Compute relative path from bin/ to the library directory
		relPath, err := filepath.Rel(binDir, libPath)
		if err != nil {
			continue // Skip paths that can't be made relative
		}

		// Convert to $ORIGIN-relative path
		// From bin/executable, $ORIGIN is bin/, so we need $ORIGIN/<relPath>
		rpathPart := "$ORIGIN/" + relPath
		rpathParts = append(rpathParts, rpathPart)
	}

	if len(rpathParts) == 0 {
		return ""
	}

	// Return the first (most common) path
	// Note: We can't use ':' to combine multiple paths because validateRpath rejects it
	// In practice, libraries are usually in one location
	return rpathParts[0]
}
