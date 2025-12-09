package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AtomicSymlink creates or replaces a symlink atomically.
// It creates a temporary symlink in the same directory, then renames it
// over the existing symlink. This ensures no window of time where the
// symlink doesn't exist.
func AtomicSymlink(target, linkPath string) error {
	// Get the directory containing the link
	linkDir := filepath.Dir(linkPath)
	linkName := filepath.Base(linkPath)

	// Create a temporary symlink name in the same directory
	// Using .tmp suffix ensures it's in the same directory (required for atomic rename)
	tmpPath := filepath.Join(linkDir, "."+linkName+".tmp")

	// Remove any existing temporary symlink
	_ = os.Remove(tmpPath)

	// Create the temporary symlink
	if err := os.Symlink(target, tmpPath); err != nil {
		return fmt.Errorf("failed to create temporary symlink: %w", err)
	}

	// Atomically rename the temporary symlink to the final path
	// os.Rename is atomic on POSIX systems when source and destination
	// are in the same directory
	if err := os.Rename(tmpPath, linkPath); err != nil {
		// Clean up the temporary symlink on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename symlink atomically: %w", err)
	}

	return nil
}

// ValidateSymlinkTarget validates that a symlink target is within the allowed
// tools directory. This prevents path traversal attacks where a malicious
// version string could point the symlink outside $TSUKU_HOME/tools/.
func ValidateSymlinkTarget(target, toolsDir string) error {
	// Clean and resolve both paths
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	cleanToolsDir, err := filepath.Abs(toolsDir)
	if err != nil {
		return fmt.Errorf("invalid tools directory: %w", err)
	}

	// Ensure the target is within the tools directory
	// Add trailing separator to prevent matching partial directory names
	// e.g., /home/user/.tsuku/tools-malicious should not match /home/user/.tsuku/tools
	toolsDirPrefix := cleanToolsDir + string(filepath.Separator)

	if !strings.HasPrefix(cleanTarget, toolsDirPrefix) && cleanTarget != cleanToolsDir {
		return fmt.Errorf("symlink target %q is outside tools directory %q", target, toolsDir)
	}

	return nil
}
