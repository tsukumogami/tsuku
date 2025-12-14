package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PipxInstallAction installs Python CLI tools using pipx with isolated venvs
type PipxInstallAction struct{ BaseAction }

// Dependencies returns pipx as install-time and python as runtime dependency.
func (PipxInstallAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"pipx"}, Runtime: []string{"python"}}
}

// Name returns the action name
func (a *PipxInstallAction) Name() string {
	return "pipx_install"
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
		// From: venvs/<package>/bin/python3
		// To: ../../../python-standalone-XXXXXXXX/bin/python3
		pythonPath := ResolvePythonStandalone()
		if pythonPath != "" {
			relPath, err := filepath.Rel(venvBinDir, pythonPath)
			if err == nil {
				_ = os.Symlink(relPath, python3Link) // Ignore error if symlink fails
			}
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
