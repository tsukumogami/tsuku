package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// NpmExecAction implements deterministic npm/Node.js build execution.
// This is an ecosystem primitive that achieves determinism through
// lockfile enforcement and reproducible build configuration.
type NpmExecAction struct{ BaseAction }

// Dependencies returns nodejs as both install-time and runtime dependency.
func (NpmExecAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"nodejs"}, Runtime: []string{"nodejs"}}
}

// RequiresNetwork returns true because npm ci needs to download packages from npm registry.
func (NpmExecAction) RequiresNetwork() bool {
	return true
}

// Name returns the action name
func (a *NpmExecAction) Name() string {
	return "npm_exec"
}

// Execute runs an npm command with deterministic configuration.
//
// This action supports two modes:
//
// Mode 1: Source build (with source_dir + command)
//
// Parameters:
//   - source_dir (required): Directory containing package.json
//   - command (required): npm command to run (e.g., "build", "run build")
//   - use_lockfile (optional): Enforce package-lock.json with npm ci (default: true)
//   - node_version (optional): Required Node.js version constraint (e.g., ">=18.0.0")
//   - output_dir (optional): Expected output directory to verify after build
//   - npm_path (optional): Path to npm executable (defaults to system npm)
//   - ignore_scripts (optional): Skip lifecycle scripts for security (default: true)
//
// Mode 2: Package install (with package + version + package_lock)
//
// Parameters:
//   - package (required): npm package name
//   - version (required): exact package version
//   - executables (required): list of executable names to verify
//   - package_lock (required): full package-lock.json content from eval time
//   - node_version (optional): Node.js version constraint
//   - npm_path (optional): Path to npm executable
//   - ignore_scripts (optional): Skip lifecycle scripts (default: true)
//
// This action:
//   - Sets SOURCE_DATE_EPOCH for reproducible timestamps
//   - Uses npm ci instead of npm install when use_lockfile is true
//   - Validates Node.js version if constraint is specified
//   - Uses isolated npm cache to prevent cross-contamination
func (a *NpmExecAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Check which mode we're in: source build or package install
	if _, hasPackageLock := GetString(params, "package_lock"); hasPackageLock {
		return a.executePackageInstall(ctx, params)
	}

	// Mode 1: Source build (original behavior)
	// Get source_dir (required)
	sourceDir, ok := GetString(params, "source_dir")
	if !ok {
		return fmt.Errorf("npm_exec action requires 'source_dir' parameter (or 'package_lock' for package install mode)")
	}

	// Resolve source_dir relative to work directory if not absolute
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(ctx.WorkDir, sourceDir)
	}

	// Verify source_dir exists and contains package.json
	packageJSONPath := filepath.Join(sourceDir, "package.json")
	if _, err := os.Stat(packageJSONPath); err != nil {
		return fmt.Errorf("source_dir does not contain package.json: %s", sourceDir)
	}

	// Get command (required)
	command, ok := GetString(params, "command")
	if !ok {
		return fmt.Errorf("npm_exec action requires 'command' parameter")
	}

	// Get optional parameters
	useLockfile := true
	if val, ok := params["use_lockfile"].(bool); ok {
		useLockfile = val
	}

	nodeVersion, _ := GetString(params, "node_version")
	outputDir, _ := GetString(params, "output_dir")
	npmPath, _ := GetString(params, "npm_path")
	if npmPath == "" {
		npmPath = "npm"
	}

	ignoreScripts := true
	if val, ok := params["ignore_scripts"].(bool); ok {
		ignoreScripts = val
	}

	// Validate Node.js version if constraint specified
	if nodeVersion != "" {
		if err := validateNodeVersion(nodeVersion, ctx.ExecPaths...); err != nil {
			return fmt.Errorf("node version validation failed: %w", err)
		}
	}

	fmt.Printf("   Source directory: %s\n", sourceDir)
	fmt.Printf("   Command: npm %s\n", command)
	fmt.Printf("   Use lockfile: %v\n", useLockfile)

	// Set up environment for deterministic builds
	env := os.Environ()

	// SOURCE_DATE_EPOCH for reproducible timestamps
	// Use a fixed epoch time if not already set
	if os.Getenv("SOURCE_DATE_EPOCH") == "" {
		env = append(env, "SOURCE_DATE_EPOCH=0")
	}

	// Set isolated npm cache directory to prevent cross-contamination
	cacheDir := filepath.Join(ctx.WorkDir, ".npm-cache")
	env = append(env, fmt.Sprintf("npm_config_cache=%s", cacheDir))

	// Add npm's bin directory to PATH
	npmDir := filepath.Dir(npmPath)
	if npmDir != "." {
		env = append(env, fmt.Sprintf("PATH=%s:%s", npmDir, os.Getenv("PATH")))
	}

	// Add any extra exec paths (e.g., for nodejs)
	if len(ctx.ExecPaths) > 0 {
		pathVal := os.Getenv("PATH")
		for _, p := range ctx.ExecPaths {
			pathVal = p + ":" + pathVal
		}
		env = append(env, fmt.Sprintf("PATH=%s", pathVal))
	}

	// Step 1: Install dependencies
	if useLockfile {
		// Verify package-lock.json exists
		lockfilePath := filepath.Join(sourceDir, "package-lock.json")
		if _, err := os.Stat(lockfilePath); err != nil {
			return fmt.Errorf("use_lockfile is true but package-lock.json not found in %s", sourceDir)
		}

		fmt.Printf("   Installing dependencies: npm ci\n")

		// Build npm ci command with security flags
		ciArgs := []string{"ci", "--no-audit", "--no-fund", "--prefer-offline"}
		if ignoreScripts {
			ciArgs = append(ciArgs, "--ignore-scripts")
		}

		ciCmd := exec.CommandContext(ctx.Context, npmPath, ciArgs...)
		ciCmd.Dir = sourceDir
		ciCmd.Env = env

		output, err := ciCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("npm ci failed: %w\nOutput: %s", err, string(output))
		}
	} else {
		fmt.Printf("   Installing dependencies: npm install\n")

		installArgs := []string{"install", "--no-audit", "--no-fund"}
		if ignoreScripts {
			installArgs = append(installArgs, "--ignore-scripts")
		}

		installCmd := exec.CommandContext(ctx.Context, npmPath, installArgs...)
		installCmd.Dir = sourceDir
		installCmd.Env = env

		output, err := installCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
		}
	}

	// Step 2: Run the build command
	fmt.Printf("   Running: npm %s\n", command)

	// Parse command - it may be "build" or "run build"
	cmdArgs := strings.Fields(command)
	execCmd := exec.CommandContext(ctx.Context, npmPath, cmdArgs...)
	execCmd.Dir = sourceDir
	execCmd.Env = env

	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm %s failed: %w\nOutput: %s", command, err, string(output))
	}

	// Step 3: Verify output directory exists if specified
	if outputDir != "" {
		if !filepath.IsAbs(outputDir) {
			outputDir = filepath.Join(sourceDir, outputDir)
		}
		if _, err := os.Stat(outputDir); err != nil {
			return fmt.Errorf("expected output directory not found: %s", outputDir)
		}
		fmt.Printf("   Output directory verified: %s\n", outputDir)
	}

	fmt.Printf("   npm %s completed successfully\n", command)

	return nil
}

