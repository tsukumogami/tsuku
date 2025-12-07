package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
)

// Manager handles tool installation to ~/.tsuku
type Manager struct {
	config *config.Config
	state  *StateManager
}

// New creates a new install manager
func New(cfg *config.Config) *Manager {
	return &Manager{
		config: cfg,
		state:  NewStateManager(cfg),
	}
}

// InstallOptions controls how a tool is installed
type InstallOptions struct {
	CreateSymlinks      bool              // Whether to create symlinks in current/
	IsHidden            bool              // Mark as hidden execution dependency
	Binaries            []string          // List of binary names this tool provides
	RuntimeDependencies map[string]string // Runtime deps: name -> version (for wrapper scripts)
}

// DefaultInstallOptions returns the default installation options
func DefaultInstallOptions() InstallOptions {
	return InstallOptions{
		CreateSymlinks: true,
		IsHidden:       false,
	}
}

// Install copies a tool from the work directory to the permanent location
// and creates a symlink in current/
func (m *Manager) Install(name, version, workDir string) error {
	return m.InstallWithOptions(name, version, workDir, DefaultInstallOptions())
}

// InstallWithOptions copies a tool from the work directory to the permanent location
// with custom options for symlink creation and visibility
func (m *Manager) InstallWithOptions(name, version, workDir string, opts InstallOptions) error {
	// Ensure directories exist
	if err := m.config.EnsureDirectories(); err != nil {
		return err
	}

	// Create tool-specific directory
	toolDir := m.config.ToolDir(name, version)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return fmt.Errorf("failed to create tool directory: %w", err)
	}

	// Copy from work directory to tool directory
	// Copy the entire .install directory to preserve full structure (bin/, lib/, share/, etc.)
	srcInstallDir := filepath.Join(workDir, ".install")

	if err := copyDir(srcInstallDir, toolDir); err != nil {
		return fmt.Errorf("failed to copy installation: %w", err)
	}

	// Fix pipx shebangs after copying to final location
	// This ensures Python script shebangs point to the venv's python in the final path
	_ = fixPipxShebangs(toolDir, m.config.ToolsDir) // Ignore errors - not all tools use pipx

	// Create symlink or wrapper in current/ (unless hidden)
	if opts.CreateSymlinks {
		if len(opts.RuntimeDependencies) > 0 {
			// Tool has runtime deps - create wrapper scripts
			if err := m.createWrappersForBinaries(name, version, opts.Binaries, opts.RuntimeDependencies); err != nil {
				return fmt.Errorf("failed to create wrappers: %w", err)
			}
			fmt.Printf("📍 Installed to: %s\n", toolDir)
			if len(opts.Binaries) > 0 {
				fmt.Printf("🔗 Wrapped %d binaries: %v\n", len(opts.Binaries), opts.Binaries)
			} else {
				fmt.Printf("🔗 Wrapped: %s\n", m.config.CurrentSymlink(name))
			}
		} else {
			// No runtime deps - use symlinks (faster)
			if err := m.createSymlinksForBinaries(name, version, opts.Binaries); err != nil {
				return fmt.Errorf("failed to create symlinks: %w", err)
			}
			fmt.Printf("📍 Installed to: %s\n", toolDir)
			if len(opts.Binaries) > 0 {
				fmt.Printf("🔗 Symlinked %d binaries: %v\n", len(opts.Binaries), opts.Binaries)
			} else {
				fmt.Printf("🔗 Symlinked: %s -> %s\n", m.config.CurrentSymlink(name), filepath.Join(toolDir, "bin", name))
			}
		}
	} else {
		fmt.Printf("📍 Installed to: %s (hidden)\n", toolDir)
	}

	// Update state
	// Note: IsExplicit and RequiredBy are handled by the caller (main.go)
	// Here we just ensure the version is recorded
	err := m.state.UpdateTool(name, func(ts *ToolState) {
		ts.Version = version
		ts.Binaries = opts.Binaries
		if opts.IsHidden {
			ts.IsHidden = true
			ts.IsExecutionDependency = true
		}
	})
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	return nil
}

// GetState returns the state manager
func (m *Manager) GetState() *StateManager {
	return m.state
}

// createSymlink creates or updates the symlink in current/ to point to the latest version
// This assumes the binary name matches the tool name (legacy behavior)
func (m *Manager) createSymlink(name, version string) error {
	return m.createBinarySymlink(name, version, name)
}

