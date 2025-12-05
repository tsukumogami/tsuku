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
type InstallBinariesAction struct{}

// Name returns the action name
func (a *InstallBinariesAction) Name() string {
	return "install_binaries"
}

// Execute installs binaries to the installation directory
//
// Parameters:
//   - binaries (required): List of binary mappings [{src: "kubectl", dest: "bin/kubectl"}]
//   - install_mode (optional): "binaries" (default), "directory", or "directory_wrapped"
func (a *InstallBinariesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get binaries list (required)
	binariesRaw, ok := params["binaries"]
	if !ok {
		return fmt.Errorf("install_binaries action requires 'binaries' parameter")
	}

	// Parse binaries list
	binaries, err := a.parseBinaries(binariesRaw)
	if err != nil {
		return fmt.Errorf("failed to parse binaries: %w", err)
	}

	// Get install_mode parameter (default: "binaries")
	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries"
	}

	// Enforce verification for directory-based installs (defense in depth)
	// This check also exists in composite actions (github_archive, download_archive),
	// but we enforce it here to prevent bypass via direct install_binaries usage
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" {
		return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section with a command to ensure the installation works correctly", installMode)
	}

	// Route to appropriate installation method
	switch installMode {
	case "binaries":
		return a.installBinaries(ctx, binaries)
	case "directory":
		return a.installDirectoryWithSymlinks(ctx, binaries)
	case "directory_wrapped":
		return fmt.Errorf("directory_wrapped mode not yet implemented (Phase 2)")
	default:
		return fmt.Errorf("invalid install_mode '%s': must be 'binaries', 'directory', or 'directory_wrapped'", installMode)
	}
}

// installBinaries implements the traditional binary-only installation
func (a *InstallBinariesAction) installBinaries(ctx *ExecutionContext, binaries []recipe.BinaryMapping) error {
	// Validate all binary paths for security
	for _, binary := range binaries {
		if err := a.validateBinaryPath(binary.Src); err != nil {
			return err
		}
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir)

	fmt.Printf("   Installing %d binary(ies)\n", len(binaries))

	for _, binary := range binaries {
		src := ExpandVars(binary.Src, vars)
		dest := ExpandVars(binary.Dest, vars)

		srcPath := filepath.Join(ctx.WorkDir, src)
		destPath := filepath.Join(ctx.InstallDir, dest)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		// Copy binary
		if err := a.copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("failed to install %s: %w", src, err)
		}

		// Make executable
		if err := os.Chmod(destPath, 0755); err != nil {
			return fmt.Errorf("failed to chmod %s: %w", dest, err)
		}

		fmt.Printf("   ✓ Installed: %s → %s\n", src, dest)
	}

	return nil
}

// parseBinaries parses the binaries parameter
// Supports both array of maps and array of strings
func (a *InstallBinariesAction) parseBinaries(raw interface{}) ([]recipe.BinaryMapping, error) {
	// Handle []interface{} from TOML
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("binaries must be an array")
	}

	var result []recipe.BinaryMapping

	for i, item := range arr {
		switch v := item.(type) {
		case map[string]interface{}:
			// Parse as {src: "...", dest: "..."}
			src, ok := v["src"].(string)
			if !ok {
				return nil, fmt.Errorf("binary %d: 'src' must be a string", i)
			}
			dest, ok := v["dest"].(string)
			if !ok {
				return nil, fmt.Errorf("binary %d: 'dest' must be a string", i)
			}
			result = append(result, recipe.BinaryMapping{Src: src, Dest: dest})

		case string:
			// Shorthand: just binary path, install to bin/<basename>
			// e.g., "dist/sam" → bin/sam
			basename := filepath.Base(v)
			result = append(result, recipe.BinaryMapping{
				Src:  v,
				Dest: filepath.Join("bin", basename),
			})

		default:
			return nil, fmt.Errorf("binary %d: invalid type %T", i, item)
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
func (a *InstallBinariesAction) installDirectoryWithSymlinks(ctx *ExecutionContext, binaries []recipe.BinaryMapping) error {
	// Validate all binary paths for security
	for _, binary := range binaries {
		if err := a.validateBinaryPath(binary.Src); err != nil {
			return err
		}
	}

	fmt.Printf("   Installing directory tree to: %s\n", ctx.InstallDir)

	// Copy entire WorkDir to InstallDir (.install/)
	// The install manager expects to find the full tree in workDir/.install
	fmt.Printf("   → Copying directory tree...\n")
	if err := CopyDirectory(ctx.WorkDir, ctx.InstallDir); err != nil {
		return fmt.Errorf("failed to copy directory tree: %w", err)
	}

	fmt.Printf("   ✓ Directory tree copied to %s\n", ctx.InstallDir)
	fmt.Printf("   ✓ %d binary(ies) will be symlinked: %v\n", len(binaries), extractBinaryNames(binaries))

	return nil
}

// extractBinaryNames extracts just the binary names from BinaryMapping for display
func extractBinaryNames(binaries []recipe.BinaryMapping) []string {
	names := make([]string, len(binaries))
	for i, b := range binaries {
		names[i] = filepath.Base(b.Src)
	}
	return names
}
