package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ConfigureMakeAction builds software using autotools (./configure && make install).
// This is an ecosystem primitive that cannot be decomposed further.
type ConfigureMakeAction struct{ BaseAction }

// IsDeterministic returns false because autotools builds depend on system compilers.

// Name returns the action name
func (a *ConfigureMakeAction) Name() string {
	return "configure_make"
}

// Preflight validates parameters without side effects.
func (a *ConfigureMakeAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "source_dir"); !ok {
		result.AddError("configure_make action requires 'source_dir' parameter")
	}
	if _, ok := GetStringSlice(params, "executables"); !ok {
		result.AddError("configure_make action requires 'executables' parameter")
	}

	// When skip_configure is true, verify we have make_args to pass to make
	skipConfigure, _ := GetBool(params, "skip_configure")
	makeArgs, hasMakeArgs := GetStringSlice(params, "make_args")
	if skipConfigure && (!hasMakeArgs || len(makeArgs) == 0) {
		result.AddWarning("skip_configure is true but no make_args specified; build may fail")
	}
	return result
}

// Dependencies returns the dependencies needed for configure_make builds.
// Install-time dependencies are make (for building), zig (as C compiler), and pkg-config (for library discovery).
func (ConfigureMakeAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"make", "zig", "pkg-config"}}
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

	// Check if we should skip configure
	skipConfigure, _ := GetBool(params, "skip_configure")

	// Verify configure script exists (unless skipping configure)
	var configureScript string
	if !skipConfigure {
		configureScript = filepath.Join(sourceDir, "configure")
		if _, err := os.Stat(configureScript); err != nil {
			return fmt.Errorf("configure script not found at %s: %w", configureScript, err)
		}
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
	makeArgs, _ := GetStringSlice(params, "make_args")

	// Expand variables in configure_args and make_args (e.g., {libs_dir}, {deps.libcurl.version})
	vars := GetStandardVarsWithDeps(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir, ctx.Dependencies)
	for i, arg := range configureArgs {
		configureArgs[i] = ExpandVars(arg, vars)
	}
	for i, arg := range makeArgs {
		makeArgs[i] = ExpandVars(arg, vars)
	}

	// Validate configure args for security after expansion
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

	// Build environment - use shared environment from setup_build_env if available
	var env []string
	if len(ctx.Env) > 0 {
		env = ctx.Env
	} else {
		env = buildAutotoolsEnv(ctx)
	}

	// Step 1: Run ./configure (unless skip_configure is true)
	if !skipConfigure {
		fmt.Printf("   Running: ./configure --prefix=%s\n", prefix)

		// Debug: Show build-related environment variables
		for _, e := range env {
			if strings.HasPrefix(e, "CURL") || strings.HasPrefix(e, "NO_CURL") ||
				strings.HasPrefix(e, "LDFLAGS=") || strings.HasPrefix(e, "CPPFLAGS=") ||
				(strings.HasPrefix(e, "PATH=") && strings.Contains(e, "curl")) {
				fmt.Printf("   Debug env: %s\n", e)
			}
		}

		args := []string{"--prefix=" + prefix}
		args = append(args, configureArgs...)

		configCmd := exec.CommandContext(ctx.Context, configureScript, args...)
		configCmd.Dir = sourceDir
		configCmd.Env = env

		configOutput, err := configCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("configure failed: %w\nOutput: %s", err, string(configOutput))
		}

		// Debug: Show relevant parts of configure output (curl detection)
		outputStr := string(configOutput)
		for _, line := range strings.Split(outputStr, "\n") {
			if strings.Contains(strings.ToLower(line), "curl") {
				fmt.Printf("   Debug configure: %s\n", line)
			}
		}

		// Debug: Check what configure detected for curl (git-specific debugging)
		configMak := filepath.Join(sourceDir, "config.mak.autogen")
		if data, err := os.ReadFile(configMak); err == nil {
			fmt.Printf("   Debug: config.mak.autogen curl settings:\n")
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(strings.ToUpper(line), "CURL") {
					fmt.Printf("     %s\n", line)
				}
			}
		} else {
			fmt.Printf("   Debug: config.mak.autogen not found (configure may not have created it)\n")
		}

		// Touch autogenerated files to prevent make from trying to regenerate them.
		// This must happen AFTER configure, because configure may generate files like
		// version.texi that are included by other source files. If we touch before
		// configure, those generated files would be newer than our touched timestamps.
		touchAutogeneratedFiles(sourceDir)
	} else {
		fmt.Printf("   Skipping ./configure (skip_configure=true)\n")
		fmt.Printf("   Building with make directly using make_args\n")
		// Still touch autogenerated files to prevent regeneration
		touchAutogeneratedFiles(sourceDir)
	}

	// Step 2: Run make targets
	// Prefer system make if available, as it's more likely to work correctly
	// in minimal containers where Homebrew bottles may have dynamic linking issues
	makePath := findMake()

	// Common make arguments to prevent documentation regeneration
	// MAKEINFO=true prevents makeinfo invocation (for .texi -> .info)
	commonMakeArgs := []string{"MAKEINFO=true"}

	// When skip_configure is true, we need to pass prefix to make
	if skipConfigure {
		commonMakeArgs = append(commonMakeArgs, "prefix="+prefix)
	}

	// Pass LDFLAGS as a make variable in addition to environment variable
	// Some Makefiles (like Git's) don't always use LDFLAGS from the environment
	// and need it passed as a make variable for certain link commands
	for _, e := range env {
		if strings.HasPrefix(e, "LDFLAGS=") {
			commonMakeArgs = append(commonMakeArgs, e)
			break
		}
	}

	// Append user-provided make arguments
	commonMakeArgs = append(commonMakeArgs, makeArgs...)

	for _, target := range makeTargets {
		var cmdMakeArgs []string
		cmdMakeArgs = append(cmdMakeArgs, commonMakeArgs...)
		if target != "" {
			cmdMakeArgs = append(cmdMakeArgs, target)
			fmt.Printf("   Running: make %s\n", target)
		} else {
			fmt.Printf("   Running: make\n")
		}
		// Debug: Show all make arguments
		if len(commonMakeArgs) > 0 {
			fmt.Printf("   Debug make args: %v\n", commonMakeArgs)
		}

		makeCmd := exec.CommandContext(ctx.Context, makePath, cmdMakeArgs...)
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

	// Debug: Show installed directory structure (useful for diagnosing install issues)
	libexecDir := filepath.Join(prefix, "libexec")
	if _, err := os.Stat(libexecDir); err == nil {
		fmt.Printf("   Debug: libexec directory exists at %s\n", libexecDir)
		// Check for git-core subdirectory
		gitCoreDir := filepath.Join(libexecDir, "git-core")
		if entries, err := os.ReadDir(gitCoreDir); err == nil {
			fmt.Printf("   Debug: git-core has %d entries\n", len(entries))
			// Check specifically for git-remote-* helpers
			var remoteHelpers []string
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "git-remote-") {
					remoteHelpers = append(remoteHelpers, entry.Name())
				}
			}
			if len(remoteHelpers) > 0 {
				fmt.Printf("   Debug: Found %d git-remote-* helpers: %v\n", len(remoteHelpers), remoteHelpers)
			} else {
				fmt.Printf("   Debug: WARNING: No git-remote-* helpers found in git-core!\n")
			}
		} else {
			fmt.Printf("   Debug: git-core directory not found or error: %v\n", err)
		}
	}

	fmt.Printf("   Build completed successfully\n")
	fmt.Printf("   Installed %d executable(s)\n", len(executables))

	return nil
}

