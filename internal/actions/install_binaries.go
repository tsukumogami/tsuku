package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// InstallBinariesAction implements binary installation
type InstallBinariesAction struct{ BaseAction }

// IsDeterministic returns true because binary installation produces identical results.
func (InstallBinariesAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *InstallBinariesAction) Name() string {
	return "install_binaries"
}

// Preflight validates parameters without side effects.
func (a *InstallBinariesAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	_, hasOutputs := params["outputs"]
	_, hasBinaries := params["binaries"]

	if !hasOutputs && !hasBinaries {
		result.AddError("install_binaries action requires 'outputs' parameter")
	}
	if hasOutputs && hasBinaries {
		result.AddError("cannot specify both 'outputs' and 'binaries'; use 'outputs' only")
	}
	if _, hasBinary := params["binary"]; hasBinary {
		result.AddError("'binary' parameter is not supported; use 'outputs' array instead")
	}

	// ERROR: Empty outputs/binaries array
	outputs := a.getOutputsParam(params)
	if outputs != nil && len(outputs) == 0 {
		result.AddError("outputs array is empty; no files will be installed")
	}
	return result
}

// getOutputsParam returns the outputs list, preferring 'outputs' over deprecated 'binaries'.
func (a *InstallBinariesAction) getOutputsParam(params map[string]interface{}) []interface{} {
	if outputsRaw, ok := params["outputs"]; ok {
		if outputs, ok := outputsRaw.([]interface{}); ok {
			return outputs
		}
	}
	if binariesRaw, ok := params["binaries"]; ok {
		if binaries, ok := binariesRaw.([]interface{}); ok {
			return binaries
		}
	}
	return nil
}

// Execute installs binaries to the installation directory
//
// Parameters:
//   - outputs (required): List of output mappings [{src: "kubectl", dest: "bin/kubectl"}]
//   - binaries (deprecated): Alias for outputs, use 'outputs' instead
//   - executables (optional): List of files to make executable; if empty, inferred from path (bin/* = executable)
//   - install_mode (optional): "binaries" (default), "directory", or "directory_wrapped"
func (a *InstallBinariesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get outputs list (required) - prefer 'outputs' over deprecated 'binaries'
	outputsRaw := a.getOutputsParam(params)
	if outputsRaw == nil {
		return fmt.Errorf("install_binaries action requires 'outputs' parameter")
	}

	// Parse outputs list
	outputs, err := a.parseOutputs(outputsRaw)
	if err != nil {
		return fmt.Errorf("failed to parse outputs: %w", err)
	}

	// Get explicit executables list (optional)
	explicitExecutables, _ := GetStringSlice(params, "executables")

	// Get install_mode parameter (default: "binaries")
	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries"
	}

	// Enforce verification for directory-based installs (defense in depth)
	// This check also exists in composite actions (github_archive, download_archive),
	// but we enforce it here to prevent bypass via direct install_binaries usage
	// Libraries are exempt since they cannot be run directly to verify
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	isLibrary := ctx.Recipe.Metadata.Type == "library"
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" && !isLibrary {
		return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section with a command to ensure the installation works correctly", installMode)
	}

	// Determine which files should be executable
	executables := DetermineExecutables(outputs, explicitExecutables)

	// Route to appropriate installation method
	switch installMode {
	case "binaries":
		return a.installBinariesMode(ctx, outputs, executables)
	case "directory":
		return a.installDirectoryWithSymlinks(ctx, outputs)
	case "directory_wrapped":
		return fmt.Errorf("directory_wrapped mode not yet implemented (Phase 2)")
	default:
		return fmt.Errorf("invalid install_mode '%s': must be 'binaries', 'directory', or 'directory_wrapped'", installMode)
	}
}

// DetermineExecutables returns the list of files that should be made executable.
// If an explicit executables list is provided, use it.
// Otherwise, infer from path: files in bin/ are executable.
func DetermineExecutables(outputs []recipe.BinaryMapping, explicitExecutables []string) []string {
	if len(explicitExecutables) > 0 {
		return explicitExecutables
	}

	var result []string
	for _, output := range outputs {
		// Use Dest path for checking since that's where it will be installed
		if strings.HasPrefix(output.Dest, "bin/") || strings.HasPrefix(output.Dest, "bin"+string(filepath.Separator)) {
			result = append(result, output.Dest)
		}
	}
	return result
}

// installBinariesMode implements the traditional binary-only installation
func (a *InstallBinariesAction) installBinariesMode(ctx *ExecutionContext, outputs []recipe.BinaryMapping, executables []string) error {
	// Validate all output paths for security
	for _, output := range outputs {
		if err := a.validateBinaryPath(output.Src); err != nil {
			return err
		}
	}

	// Build set of executable paths for quick lookup
	executableSet := make(map[string]bool)
	for _, exe := range executables {
		executableSet[exe] = true
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)

	// Log installation details
	logger := ctx.Log()
	logger.Debug("install_binaries action starting",
		"count", len(outputs),
		"executables", len(executables),
		"installDir", ctx.InstallDir)

	fmt.Printf("   Installing %d file(s)\n", len(outputs))

	for _, output := range outputs {
		src := ExpandVars(output.Src, vars)
		dest := ExpandVars(output.Dest, vars)

		srcPath := filepath.Join(ctx.WorkDir, src)
		destPath := filepath.Join(ctx.InstallDir, dest)

		logger.Debug("installing file",
			"src", src,
			"srcPath", srcPath,
			"dest", dest,
			"destPath", destPath)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		// Copy file
		if err := a.copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", src, err)
		}

		// Make executable only if it's in the executables list
		if executableSet[dest] {
			if err := os.Chmod(destPath, 0755); err != nil {
				return fmt.Errorf("failed to chmod %s: %w", dest, err)
			}
			logger.Debug("file installed as executable", "dest", destPath)
			fmt.Printf("   ✓ Installed (executable): %s → %s\n", src, dest)
		} else {
			logger.Debug("file installed", "dest", destPath)
			fmt.Printf("   ✓ Installed: %s → %s\n", src, dest)
		}
	}

	return nil
}