// validateNodeVersion checks if the installed Node.js version satisfies the constraint.
// Supports simple constraints like ">=18.0.0", "18.x", or exact versions like "20.10.0".
// If execPaths is provided, those paths are searched first when looking for node.
func validateNodeVersion(constraint string, execPaths ...string) error {
	// Find node binary - check exec paths first, then fall back to system PATH
	nodeBin := "node"
	for _, p := range execPaths {
		nodePath := filepath.Join(p, "node")
		if _, err := os.Stat(nodePath); err == nil {
			nodeBin = nodePath
			break
		}
	}

	fmt.Printf("   Validating node version using: %s\n", nodeBin)

	// Build PATH with exec paths prepended for the wrapper script
	env := os.Environ()
	if len(execPaths) > 0 {
		pathVal := os.Getenv("PATH")
		for _, p := range execPaths {
			pathVal = p + ":" + pathVal
		}
		// Update PATH in environment
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + pathVal
				break
			}
		}
	}

	// Run node --version
	cmd := exec.Command(nodeBin, "--version")
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("node.js not found or failed to execute: %w", err)
	}

	// Parse version (format: v20.10.0)
	versionStr := strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
	installedMajor, installedMinor, installedPatch, err := parseVersion(versionStr)
	if err != nil {
		return fmt.Errorf("failed to parse node version %s: %w", versionStr, err)
	}

	// Parse constraint
	constraint = strings.TrimSpace(constraint)

	// Handle "18.x" format
	if strings.HasSuffix(constraint, ".x") {
		requiredMajor, err := strconv.Atoi(strings.TrimSuffix(constraint, ".x"))
		if err != nil {
			return fmt.Errorf("invalid version constraint: %s", constraint)
		}
		if installedMajor != requiredMajor {
			return fmt.Errorf("node.js %s does not match constraint %s (major version mismatch)", versionStr, constraint)
		}
		return nil
	}

	// Handle ">=" constraint
	if strings.HasPrefix(constraint, ">=") {
		requiredVersion := strings.TrimPrefix(constraint, ">=")
		requiredMajor, requiredMinor, requiredPatch, err := parseVersion(requiredVersion)
		if err != nil {
			return fmt.Errorf("invalid version constraint: %s", constraint)
		}

		if !versionGTE(installedMajor, installedMinor, installedPatch,
			requiredMajor, requiredMinor, requiredPatch) {
			return fmt.Errorf("node.js %s does not satisfy constraint %s", versionStr, constraint)
		}
		return nil
	}

	// Handle ">" constraint
	if strings.HasPrefix(constraint, ">") && !strings.HasPrefix(constraint, ">=") {
		requiredVersion := strings.TrimPrefix(constraint, ">")
		requiredMajor, requiredMinor, requiredPatch, err := parseVersion(requiredVersion)
		if err != nil {
			return fmt.Errorf("invalid version constraint: %s", constraint)
		}

		if !versionGT(installedMajor, installedMinor, installedPatch,
			requiredMajor, requiredMinor, requiredPatch) {
			return fmt.Errorf("node.js %s does not satisfy constraint %s", versionStr, constraint)
		}
		return nil
	}

	// Handle exact version match
	requiredMajor, requiredMinor, requiredPatch, err := parseVersion(constraint)
	if err != nil {
		return fmt.Errorf("invalid version constraint: %s", constraint)
	}

	if installedMajor != requiredMajor || installedMinor != requiredMinor || installedPatch != requiredPatch {
		return fmt.Errorf("node.js %s does not match required version %s", versionStr, constraint)
	}

	return nil
}