// createBinarySymlink creates a symlink for a specific binary
func (m *Manager) createBinarySymlink(toolName, version, binaryPath string) error {
	// For directory-mode installs, binaryPath is relative to tool root (e.g., "cargo/bin/cargo", "zig")
	// Extract basename for the symlink name in current/ (e.g., "cargo", "zig")
	binaryName := filepath.Base(binaryPath)
	symlinkPath := m.config.CurrentSymlink(binaryName)

	// Target is relative to tool directory (not bin/), since binaryPath is already relative
	targetPath := filepath.Join(m.config.ToolDir(toolName, version), binaryPath)

	// Remove existing symlink if it exists
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("failed to remove old symlink: %w", err)
		}
	}

	// Create new symlink
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// createSymlinksForBinaries creates symlinks for all binaries provided by a tool
func (m *Manager) createSymlinksForBinaries(toolName, version string, binaries []string) error {
	if len(binaries) == 0 {
		// Fallback to old behavior if no binaries specified
		return m.createSymlink(toolName, version)
	}

	for _, binary := range binaries {
		if err := m.createBinarySymlink(toolName, version, binary); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", binary, err)
		}
	}

	return nil
}

// createWrappersForBinaries creates wrapper scripts for all binaries provided by a tool.
// Wrapper scripts prepend runtime dependency bin directories to PATH before exec'ing the real binary.
func (m *Manager) createWrappersForBinaries(toolName, version string, binaries []string, runtimeDeps map[string]string) error {
	if len(binaries) == 0 {
		// Fallback: create wrapper for tool with same name as binary
		return m.createBinaryWrapper(toolName, version, filepath.Join("bin", toolName), runtimeDeps)
	}

	for _, binary := range binaries {
		if err := m.createBinaryWrapper(toolName, version, binary, runtimeDeps); err != nil {
			return fmt.Errorf("failed to create wrapper for %s: %w", binary, err)
		}
	}

	return nil
}

// createBinaryWrapper creates a wrapper script for a specific binary.
// The wrapper prepends runtime dependency paths to PATH and exec's the real binary.
func (m *Manager) createBinaryWrapper(toolName, version, binaryPath string, runtimeDeps map[string]string) error {
	// Validate binaryPath
	if binaryPath == "" || binaryPath == "." || binaryPath == ".." {
		return fmt.Errorf("invalid binary path: %q", binaryPath)
	}

	binaryName := filepath.Base(binaryPath)
	wrapperPath := m.config.CurrentSymlink(binaryName)

	// Build the target path (absolute)
	targetPath := filepath.Join(m.config.ToolDir(toolName, version), binaryPath)

	// Validate target path doesn't contain shell metacharacters
	if err := validateShellSafePath(targetPath); err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	// Build PATH additions from runtime deps in deterministic order
	// Sort by dependency name to ensure reproducible wrapper content
	depNames := make([]string, 0, len(runtimeDeps))
	for depName := range runtimeDeps {
		depNames = append(depNames, depName)
	}
	sort.Strings(depNames)

	var pathAdditions []string
	for _, depName := range depNames {
		depVersion := runtimeDeps[depName]
		depBinDir := m.config.ToolBinDir(depName, depVersion)

		// Validate each PATH addition
		if err := validateShellSafePath(depBinDir); err != nil {
			return fmt.Errorf("invalid dependency path for %s: %w", depName, err)
		}

		pathAdditions = append(pathAdditions, depBinDir)
	}

	// Generate wrapper script content
	content := generateWrapperScript(targetPath, pathAdditions)

	// Use atomic write: write to temp file then rename
	// This prevents TOCTOU race conditions and ensures atomicity
	tmpPath := wrapperPath + ".tmp"

	// Write to temporary file
	if err := os.WriteFile(tmpPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}

	// Atomic rename (replaces existing file atomically on Unix)
	if err := os.Rename(tmpPath, wrapperPath); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to install wrapper script: %w", err)
	}

	return nil
}

// validateShellSafePath checks that a path doesn't contain characters that could
// cause shell injection or break the wrapper script.
func validateShellSafePath(path string) error {
	// Reject paths containing characters that could break shell quoting or enable injection
	dangerous := "\n\r\"'`$\\;"
	for _, char := range dangerous {
		if strings.ContainsRune(path, char) {
			return fmt.Errorf("path contains dangerous character %q: %s", char, path)
		}
	}
	return nil
}

// generateWrapperScript creates the content of a wrapper script.
// The script prepends dependency paths to PATH and exec's the target binary.
func generateWrapperScript(targetPath string, pathAdditions []string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")

	// Only add PATH line if there are additions
	if len(pathAdditions) > 0 {
		sb.WriteString("PATH=\"")
		for i, p := range pathAdditions {
			if i > 0 {
				sb.WriteString(":")
			}
			sb.WriteString(p)
		}
		sb.WriteString(":$PATH\"\n")
	}

	sb.WriteString("exec \"")
	sb.WriteString(targetPath)
	sb.WriteString("\" \"$@\"\n")

	return sb.String()
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Check if it's a symlink
		info, err := entry.Info()
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Preserve symlink
			if err := copySymlink(srcPath, dstPath); err != nil {
				return err
			}
		} else if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copySymlink copies a symlink from src to dst, preserving the link target
