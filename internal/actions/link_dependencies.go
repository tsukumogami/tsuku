package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LinkDependenciesAction creates symlinks from a tool's lib/ directory to shared library location
type LinkDependenciesAction struct{}

// Name returns the action name
func (a *LinkDependenciesAction) Name() string {
	return "link_dependencies"
}

// Execute creates symlinks from tool/lib/ to libs/{name}-{version}/lib/
//
// Parameters:
//   - library (required): Library name (e.g., "libyaml")
//   - version (required): Library version (e.g., "0.2.5")
//
// The action creates relative symlinks so the installation is relocatable.
// Example: ruby-3.4.0/lib/libyaml.so.2 -> ../../../libs/libyaml-0.2.5/lib/libyaml.so.2
func (a *LinkDependenciesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get library name (required)
	libraryRaw, ok := params["library"]
	if !ok {
		return fmt.Errorf("link_dependencies action requires 'library' parameter")
	}
	library, ok := libraryRaw.(string)
	if !ok {
		return fmt.Errorf("'library' parameter must be a string")
	}

	// Get version (required)
	versionRaw, ok := params["version"]
	if !ok {
		return fmt.Errorf("link_dependencies action requires 'version' parameter")
	}
	version, ok := versionRaw.(string)
	if !ok {
		return fmt.Errorf("'version' parameter must be a string")
	}

	// Validate library name for security
	if err := a.validateLibraryName(library); err != nil {
		return err
	}

	// Validate version for security
	if err := a.validateVersion(version); err != nil {
		return err
	}

	// Construct paths
	// Source: libs/{library}-{version}/lib/
	// We need to find this relative to ToolsDir (which is $TSUKU_HOME/tools)
	// LibsDir is $TSUKU_HOME/libs
	libsDirName := "libs"
	libVersionDir := fmt.Sprintf("%s-%s", library, version)

	// The absolute path to the library's lib directory
	// ToolsDir is $TSUKU_HOME/tools, so libs is at ../libs relative to tools
	toolsParent := filepath.Dir(ctx.ToolsDir) // $TSUKU_HOME
	srcLibDir := filepath.Join(toolsParent, libsDirName, libVersionDir, "lib")

	// Verify source library directory exists
	if _, err := os.Stat(srcLibDir); os.IsNotExist(err) {
		return fmt.Errorf("library directory does not exist: %s", srcLibDir)
	}

	// Destination: tool's lib directory
	destLibDir := filepath.Join(ctx.ToolInstallDir, "lib")

	// Ensure destination lib directory exists
	if err := os.MkdirAll(destLibDir, 0755); err != nil {
		return fmt.Errorf("failed to create lib directory: %w", err)
	}

	// Calculate relative path from destLibDir to srcLibDir
	// From tools/{tool}-{version}/lib/ to libs/{library}-{version}/lib/
	// That's: ../../../libs/{library}-{version}/lib/
	relPath, err := filepath.Rel(destLibDir, srcLibDir)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Enumerate files in source library directory
	entries, err := os.ReadDir(srcLibDir)
	if err != nil {
		return fmt.Errorf("failed to read library directory: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("library directory is empty: %s", srcLibDir)
	}

	fmt.Printf("   Linking %d library file(s) from %s\n", len(entries), libVersionDir)

	for _, entry := range entries {
		srcFile := filepath.Join(srcLibDir, entry.Name())
		destFile := filepath.Join(destLibDir, entry.Name())
		symlinkTarget := filepath.Join(relPath, entry.Name())

		// Check for collision
		if info, err := os.Lstat(destFile); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				// It's a symlink - check if it points to our target
				existingTarget, readErr := os.Readlink(destFile)
				if readErr == nil && existingTarget == symlinkTarget {
					// Already linked correctly, skip
					fmt.Printf("   - Already linked: %s\n", entry.Name())
					continue
				}
			}
			// File exists but is not our symlink - collision
			return fmt.Errorf("collision: %s already exists and is not a symlink to %s", destFile, symlinkTarget)
		}

		// Get info about source to preserve symlink chains
		srcInfo, err := os.Lstat(srcFile)
		if err != nil {
			return fmt.Errorf("failed to stat source file %s: %w", entry.Name(), err)
		}

		if srcInfo.Mode()&os.ModeSymlink != 0 {
			// Source is a symlink - read its target and create equivalent symlink
			// This preserves the library versioning chain (e.g., libyaml.so.2 -> libyaml.so.2.0.9)
			srcTarget, err := os.Readlink(srcFile)
			if err != nil {
				return fmt.Errorf("failed to read source symlink %s: %w", entry.Name(), err)
			}

			// Validate symlink target doesn't escape lib/ directory
			if err := a.validateSymlinkTarget(srcTarget, entry.Name()); err != nil {
				return err
			}

			// Create symlink with same target (relative within lib/)
			if err := os.Symlink(srcTarget, destFile); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", entry.Name(), err)
			}
			fmt.Printf("   + Linked (symlink): %s -> %s\n", entry.Name(), srcTarget)
		} else {
			// Source is a regular file - create symlink to it
			if err := os.Symlink(symlinkTarget, destFile); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", entry.Name(), err)
			}
			fmt.Printf("   + Linked: %s\n", entry.Name())
		}
	}

	return nil
}

// validateLibraryName validates that the library name is safe
func (a *LinkDependenciesAction) validateLibraryName(name string) error {
	if name == "" {
		return fmt.Errorf("library name cannot be empty")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("library name cannot contain '..': %s", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("library name cannot contain path separators: %s", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("library name cannot be an absolute path: %s", name)
	}
	return nil
}

// validateVersion validates that the version string is safe
func (a *LinkDependenciesAction) validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	if strings.Contains(version, "..") {
		return fmt.Errorf("version cannot contain '..': %s", version)
	}
	if strings.Contains(version, "/") || strings.Contains(version, "\\") {
		return fmt.Errorf("version cannot contain path separators: %s", version)
	}
	return nil
}

// validateSymlinkTarget validates that a symlink target doesn't escape the lib/ directory
func (a *LinkDependenciesAction) validateSymlinkTarget(target, symlinkName string) error {
	// Absolute symlinks could point anywhere - reject them
	if filepath.IsAbs(target) {
		return fmt.Errorf("source symlink %s has absolute target which could escape lib directory: %s", symlinkName, target)
	}

	// Check for directory traversal
	if strings.Contains(target, "..") {
		return fmt.Errorf("source symlink %s has target with path traversal: %s", symlinkName, target)
	}

	return nil
}