// parseVersion parses a semver string into major, minor, patch components.
func parseVersion(version string) (major, minor, patch int, err error) {
	// Match patterns like "20.10.0" or "18.0"
	re := regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	matches := re.FindStringSubmatch(version)
	if matches == nil {
		return 0, 0, 0, fmt.Errorf("invalid version format")
	}

	major, _ = strconv.Atoi(matches[1])
	if matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		patch, _ = strconv.Atoi(matches[3])
	}

	return major, minor, patch, nil
}

// versionGTE returns true if installed version >= required version
func versionGTE(iMajor, iMinor, iPatch, rMajor, rMinor, rPatch int) bool {
	if iMajor > rMajor {
		return true
	}
	if iMajor < rMajor {
		return false
	}
	// Major equal
	if iMinor > rMinor {
		return true
	}
	if iMinor < rMinor {
		return false
	}
	// Minor equal
	return iPatch >= rPatch
}

// versionGT returns true if installed version > required version
func versionGT(iMajor, iMinor, iPatch, rMajor, rMinor, rPatch int) bool {
	if iMajor > rMajor {
		return true
	}
	if iMajor < rMajor {
		return false
	}
	// Major equal
	if iMinor > rMinor {
		return true
	}
	if iMinor < rMinor {
		return false
	}
	// Minor equal
	return iPatch > rPatch
}