func copySymlink(src, dst string) error {
	// Read the symlink target
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}

	// Remove destination if it already exists
	os.Remove(dst)

	// Create the symlink
	return os.Symlink(target, dst)
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

// fixPipxShebangs fixes Python script shebangs in pipx venvs after installation.
// pipx creates scripts with shebangs pointing to absolute paths in temp directories.
// We need to rewrite them to point to the venv's python in the final installation path.
func fixPipxShebangs(toolDir, toolsDir string) error {
	// Check if there's a venvs/ directory (indicates pipx installation)
	venvsDir := filepath.Join(toolDir, "venvs")
	if _, err := os.Stat(venvsDir); os.IsNotExist(err) {
		// Not a pipx installation, nothing to do
		return nil
	}

	// Read all venv directories
	entries, err := os.ReadDir(venvsDir)
	if err != nil {
		return fmt.Errorf("failed to read venvs directory: %w", err)
	}

	// For each venv, fix shebangs in bin/
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packageName := entry.Name()
		venvBinDir := filepath.Join(venvsDir, packageName, "bin")

		// Fix python3 symlink in venv to point to python-standalone with correct relative path
		// The symlink may be broken because it was created from the temp directory
		python3Link := filepath.Join(venvBinDir, "python3")
		if info, err := os.Lstat(python3Link); err == nil && info.Mode()&os.ModeSymlink != 0 {
			// Remove the potentially broken symlink
			os.Remove(python3Link)

			// Find python-standalone
			pythonPath := findPythonStandalone(toolsDir)
			if pythonPath != "" {
				// Create correct relative symlink from venv's bin/ to python-standalone
				relPath, err := filepath.Rel(venvBinDir, pythonPath)
				if err == nil {
					_ = os.Symlink(relPath, python3Link) // Ignore error if symlink fails
				}
			}
		}

		// Read all files in venv's bin/
		binEntries, err := os.ReadDir(venvBinDir)
		if err != nil {
			continue // Skip if bin/ doesn't exist
		}

		// Fix each executable script
		for _, binEntry := range binEntries {
			if binEntry.IsDir() {
				continue
			}

			scriptPath := filepath.Join(venvBinDir, binEntry.Name())
			if err := fixPythonShebang(scriptPath, toolDir, packageName); err != nil {
				// Silently skip non-Python scripts or files without shebangs
				continue
			}
		}
	}

	return nil
}

// findPythonStandalone finds the path to tsuku's python-standalone installation
func findPythonStandalone(toolsDir string) string {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	// Find the latest python-standalone-* directory
	var pythonDirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "python-standalone-") {
			pythonDirs = append(pythonDirs, entry.Name())
		}
	}

	if len(pythonDirs) == 0 {
		return ""
	}

	// Sort to get the latest version (lexicographically)
	// Note: We should import "sort" package
	// For now, just use the first one found or implement simple max
	latestDir := pythonDirs[0]
	for _, dir := range pythonDirs {
		if dir > latestDir {
			latestDir = dir
		}
	}

	// Return path to python3
	pythonPath := filepath.Join(toolsDir, latestDir, "bin", "python3")
	if _, err := os.Stat(pythonPath); err == nil {
		return pythonPath
	}

	return ""
}

// fixPythonShebang rewrites a Python script's shebang to point to the venv's python in the final path
func fixPythonShebang(filePath, toolDir, packageName string) error {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Check if it has a shebang
	if !strings.HasPrefix(string(data), "#!") {
		return fmt.Errorf("no shebang")
	}

	// Get the first line (shebang)
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) < 2 {
		return fmt.Errorf("invalid file format")
	}

	shebang := lines[0]
	rest := lines[1]

	// Only fix if the shebang points to a python interpreter
	// (contains "python" and points to either /tmp or an absolute path with python)
	if !strings.Contains(shebang, "python") {
		return fmt.Errorf("not a python script")
	}

	// Construct the new shebang pointing to the venv's python in the final location
	venvPythonPath := filepath.Join(toolDir, "venvs", packageName, "bin", "python3")
	newShebang := "#!" + venvPythonPath
	newContent := newShebang + "\n" + rest

	// Write back
	if err := os.WriteFile(filePath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
