package install

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Remove removes an installed tool (legacy method - removes active version only)
// Deprecated: Use RemoveVersion or RemoveAllVersions instead
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

// RemoveVersion removes a specific version of a tool.
// If the removed version was active, switches to the most recently installed remaining version.
// If this was the last version, removes the tool entirely from state.
func (m *Manager) RemoveVersion(name, version string) error {
	// Validate version string to prevent path traversal attacks
	if err := ValidateVersionString(version); err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}

	// Get tool state
	toolState, err := m.state.GetToolState(name)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if toolState == nil {
		return fmt.Errorf("tool %q is not installed", name)
	}

	// Check if version exists
	versionState, exists := toolState.Versions[version]
	if !exists {
		return m.versionNotInstalledError(name, version, toolState)
	}

	// Remove version directory
	toolDir := m.config.ToolDir(name, version)
	if err := os.RemoveAll(toolDir); err != nil {
		return fmt.Errorf("failed to remove tool directory: %w", err)
	}

	// Remove .app bundle and ~/Applications symlink if this was an app
	if versionState.AppPath != "" {
		_ = os.RemoveAll(versionState.AppPath)
	}
	if versionState.ApplicationSymlink != "" {
		_ = os.Remove(versionState.ApplicationSymlink)
	}

	// Check if this was the last version
	if len(toolState.Versions) == 1 {
		// Last version - remove entire tool
		return m.removeToolEntirely(name, toolState)
	}

	// Track if we need to switch active version
	wasActive := toolState.ActiveVersion == version
	var newActiveVersion string

	// Update state - remove version from map
	err = m.state.UpdateTool(name, func(ts *ToolState) {
		delete(ts.Versions, version)
		if wasActive {
			// Switch to most recently installed remaining version
			newActiveVersion = getMostRecentVersion(ts.Versions)
			ts.ActiveVersion = newActiveVersion
			// Update legacy fields
			ts.Version = newActiveVersion
			if vs, ok := ts.Versions[newActiveVersion]; ok {
				ts.Binaries = vs.Binaries
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// If active version was removed, update symlinks to point to new active version
	if wasActive && newActiveVersion != "" {
		// Reload state to get binaries for new active version
		toolState, err = m.state.GetToolState(name)
		if err != nil {
			return fmt.Errorf("failed to reload state: %w", err)
		}
		binaries := toolState.Versions[newActiveVersion].Binaries
		if len(binaries) == 0 {
			binaries = []string{name}
		}
		if err := m.createSymlinksForBinaries(name, newActiveVersion, binaries); err != nil {
			return fmt.Errorf("failed to update symlinks: %w", err)
		}
	}

	return nil
}

// RemoveAllVersions removes all versions of a tool.
func (m *Manager) RemoveAllVersions(name string) error {
	// Get tool state
	toolState, err := m.state.GetToolState(name)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if toolState == nil {
		return fmt.Errorf("tool %q is not installed", name)
	}

	// Remove all version directories
	for version := range toolState.Versions {
		toolDir := m.config.ToolDir(name, version)
		if err := os.RemoveAll(toolDir); err != nil {
			return fmt.Errorf("failed to remove version %s: %w", version, err)
		}
	}

	// Remove symlinks and state
	return m.removeToolEntirely(name, toolState)
}

// removeToolEntirely removes all symlinks and state for a tool.
func (m *Manager) removeToolEntirely(name string, toolState *ToolState) error {
	// Collect all binaries to remove symlinks
	binaries := make(map[string]bool)

	// From legacy field
	for _, b := range toolState.Binaries {
		binaries[filepath.Base(b)] = true
	}

	// From each version
	for _, vs := range toolState.Versions {
		for _, b := range vs.Binaries {
			binaries[filepath.Base(b)] = true
		}

		// Remove .app bundles and ~/Applications symlinks
		if vs.AppPath != "" {
			_ = os.RemoveAll(vs.AppPath) // Remove the .app bundle
		}
		if vs.ApplicationSymlink != "" {
			_ = os.Remove(vs.ApplicationSymlink) // Remove ~/Applications symlink
		}
	}

	// Fallback to tool name if no binaries found
	if len(binaries) == 0 {
		binaries[name] = true
	}

	// Remove symlinks
	for binaryName := range binaries {
		symlinkPath := m.config.CurrentSymlink(binaryName)
		_ = os.Remove(symlinkPath) // Ignore errors - symlink may not exist
	}

	// Remove from state
	return m.state.RemoveTool(name)
}

// getMostRecentVersion returns the version with the most recent InstalledAt time.
func getMostRecentVersion(versions map[string]VersionState) string {
	var mostRecent string
	var mostRecentTime time.Time

	// Sort keys for deterministic behavior when timestamps are equal
	versionKeys := make([]string, 0, len(versions))
	for v := range versions {
		versionKeys = append(versionKeys, v)
	}
	sort.Strings(versionKeys)

	for _, v := range versionKeys {
		state := versions[v]
		if mostRecent == "" || state.InstalledAt.After(mostRecentTime) {
			mostRecent = v
			mostRecentTime = state.InstalledAt
		}
	}
	return mostRecent
}