// buildAutotoolsEnv creates an environment for autotools builds.
// It sets PATH, PKG_CONFIG_PATH, CPPFLAGS, and LDFLAGS from dependency paths.
func buildAutotoolsEnv(ctx *ExecutionContext) []string {
	env := os.Environ()

	// Extract existing PATH before filtering
	var existingPath string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			existingPath = strings.TrimPrefix(e, "PATH=")
			break
		}
	}

	// Set deterministic build variables
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		// Filter variables that could cause non-determinism or will be set explicitly
		if !strings.HasPrefix(e, "SOURCE_DATE_EPOCH=") &&
			!strings.HasPrefix(e, "PATH=") &&
			!strings.HasPrefix(e, "PKG_CONFIG_PATH=") &&
			!strings.HasPrefix(e, "CPPFLAGS=") &&
			!strings.HasPrefix(e, "LDFLAGS=") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Add ExecPaths to PATH so dependency binaries (cmake, make, etc.) are found
	// ExecPaths contains bin directories from installed dependencies
	if len(ctx.ExecPaths) > 0 {
		newPath := strings.Join(ctx.ExecPaths, ":") + ":" + existingPath
		filteredEnv = append(filteredEnv, "PATH="+newPath)
	} else if existingPath != "" {
		filteredEnv = append(filteredEnv, "PATH="+existingPath)
	}

	// Set SOURCE_DATE_EPOCH for reproducible builds
	filteredEnv = append(filteredEnv, "SOURCE_DATE_EPOCH=0")

	// Override libtool's system library path detection to prevent RPATH stripping
	// Libtool filters out RPATH for paths it considers "system defaults", but our
	// tsuku-provided libraries are NOT system libraries. Setting this to empty
	// forces libtool to preserve RPATH for all dependency paths.
	filteredEnv = append(filteredEnv, "lt_cv_sys_lib_dlsearch_path_spec=")

	// Build paths from dependencies
	var binPaths []string
	var pkgConfigPaths []string
	var cppFlags []string
	var ldFlags []string

	// Iterate over install-time dependencies to build paths
	var curlConfig string // Track curl-config path for CURL_CONFIG env var
	for depName, depVersion := range ctx.Dependencies.InstallTime {
		// Check both tools and libs directories for the dependency
		// Libraries are installed to ~/.tsuku/libs/, tools to ~/.tsuku/tools/
		var depDir string
		toolsDepDir := filepath.Join(ctx.ToolsDir, depName+"-"+depVersion)
		libsDepDir := filepath.Join(ctx.LibsDir, depName+"-"+depVersion)

		// Prefer libs directory if it exists, otherwise use tools directory
		// We don't check if either exists here - let the subdirectory checks below
		// handle whether to add flags. This preserves the original behavior.
		if _, err := os.Stat(libsDepDir); err == nil {
			depDir = libsDepDir
		} else {
			depDir = toolsDepDir
		}

		// PATH: check for bin directory (for tools like curl-config, pkg-config, etc.)
		binDir := filepath.Join(depDir, "bin")
		if _, err := os.Stat(binDir); err == nil {
			binPaths = append(binPaths, binDir)

			// Special handling for libcurl: set CURL_CONFIG for Git's Makefile
			// Git's Makefile has a known issue where it assumes curl-config is in PATH
			// but doesn't always find it. Setting CURL_CONFIG explicitly fixes this.
			if depName == "libcurl" {
				curlConfigPath := filepath.Join(binDir, "curl-config")
				if _, err := os.Stat(curlConfigPath); err == nil {
					curlConfig = curlConfigPath
				}
			}
		}

		// PKG_CONFIG_PATH: check for lib/pkgconfig directory
		pkgConfigDir := filepath.Join(depDir, "lib", "pkgconfig")
		if _, err := os.Stat(pkgConfigDir); err == nil {
			pkgConfigPaths = append(pkgConfigPaths, pkgConfigDir)
		}

		// CPPFLAGS: check for include directory
		includeDir := filepath.Join(depDir, "include")
		if _, err := os.Stat(includeDir); err == nil {
			cppFlags = append(cppFlags, "-I"+includeDir)
		}

		// LDFLAGS: check for lib directory
		libDir := filepath.Join(depDir, "lib")
		if _, err := os.Stat(libDir); err == nil {
			ldFlags = append(ldFlags, "-L"+libDir)
			// Add RPATH for runtime library resolution
			ldFlags = append(ldFlags, "-Wl,-rpath,"+libDir)
		}
	}

	// Set PATH with dependency bin directories prepended
	if len(binPaths) > 0 {
		newPath := strings.Join(binPaths, ":")
		if existingPath != "" {
			newPath = newPath + ":" + existingPath
		}
		filteredEnv = append(filteredEnv, "PATH="+newPath)
	} else if existingPath != "" {
		// No dependency bin paths, but preserve existing PATH
		filteredEnv = append(filteredEnv, "PATH="+existingPath)
	}

	// Set CURL_CONFIG if libcurl's curl-config was found
	// This ensures Git's Makefile can find curl-config even when it's not in the default PATH
	var curlDir string
	if curlConfig != "" {
		filteredEnv = append(filteredEnv, "CURL_CONFIG="+curlConfig)
		// Also set CURLDIR for Git's Makefile (expects headers in $CURLDIR/include, libs in $CURLDIR/lib)
		curlDir = filepath.Dir(filepath.Dir(curlConfig)) // go from bin/curl-config to libcurl root
		filteredEnv = append(filteredEnv, "CURLDIR="+curlDir)
		// Explicitly enable curl support by unsetting NO_CURL (Git's Makefile variable)
		filteredEnv = append(filteredEnv, "NO_CURL=")
	}

	// Set PKG_CONFIG_PATH if any paths found
	if len(pkgConfigPaths) > 0 {
		filteredEnv = append(filteredEnv, "PKG_CONFIG_PATH="+strings.Join(pkgConfigPaths, ":"))
	}

	// Set CPPFLAGS if any flags found
	if len(cppFlags) > 0 {
		filteredEnv = append(filteredEnv, "CPPFLAGS="+strings.Join(cppFlags, " "))
	}

	// Set LDFLAGS if any flags found
	if len(ldFlags) > 0 {
		filteredEnv = append(filteredEnv, "LDFLAGS="+strings.Join(ldFlags, " "))
	}

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

