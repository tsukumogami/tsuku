package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ensure NpmInstallAction implements Decomposable
var _ Decomposable = (*NpmInstallAction)(nil)

// NpmInstallAction implements npm package installation with --prefix isolation
type NpmInstallAction struct{ BaseAction }

// Dependencies returns nodejs as install-time, runtime, and eval-time dependency.
// EvalTime is needed because Decompose() runs npm to generate package-lock.json.
func (NpmInstallAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"nodejs"},
		Runtime:     []string{"nodejs"},
		EvalTime:    []string{"nodejs"},
	}
}

// RequiresNetwork returns true because npm_install fetches packages from npm registry.
func (NpmInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *NpmInstallAction) Name() string {
	return "npm_install"
}

// Preflight validates parameters without side effects.
func (a *NpmInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "package"); !ok {
		result.AddError("npm_install action requires 'package' parameter")
	}
	if _, hasExecutables := params["executables"]; !hasExecutables {
		result.AddError("npm_install action requires 'executables' parameter")
	}
	return result
}

// Execute installs an npm package to the install directory
//
// Parameters:
//   - package (required): npm package name
//   - executables (required): List of executable names to install
//   - npm_path (optional): Path to npm executable (defaults to system npm)
//
// This action uses npm install -g --prefix for isolation.
// Note: Ensure npm is available before executing this action.
func (a *NpmInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get package name (required)
	packageName, ok := GetString(params, "package")
	if !ok {
		return fmt.Errorf("npm_install action requires 'package' parameter")
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("npm_install action requires 'executables' parameter with at least one executable")
	}

	// Get npm path (optional, defaults to "npm" from PATH)
	npmPath, _ := GetString(params, "npm_path")
	if npmPath == "" {
		npmPath = "npm"
	}

	fmt.Printf("   Package: %s@%s\n", packageName, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using npm: %s\n", npmPath)

	// Install package with --prefix for isolation
	installDir := ctx.InstallDir
	packageSpec := fmt.Sprintf("%s@%s", packageName, ctx.Version)

	fmt.Printf("   Installing: npm install -g --prefix=%s %s\n", installDir, packageSpec)

	// Use CommandContext for cancellation support
	cmd := exec.CommandContext(ctx.Context, npmPath, "install", "-g", fmt.Sprintf("--prefix=%s", installDir), packageSpec)

	// Set up environment: add npm's bin directory to PATH so npm can find node
	// npm is a Node.js script that needs node in PATH
	npmDir := filepath.Dir(npmPath) // e.g., /root/.tsuku/tools/nodejs-24.11.1/bin
	env := os.Environ()
	// Prepend npm's bin dir to PATH
	env = append(env, fmt.Sprintf("PATH=%s:%s", npmDir, os.Getenv("PATH")))
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
	}

	// npm is verbose, only show output on error or if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   npm output:\n%s\n", outputStr)
	}

	// Verify executables exist
	binDir := filepath.Join(installDir, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}
	}

	fmt.Printf("   ✓ Package installed successfully\n")
	fmt.Printf("   ✓ Verified %d executable(s)\n", len(executables))

	return nil
}