// parseOutputs parses the outputs parameter
// Supports both array of maps and array of strings
func (a *InstallBinariesAction) parseOutputs(raw []interface{}) ([]recipe.BinaryMapping, error) {
	var result []recipe.BinaryMapping

	for i, item := range raw {
		switch v := item.(type) {
		case map[string]interface{}:
			// Parse as {src: "...", dest: "..."}
			src, ok := v["src"].(string)
			if !ok {
				return nil, fmt.Errorf("output %d: 'src' must be a string", i)
			}
			dest, ok := v["dest"].(string)
			if !ok {
				return nil, fmt.Errorf("output %d: 'dest' must be a string", i)
			}
			result = append(result, recipe.BinaryMapping{Src: src, Dest: dest})

		case string:
			// Shorthand: just output path, install to bin/<basename>
			// e.g., "dist/sam" → bin/sam
			basename := filepath.Base(v)
			result = append(result, recipe.BinaryMapping{
				Src:  v,
				Dest: filepath.Join("bin", basename),
			})

		default:
			return nil, fmt.Errorf("output %d: invalid type %T", i, item)
		}
	}

	return result, nil
}

// copyFile copies a file from src to dst
func (a *InstallBinariesAction) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// validateBinaryPath validates that a binary path doesn't contain directory traversal attempts
// This prevents security issues where malicious recipes could specify paths like "../../etc/passwd"
func (a *InstallBinariesAction) validateBinaryPath(binaryPath string) error {
	// Check for directory traversal patterns
	if strings.Contains(binaryPath, "..") {
		return fmt.Errorf("binary path cannot contain '..': %s", binaryPath)
	}

	// Check for absolute paths (binaries should be relative to WorkDir)
	if filepath.IsAbs(binaryPath) {
		return fmt.Errorf("binary path must be relative, not absolute: %s", binaryPath)
	}

	return nil
}

// createSymlink creates a relative symlink from linkPath to targetPath
// The symlink uses relative path computation to avoid hardcoded absolute paths
// SECURITY: Uses atomic rename to prevent TOCTOU race conditions
func (a *InstallBinariesAction) createSymlink(targetPath, linkPath string) error {
	// Ensure parent directory of link exists
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return fmt.Errorf("failed to create symlink directory: %w", err)
	}

	// Compute relative path from link to target
	// linkPath: ~/.tsuku/tools/.install/bin/java
	// targetPath: ~/.tsuku/tools/liberica-25.0.1/bin/java
	// Result: ../../liberica-25.0.1/bin/java
	relPath, err := filepath.Rel(filepath.Dir(linkPath), targetPath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Use atomic symlink creation to prevent TOCTOU race conditions
	// Create temporary symlink, then atomically rename to final location
	tmpLink := linkPath + ".tmp"

	// Remove any existing temp symlink
	os.Remove(tmpLink)

	// Create the symlink at temporary location
	if err := os.Symlink(relPath, tmpLink); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Atomically rename to final location (POSIX guarantees atomic rename)
	if err := os.Rename(tmpLink, linkPath); err != nil {
		os.Remove(tmpLink) // Clean up temp file on failure
		return fmt.Errorf("failed to rename symlink: %w", err)
	}

	return nil
}

// installDirectoryWithSymlinks implements directory-based installation
// This method copies the entire directory tree to InstallDir (workDir/.install)
// The install manager will then copy from .install to the permanent location
func (a *InstallBinariesAction) installDirectoryWithSymlinks(ctx *ExecutionContext, outputs []recipe.BinaryMapping) error {
	// Validate all output paths for security
	for _, output := range outputs {
		if err := a.validateBinaryPath(output.Src); err != nil {
			return err
		}
	}

	// Log directory installation details
	logger := ctx.Log()
	logger.Debug("install_binaries (directory mode) starting",
		"workDir", ctx.WorkDir,
		"installDir", ctx.InstallDir,
		"outputCount", len(outputs))

	fmt.Printf("   Installing directory tree to: %s\n", ctx.InstallDir)

	// Copy entire WorkDir to InstallDir (.install/), excluding the .install subdirectory
	// to prevent recursive copy (since InstallDir is workDir/.install)
	// The install manager expects to find the full tree in workDir/.install
	fmt.Printf("   → Copying directory tree...\n")
	if err := CopyDirectoryExcluding(ctx.WorkDir, ctx.InstallDir, ".install"); err != nil {
		return fmt.Errorf("failed to copy directory tree: %w", err)
	}

	fmt.Printf("   ✓ Directory tree copied to %s\n", ctx.InstallDir)
	fmt.Printf("   ✓ %d output(s) will be symlinked: %v\n", len(outputs), extractOutputNames(outputs))

	return nil
}

// extractOutputNames extracts just the output names from BinaryMapping for display
func extractOutputNames(outputs []recipe.BinaryMapping) []string {
	names := make([]string, len(outputs))
	for i, o := range outputs {
		names[i] = filepath.Base(o.Src)
	}
	return names
}