// findMake returns the path to make, preferring system make over tsuku-installed make.
// This is important in minimal containers where Homebrew bottles may have dynamic
// linking issues, but system make works correctly.
func findMake() string {
	// Check common system locations first
	systemPaths := []string{"/usr/bin/make", "/bin/make", "/usr/local/bin/make"}
	for _, p := range systemPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Fall back to PATH lookup
	return "make"
}

// touchAutogeneratedFiles updates timestamps on autogenerated files to prevent
// make from trying to regenerate them. GNU source tarballs often have timestamp
// issues that cause make to invoke autotools even when they're not available.
func touchAutogeneratedFiles(sourceDir string) {
	// First, set all source files to a base time
	baseTime := time.Now().Add(-time.Hour)

	// Touch source files to base time
	_ = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		// Source files that generate other files
		if strings.HasSuffix(name, ".ac") || // configure.ac
			strings.HasSuffix(name, ".am") || // Makefile.am
			strings.HasSuffix(name, ".m4") || // m4 macros
			strings.HasSuffix(name, ".l") || // lex source -> .c
			strings.HasSuffix(name, ".y") || // yacc source -> .c
			strings.HasSuffix(name, ".texi") { // Texinfo source -> .info
			_ = os.Chtimes(path, baseTime, baseTime)
		}
		return nil
	})

	// Generated files should be newer than source files
	genTime := time.Now()

	// List of top-level autogenerated files to touch
	files := []string{
		"configure",
		"aclocal.m4",
		"config.h.in",
		"config.h.in~",
		"autoconf.h.in", // Some projects use this name
		"config.status", // Generated by configure, must be newer than configure
		"config.log",    // Generated by configure
		"Makefile",      // Generated by config.status
	}

	// Touch top-level generated files
	for _, f := range files {
		path := filepath.Join(sourceDir, f)
		if _, err := os.Stat(path); err == nil {
			_ = os.Chtimes(path, genTime, genTime)
		}
	}

	// Touch all generated files recursively
	_ = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		// Generated files that shouldn't be regenerated
		if strings.HasSuffix(name, "Makefile.in") ||
			name == "Makefile" || // Generated from Makefile.in by config.status
			strings.HasSuffix(name, ".info") ||
			strings.HasSuffix(name, ".h.in") { // config.h.in, autoconf.h.in, etc.
			_ = os.Chtimes(path, genTime, genTime)
		}
		return nil
	})
}