// Decompose converts an npm_install composite action into an npm_exec primitive step.
// This is called during plan generation to capture package-lock.json at eval time.
//
// The decomposition:
//  1. Creates a temporary directory with package.json
//  2. Runs `npm install --package-lock-only` to generate lockfile without downloading packages
//  3. Reads the generated package-lock.json
//  4. Detects native addons that would affect reproducibility
//  5. Returns an npm_exec step with the captured lockfile
func (a *NpmInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get package name (required)
	packageName, ok := GetString(params, "package")
	if !ok {
		return nil, fmt.Errorf("npm_install action requires 'package' parameter")
	}

	// Validate package name
	if !isValidNpmPackage(packageName) {
		return nil, fmt.Errorf("invalid npm package name '%s'", packageName)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return nil, fmt.Errorf("npm_install action requires 'executables' parameter with at least one executable")
	}

	// Use Version from context
	version := ctx.Version
	if version == "" {
		return nil, fmt.Errorf("npm_install decomposition requires a resolved version")
	}

	// Find npm binary from nodejs installation
	npmPath := ResolveNpm()
	if npmPath == "" {
		return nil, fmt.Errorf("npm not found: install nodejs first (tsuku install nodejs)")
	}

	// Create temp directory for lockfile generation
	tempDir, err := os.MkdirTemp("", "tsuku-npm-decompose-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create package.json with single dependency
	packageJSON := map[string]interface{}{
		"name":    "tsuku-npm-eval",
		"version": "0.0.0",
		"dependencies": map[string]string{
			packageName: version,
		},
	}

	packageJSONBytes, err := json.MarshalIndent(packageJSON, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal package.json: %w", err)
	}

	packageJSONPath := filepath.Join(tempDir, "package.json")
	if err := os.WriteFile(packageJSONPath, packageJSONBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write package.json: %w", err)
	}

	// Set up environment with npm's bin directory in PATH
	npmDir := filepath.Dir(npmPath)
	env := os.Environ()
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

	// Run npm install --package-lock-only to generate lockfile without installing
	cmd := exec.CommandContext(ctx.Context, npmPath, "install", "--package-lock-only")
	cmd.Dir = tempDir
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("npm install --package-lock-only failed: %w\nOutput: %s", err, string(output))
	}

	// Read the generated package-lock.json
	lockfilePath := filepath.Join(tempDir, "package-lock.json")
	lockfileBytes, err := os.ReadFile(lockfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package-lock.json: %w", err)
	}

	lockfileContent := string(lockfileBytes)

	// Detect native addons in the dependency tree
	hasNativeAddons := detectNativeAddons(lockfileContent)

	// Get Node.js version for reproducibility
	nodeVersion := GetNodeVersion(npmPath)

	// Get npm version for documentation
	npmVersion := GetNpmVersion(npmPath)

	// Build npm_exec params
	npmExecParams := map[string]interface{}{
		"package":           packageName,
		"version":           version,
		"executables":       executables,
		"package_lock":      lockfileContent,
		"ignore_scripts":    true, // Security default
		"has_native_addons": hasNativeAddons,
	}

	// Add version info if available
	if nodeVersion != "" {
		npmExecParams["node_version"] = nodeVersion
	}
	if npmVersion != "" {
		npmExecParams["npm_version"] = npmVersion
	}

	return []Step{
		{
			Action: "npm_exec",
			Params: npmExecParams,
		},
	}, nil
}

// isValidNpmPackage validates npm package names
// Valid: alphanumeric, hyphens, underscores, dots, @ for scoped packages
// Must not contain shell metacharacters
func isValidNpmPackage(name string) bool {
	if name == "" || len(name) > 214 { // npm limit is 214 chars
		return false
	}

	// Check for shell metacharacters
	shellChars := []string{";", "&", "|", "`", "$", "(", ")", "{", "}", "<", ">", "\n", "\r", " ", "\t"}
	for _, char := range shellChars {
		if strings.Contains(name, char) {
			return false
		}
	}

	return true
}

// ResolveNpm finds the npm binary from the nodejs installation
func ResolveNpm() string {
	// Try to find npm from tsuku's nodejs installation
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	toolsDir := filepath.Join(homeDir, ".tsuku", "tools")

	// Look for nodejs-* directories
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "nodejs-") {
			npmPath := filepath.Join(toolsDir, entry.Name(), "bin", "npm")
			if _, err := os.Stat(npmPath); err == nil {
				return npmPath
			}
		}
	}

	// Fall back to PATH
	if path, err := exec.LookPath("npm"); err == nil {
		return path
	}

	return ""
}

// GetNodeVersion returns the Node.js version string
func GetNodeVersion(npmPath string) string {
	// Node is in the same directory as npm
	nodeDir := filepath.Dir(npmPath)
	nodePath := filepath.Join(nodeDir, "node")

	cmd := exec.Command(nodePath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// GetNpmVersion returns the npm version string
func GetNpmVersion(npmPath string) string {
	cmd := exec.Command(npmPath, "--version")

	// Set PATH to include npm's directory so it can find node
	npmDir := filepath.Dir(npmPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s:%s", npmDir, os.Getenv("PATH")))

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// detectNativeAddons checks the lockfile for indicators of native addons
func detectNativeAddons(lockfileContent string) bool {
	var lockfile map[string]interface{}
	if err := json.Unmarshal([]byte(lockfileContent), &lockfile); err != nil {
		return false // Conservative: can't parse, assume no native addons
	}

	// Check packages in lockfile v2/v3 format
	packages, ok := lockfile["packages"].(map[string]interface{})
	if !ok {
		return false
	}

	for _, pkg := range packages {
		pkgMap, ok := pkg.(map[string]interface{})
		if !ok {
			continue
		}

		// Check for gypfile flag (explicit native addon marker)
		if gypfile, ok := pkgMap["gypfile"].(bool); ok && gypfile {
			return true
		}

		// Check for hasInstallScript (often indicates native compilation)
		if hasInstall, ok := pkgMap["hasInstallScript"].(bool); ok && hasInstall {
			return true
		}

		// Check for node-gyp in dependencies
		if deps, ok := pkgMap["dependencies"].(map[string]interface{}); ok {
			if _, hasNodeGyp := deps["node-gyp"]; hasNodeGyp {
				return true
			}
		}
	}

	return false
}
