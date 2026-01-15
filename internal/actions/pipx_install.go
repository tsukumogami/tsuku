package actions

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Ensure PipxInstallAction implements Decomposable
var _ Decomposable = (*PipxInstallAction)(nil)

// PipxInstallAction installs Python CLI tools using pipx with isolated venvs
type PipxInstallAction struct{ BaseAction }

// Dependencies returns python-standalone as eval-time, install-time and runtime dependency.
// EvalTime is needed because Decompose() runs pip to generate requirements with hashes.
func (PipxInstallAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"python-standalone"},
		Runtime:     []string{"python-standalone"},
		EvalTime:    []string{"python-standalone"},
	}
}

// RequiresNetwork returns true because pipx_install fetches packages from PyPI.
func (PipxInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *PipxInstallAction) Name() string {
	return "pipx_install"
}

// Preflight validates parameters without side effects.
func (a *PipxInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "package"); !ok {
		result.AddError("pipx_install action requires 'package' parameter")
	}
	if _, hasExecutables := params["executables"]; !hasExecutables {
		result.AddError("pipx_install action requires 'executables' parameter")
	}
	return result
}

// Execute installs a Python package via pipx
//
// Parameters:
//   - package (required): PyPI package name (e.g., "black")
//   - executables (required): List of executable names to verify
//   - pipx_path (optional): Path to pipx (defaults to "pipx" in PATH)
//   - python_path (optional): Python interpreter for pipx to use
//
// Environment Strategy:
//
//	PIPX_HOME=<install_dir>        - Where to create venvs
//	PIPX_BIN_DIR=<install_dir>/bin - Where to symlink executables
//
// Installation:
//
//	pipx install <package>==<version>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  venvs/<package>/           - Virtual environment
//	  bin/<executable>           - Symlink to venv/bin/<executable>
func (a *PipxInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get package name (required)
	packageName, ok := GetString(params, "package")
	if !ok {
		return fmt.Errorf("pipx_install action requires 'package' parameter")
	}

	// SECURITY: Validate version string to prevent command injection (CRITICAL)
	if !isValidPyPIVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match PEP 440 format (e.g., 1.0.0, 24.10.0rc1)", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("pipx_install action requires 'executables' parameter with at least one executable")
	}

	// Get pipx path (optional, defaults to tsuku's pipx)
	pipxPath, _ := GetString(params, "pipx_path")
	if pipxPath == "" {
		// Try to resolve pipx from tsuku's tools directory
		pipxPath = ResolvePipx()
		if pipxPath == "" {
			// Check ExecPaths from dependencies (for golden file execution)
			for _, p := range ctx.ExecPaths {
				candidatePath := filepath.Join(p, "pipx")
				if _, err := os.Stat(candidatePath); err == nil {
					pipxPath = candidatePath
					break
				}
			}
		}
		if pipxPath == "" {
			// Fallback to pipx in PATH
			pipxPath = "pipx"
		}
	}

	// Get python path (optional, for pipx --python flag)
	// If not specified in recipe, auto-detect python-standalone (User Decision Q2-B)
	pythonPath, _ := GetString(params, "python_path")
	if pythonPath == "" {
		// CRITICAL: Enforce Q2-B decision - always use python-standalone
		// Look for python-standalone in tsuku's installation
		pythonPath = ResolvePythonStandalone()
		if pythonPath == "" {
			// Check ExecPaths from dependencies (for golden file execution)
			for _, p := range ctx.ExecPaths {
				candidatePath := filepath.Join(p, "python3")
				if _, err := os.Stat(candidatePath); err == nil {
					pythonPath = candidatePath
					break
				}
			}
		}
		if pythonPath != "" {
			fmt.Printf("   Using python-standalone: %s\n", pythonPath)
		}
		// If not found, let pipx use system Python (fallback)
	}

	fmt.Printf("   Package: %s==%s\n", packageName, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using pipx: %s\n", pipxPath)

	// Set up pipx environment variables for isolation
	installDir := ctx.InstallDir
	packageSpec := fmt.Sprintf("%s==%s", packageName, ctx.Version)

	fmt.Printf("   Installing: pipx install %s\n", packageSpec)

	// Build command: pipx install <package>==<version>
	// Use CommandContext for cancellation support
	args := []string{"install", packageSpec}
	if pythonPath != "" {
		args = append(args, "--python", pythonPath)
	}

	cmd := exec.CommandContext(ctx.Context, pipxPath, args...)

	// Set environment for isolated installation
	env := os.Environ()
	env = append(env, fmt.Sprintf("PIPX_HOME=%s", installDir))
	env = append(env, fmt.Sprintf("PIPX_BIN_DIR=%s", filepath.Join(installDir, "bin")))
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pipx install failed: %w\nOutput: %s", err, string(output))
	}

	// pipx is verbose, only show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   pipx output:\n%s\n", outputStr)
	}

	// pipx creates symlinks with absolute paths to the temporary installDir
	// We need to recreate them with relative paths so they work after the
	// executor moves the directory to the final location
	binDir := filepath.Join(installDir, "bin")
	venvBinDir := filepath.Join(installDir, "venvs", packageName, "bin")

	// Fix python3 symlink in venv to use relative path to python-standalone
	python3Link := filepath.Join(venvBinDir, "python3")
	if target, err := os.Readlink(python3Link); err == nil && filepath.IsAbs(target) {
		// Remove the absolute symlink
		os.Remove(python3Link)
		// Create relative symlink to tsuku's python-standalone
		// From: $TSUKU_HOME/tools/<tool>/venvs/<package>/bin/python3
		// To:   $TSUKU_HOME/tools/python-standalone-VERSION/bin/python3
		// Relative: ../../../../python-standalone-VERSION/bin/python3
		pythonPath := ResolvePythonStandalone()
		if pythonPath != "" {
			// Extract python-standalone directory name (e.g., "python-standalone-20251217")
			// pythonPath is like: /home/user/.tsuku/tools/python-standalone-20251217/bin/python3
			pythonDir := filepath.Dir(filepath.Dir(pythonPath)) // Get tools/python-standalone-VERSION
			pythonDirName := filepath.Base(pythonDir)           // Get python-standalone-VERSION
			// Relative path from venvs/<pkg>/bin/ to sibling tool in $TSUKU_HOME/tools/
			relPath := filepath.Join("..", "..", "..", "..", pythonDirName, "bin", "python3")
			_ = os.Symlink(relPath, python3Link) // Ignore error if symlink fails
		}
	}

	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)

		// Remove the absolute symlink created by pipx
		if err := os.Remove(exePath); err != nil {
			return fmt.Errorf("failed to remove pipx symlink for %s: %w", exe, err)
		}

		// Create a relative symlink: bin/exe -> ../venvs/<package>/bin/exe
		targetPath := filepath.Join("..", "venvs", packageName, "bin", exe)
		if err := os.Symlink(targetPath, exePath); err != nil {
			return fmt.Errorf("failed to create relative symlink for %s: %w", exe, err)
		}

		// Verify the new symlink works
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("symlink verification failed for %s: %w", exe, err)
		}
	}

	fmt.Printf("   ✓ Package installed successfully\n")
	fmt.Printf("   ✓ Verified %d executable(s)\n", len(executables))

	return nil
}

