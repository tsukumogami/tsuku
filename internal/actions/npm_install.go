package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NpmInstallAction implements npm package installation with --prefix isolation
type NpmInstallAction struct{}

// Name returns the action name
func (a *NpmInstallAction) Name() string {
	return "npm_install"
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

	cmd := exec.Command(npmPath, "install", "-g", fmt.Sprintf("--prefix=%s", installDir), packageSpec)

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