// executePackageInstall implements Mode 2: package installation from lockfile
// This mode is used when npm_install decomposes into npm_exec with a captured lockfile.
func (a *NpmExecAction) executePackageInstall(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get required parameters
	packageName, ok := GetString(params, "package")
	if !ok {
		return fmt.Errorf("npm_exec package install mode requires 'package' parameter")
	}

	version, ok := GetString(params, "version")
	if !ok {
		return fmt.Errorf("npm_exec package install mode requires 'version' parameter")
	}

	packageLock, ok := GetString(params, "package_lock")
	if !ok {
		return fmt.Errorf("npm_exec package install mode requires 'package_lock' parameter")
	}

	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("npm_exec package install mode requires 'executables' parameter")
	}

	// Get optional parameters
	nodeVersion, _ := GetString(params, "node_version")
	npmPath, _ := GetString(params, "npm_path")
	if npmPath == "" {
		// First check ExecPaths for npm
		for _, p := range ctx.ExecPaths {
			candidatePath := filepath.Join(p, "npm")
			if _, err := os.Stat(candidatePath); err == nil {
				npmPath = candidatePath
				break
			}
		}
		// Then try ResolveNpm (looks in ~/.tsuku)
		if npmPath == "" {
			npmPath = ResolveNpm()
		}
		// Fall back to "npm" in PATH
		if npmPath == "" {
			npmPath = "npm"
		}
	}

	ignoreScripts := true
	if val, ok := params["ignore_scripts"].(bool); ok {
		ignoreScripts = val
	}

	// Validate Node.js version if constraint specified
	if nodeVersion != "" {
		// Strip the "v" prefix if present for validation
		constraint := strings.TrimPrefix(nodeVersion, "v")
		if err := validateNodeVersion(constraint, ctx.ExecPaths...); err != nil {
			return fmt.Errorf("node version validation failed: %w", err)
		}
	}

	fmt.Printf("   Package: %s@%s\n", packageName, version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using npm: %s\n", npmPath)

	// Ensure install directory exists
	if err := os.MkdirAll(ctx.InstallDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// Create package.json with the dependency in the install directory
	// npm ci --prefix expects package.json and package-lock.json in the prefix directory
	packageJSON := map[string]interface{}{
		"name":    "tsuku-npm-install",
		"version": "0.0.0",
		"dependencies": map[string]string{
			packageName: version,
		},
	}

	packageJSONBytes, err := json.MarshalIndent(packageJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package.json: %w", err)
	}

	packageJSONPath := filepath.Join(ctx.InstallDir, "package.json")
	if err := os.WriteFile(packageJSONPath, packageJSONBytes, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}

	// Write the lockfile captured at eval time
	lockfilePath := filepath.Join(ctx.InstallDir, "package-lock.json")
	if err := os.WriteFile(lockfilePath, []byte(packageLock), 0644); err != nil {
		return fmt.Errorf("failed to write package-lock.json: %w", err)
	}

	// Set up environment
	env := os.Environ()

	// SOURCE_DATE_EPOCH for reproducible timestamps
	if os.Getenv("SOURCE_DATE_EPOCH") == "" {
		env = append(env, "SOURCE_DATE_EPOCH=0")
	}

	// Set isolated npm cache directory
	cacheDir := filepath.Join(ctx.WorkDir, ".npm-cache")
	env = append(env, fmt.Sprintf("npm_config_cache=%s", cacheDir))

	// Add npm's bin directory to PATH
	npmDir := filepath.Dir(npmPath)
	if npmDir != "." {
		pathUpdated := false
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = fmt.Sprintf("PATH=%s:%s", npmDir, e[5:])
				pathUpdated = true
				break
			}
		}
		if !pathUpdated {
			env = append(env, fmt.Sprintf("PATH=%s:%s", npmDir, os.Getenv("PATH")))
		}
	}

	// Run npm ci with security hardening flags
	fmt.Printf("   Installing: npm ci in %s\n", ctx.InstallDir)

	ciArgs := []string{"ci", "--no-audit", "--no-fund", "--prefer-offline"}
	if ignoreScripts {
		ciArgs = append(ciArgs, "--ignore-scripts")
	}

	ciCmd := exec.CommandContext(ctx.Context, npmPath, ciArgs...)
	ciCmd.Dir = ctx.InstallDir
	ciCmd.Env = env

	output, err := ciCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm ci failed: %w\nOutput: %s", err, string(output))
	}

	// npm ci installs executables to node_modules/.bin/
	nodeModulesBin := filepath.Join(ctx.InstallDir, "node_modules", ".bin")

	// Create bin/ directory and symlinks to node_modules/.bin/ executables
	binDir := filepath.Join(ctx.InstallDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Verify executables exist and create symlinks
	for _, exe := range executables {
		srcPath := filepath.Join(nodeModulesBin, exe)
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, srcPath)
		}

		// Create symlink in bin/ pointing to node_modules/.bin/
		dstPath := filepath.Join(binDir, exe)
		// Use relative path for the symlink target
		relPath := filepath.Join("..", "node_modules", ".bin", exe)
		if err := os.Symlink(relPath, dstPath); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create symlink for %s: %w", exe, err)
		}
	}

	fmt.Printf("   Package installed successfully\n")
	fmt.Printf("   Verified %d executable(s)\n", len(executables))

	return nil
}