// isValidPyPIVersion validates version strings against PEP 440 format
// to prevent command injection attacks.
//
// Valid examples: 1.0.0, 24.10.0, 1.2.3rc1, 2.0.0a1, 3.0.0b2
// Invalid: 1.0.0; rm -rf /, ../etc/passwd, $(evil)
//
// Pattern: ^[0-9]+(\.[0-9]+)*([a-z]+[0-9]*)?$
func isValidPyPIVersion(version string) bool {
	if version == "" || len(version) > 50 {
		return false
	}

	// Must start with a digit
	if version[0] < '0' || version[0] > '9' {
		return false
	}

	// Track state: expecting digit, dot, or release tag
	inRelease := false
	for i, c := range version {
		if i == 0 {
			continue // already checked
		}

		if c >= '0' && c <= '9' {
			continue // digits always allowed
		}

		if c == '.' {
			if inRelease {
				return false // no dots in release tags
			}
			continue
		}

		if c >= 'a' && c <= 'z' {
			inRelease = true
			continue
		}

		// Invalid character
		return false
	}

	return true
}

// Decompose converts a pipx_install composite action into a pip_exec primitive step.
// This is called during plan generation to capture requirements with hashes at eval time.
//
// The decomposition:
//  1. Creates a temporary directory with requirements.in
//  2. Runs pip to generate requirements with hashes (using pip download + hasher)
//  3. Detects native addons that would affect reproducibility
//  4. Returns a pip_exec step with the captured requirements
func (a *PipxInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get package name (required)
	packageName, ok := GetString(params, "package")
	if !ok {
		return nil, fmt.Errorf("pipx_install action requires 'package' parameter")
	}

	// Validate package name
	if !isValidPyPIPackage(packageName) {
		return nil, fmt.Errorf("invalid PyPI package name '%s'", packageName)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return nil, fmt.Errorf("pipx_install action requires 'executables' parameter with at least one executable")
	}

	// Use Version from context
	version := ctx.Version
	if version == "" {
		return nil, fmt.Errorf("pipx_install decomposition requires a resolved version")
	}

	// Validate version format
	if !isValidPyPIVersion(version) {
		return nil, fmt.Errorf("invalid version format '%s'", version)
	}

	// Find Python interpreter from python-standalone installation
	pythonPath := ResolvePythonStandalone()
	if pythonPath == "" {
		return nil, fmt.Errorf("python-standalone not found: install it first (tsuku install python-standalone)")
	}

	// Create temp directory for requirements generation
	tempDir, err := os.MkdirTemp("", "tsuku-pipx-decompose-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate requirements with hashes using pip download + hash computation
	lockedRequirements, hasNativeAddons, err := generateLockedRequirements(ctx, pythonPath, packageName, version, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate locked requirements: %w", err)
	}

	// Get Python version for reproducibility
	pythonVersion, _ := getPythonVersion(pythonPath)

	// Build pip_exec params
	pipExecParams := map[string]interface{}{
		"package":             packageName,
		"version":             version,
		"executables":         executables,
		"locked_requirements": lockedRequirements,
		"has_native_addons":   hasNativeAddons,
	}

	// Add version info if available
	if pythonVersion != "" {
		pipExecParams["python_version"] = pythonVersion
	}

	return []Step{
		{
			Action: "pip_exec",
			Params: pipExecParams,
		},
	}, nil
}

// generateLockedRequirements creates a requirements.txt with SHA256 hashes.
// It uses pip to resolve dependencies and compute hashes.
func generateLockedRequirements(ctx *EvalContext, pythonPath, packageName, version, tempDir string) (string, bool, error) {
	// First, try using pip-compile if pip-tools is available
	// If pip-tools is not available, fall back to pip freeze approach

	// Create a minimal requirements.in
	reqIn := filepath.Join(tempDir, "requirements.in")
	reqContent := fmt.Sprintf("%s==%s\n", packageName, version)
	if err := os.WriteFile(reqIn, []byte(reqContent), 0644); err != nil {
		return "", false, fmt.Errorf("failed to write requirements.in: %w", err)
	}

	// Set up environment with python bin directory
	pythonDir := filepath.Dir(pythonPath)
	env := os.Environ()
	env = append(env, fmt.Sprintf("PATH=%s:%s", pythonDir, os.Getenv("PATH")))

	// Try pip-compile first (from pip-tools)
	pipCompilePath := filepath.Join(pythonDir, "pip-compile")
	if _, err := os.Stat(pipCompilePath); err == nil {
		return runPipCompile(ctx, pipCompilePath, reqIn, tempDir, env)
	}

	// Fall back to pip download + manual hash computation
	return runPipDownloadWithHashes(ctx, pythonPath, packageName, version, tempDir, env)
}

// runPipCompile runs pip-compile to generate requirements with hashes
func runPipCompile(ctx *EvalContext, pipCompilePath, reqIn, tempDir string, env []string) (string, bool, error) {
	reqOut := filepath.Join(tempDir, "requirements.txt")

	cmd := exec.CommandContext(ctx.Context, pipCompilePath,
		"--generate-hashes",
		"--no-header",
		"--output-file", reqOut,
		reqIn,
	)
	cmd.Env = env
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("pip-compile failed: %w\nOutput: %s", err, string(output))
	}

	// Read the generated requirements.txt
	reqBytes, err := os.ReadFile(reqOut)
	if err != nil {
		return "", false, fmt.Errorf("failed to read generated requirements: %w", err)
	}

	lockedRequirements := string(reqBytes)
	hasNativeAddons := detectPythonNativeAddons(lockedRequirements)

	return lockedRequirements, hasNativeAddons, nil
}

