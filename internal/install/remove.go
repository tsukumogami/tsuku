package install

import (
	"fmt"
	"os"
)

// Remove removes an installed tool
func (m *Manager) Remove(name string) error {
	// 1. Find installed version
	tools, err := m.List()
	if err != nil {
		return fmt.Errorf("failed to list installed tools: %w", err)
	}

	var version string
	for _, tool := range tools {
		if tool.Name == name {
			version = tool.Version
			break
		}
	}

	if version == "" {
		return fmt.Errorf("tool %s is not installed", name)
	}

	// 2. Remove tool directory
	toolDir := m.config.ToolDir(name, version)
	if err := os.RemoveAll(toolDir); err != nil {
		return fmt.Errorf("failed to remove tool directory: %w", err)
	}

	// 3. Remove symlink if it points to this tool
	symlinkPath := m.config.CurrentSymlink(name)
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("failed to remove symlink: %w", err)
		}
	}

	return nil
}