// runPipDownloadWithHashes uses pip download to get packages and compute hashes
func runPipDownloadWithHashes(ctx *EvalContext, pythonPath, packageName, version, tempDir string, env []string) (string, bool, error) {
	downloadDir := filepath.Join(tempDir, "downloads")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Run pip download to get all dependencies
	pipPath := filepath.Join(filepath.Dir(pythonPath), "pip")
	// If pip is not in the same directory, try pip3
	if _, err := os.Stat(pipPath); err != nil {
		pipPath = filepath.Join(filepath.Dir(pythonPath), "pip3")
	}
	// If still not found, use python -m pip
	usePythonMPip := false
	if _, err := os.Stat(pipPath); err != nil {
		usePythonMPip = true
	}

	packageSpec := fmt.Sprintf("%s==%s", packageName, version)

	var cmd *exec.Cmd
	if usePythonMPip {
		cmd = exec.CommandContext(ctx.Context, pythonPath, "-m", "pip", "download",
			"--only-binary", ":all:",
			"--dest", downloadDir,
			packageSpec,
		)
	} else {
		cmd = exec.CommandContext(ctx.Context, pipPath, "download",
			"--only-binary", ":all:",
			"--dest", downloadDir,
			packageSpec,
		)
	}
	cmd.Env = env
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("pip download failed: %w\nOutput: %s", err, string(output))
	}

	// Read downloaded wheel files and compute hashes
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		return "", false, fmt.Errorf("failed to read download directory: %w", err)
	}

	var requirements strings.Builder
	hasNativeAddons := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, ".whl") {
			// Source distribution - indicates native addon or no wheel available
			hasNativeAddons = true
			continue
		}

		// Parse wheel filename: name-version-python-abi-platform.whl
		wheelInfo := parseWheelFilename(filename)
		if wheelInfo == nil {
			continue
		}

		// Compute SHA256 hash
		wheelPath := filepath.Join(downloadDir, filename)
		hash, err := computeFileSHA256(wheelPath)
		if err != nil {
			return "", false, fmt.Errorf("failed to compute hash for %s: %w", filename, err)
		}

		// Check for native addons (platform-specific wheels)
		if wheelInfo.platform != "any" {
			hasNativeAddons = true
		}

		// Write requirement line with hash
		requirements.WriteString(fmt.Sprintf("%s==%s \\\n    --hash=sha256:%s\n",
			wheelInfo.name, wheelInfo.version, hash))
	}

	if requirements.Len() == 0 {
		return "", false, fmt.Errorf("no wheels found for %s==%s", packageName, version)
	}

	return requirements.String(), hasNativeAddons, nil
}

// wheelInfo contains parsed wheel filename information
type wheelInfo struct {
	name     string
	version  string
	python   string
	abi      string
	platform string
}

// parseWheelFilename parses a wheel filename into its components
// Format: {distribution}-{version}(-{build tag})?-{python tag}-{abi tag}-{platform tag}.whl
func parseWheelFilename(filename string) *wheelInfo {
	if !strings.HasSuffix(filename, ".whl") {
		return nil
	}

	// Remove .whl extension
	name := strings.TrimSuffix(filename, ".whl")

	// Split by hyphens
	parts := strings.Split(name, "-")
	if len(parts) < 5 {
		return nil
	}

	// Last three parts are python, abi, platform
	// Everything before that is name-version (possibly with build tag)
	platform := parts[len(parts)-1]
	abi := parts[len(parts)-2]
	python := parts[len(parts)-3]

	// Find version - it's the part that starts with a digit after the package name
	var pkgName, pkgVersion string
	for i := 1; i < len(parts)-3; i++ {
		if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			pkgName = strings.Join(parts[:i], "-")
			// Handle underscore -> hyphen normalization
			pkgName = strings.ReplaceAll(pkgName, "_", "-")
			pkgVersion = parts[i]
			break
		}
	}

	if pkgName == "" || pkgVersion == "" {
		return nil
	}

	return &wheelInfo{
		name:     pkgName,
		version:  pkgVersion,
		python:   python,
		abi:      abi,
		platform: platform,
	}
}

// computeFileSHA256 computes the SHA256 hash of a file
func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Import crypto/sha256 is needed - add to imports
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// detectPythonNativeAddons checks the requirements for indicators of native addons
func detectPythonNativeAddons(requirements string) bool {
	// Check for platform-specific markers or known native extension packages
	nativeIndicators := []string{
		"manylinux",
		"win32",
		"win_amd64",
		"macosx",
	}

	for _, indicator := range nativeIndicators {
		if strings.Contains(requirements, indicator) {
			return true
		}
	}

	// Check for known packages with native extensions
	knownNativePackages := []string{
		"numpy",
		"scipy",
		"pandas",
		"cryptography",
		"pillow",
		"lxml",
	}

	reqLower := strings.ToLower(requirements)
	for _, pkg := range knownNativePackages {
		if strings.Contains(reqLower, pkg+"==") {
			return true
		}
	}

	return false
}

// isValidPyPIPackage validates PyPI package names
// Valid: alphanumeric, hyphens, underscores, dots
// Must not contain shell metacharacters
func isValidPyPIPackage(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}

	// Check for shell metacharacters
	shellChars := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "<", ">", "\n", "\r", " ", "\t"}
	for _, char := range shellChars {
		if strings.Contains(name, char) {
			return false
		}
	}

	// Package names must start with alphanumeric
	if name[0] != '_' && (name[0] < 'A' || name[0] > 'Z') && (name[0] < 'a' || name[0] > 'z') && (name[0] < '0' || name[0] > '9') {
		return false
	}

	// Allow alphanumeric, hyphens, underscores, dots
	validPattern := regexp.MustCompile(`^[A-Za-z0-9][-A-Za-z0-9._]*$`)
	return validPattern.MatchString(name)
}
