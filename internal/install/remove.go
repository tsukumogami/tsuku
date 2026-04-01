package install

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/shellenv"
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

	// Execute cleanup actions before removing the directory.
	// Collect other versions' cleanup paths so we don't delete shared resources.
	affectedShells := m.executeCleanupActions(name, version, versionState.CleanupActions, toolState)

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

	// Rebuild shell cache for affected shells
	m.rebuildShellCaches(affectedShells)

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
			binaries = []string{filepath.Join("bin", name)}
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

	// Execute cleanup actions for all versions. When removing all versions,
	// no cross-version safety check is needed -- everything goes.
	allShells := make(map[string]bool)
	for _, vs := range toolState.Versions {
		for _, ca := range vs.CleanupActions {
			m.executeSingleCleanup(ca)
			if shell := shellFromCleanupPath(ca.Path); shell != "" {
				allShells[shell] = true
			}
		}
	}

	// Remove all version directories
	for version := range toolState.Versions {
		toolDir := m.config.ToolDir(name, version)
		if err := os.RemoveAll(toolDir); err != nil {
			return fmt.Errorf("failed to remove version %s: %w", version, err)
		}
	}

	// Rebuild shell cache for affected shells
	m.rebuildShellCaches(allShells)

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

// executeCleanupActions runs cleanup actions for a version being removed.
// It skips any cleanup path that another version of the same tool also references
// (multi-version safety). Returns the set of shell names affected for cache rebuild.
func (m *Manager) executeCleanupActions(name, version string, actions []CleanupAction, toolState *ToolState) map[string]bool {
	if len(actions) == 0 {
		return nil
	}

	// Build set of cleanup paths owned by other versions of this tool.
	otherPaths := make(map[string]bool)
	for v, vs := range toolState.Versions {
		if v == version {
			continue
		}
		for _, ca := range vs.CleanupActions {
			otherPaths[ca.Path] = true
		}
	}

	affectedShells := make(map[string]bool)
	for _, ca := range actions {
		if otherPaths[ca.Path] {
			// Another version still references this path -- skip.
			fmt.Printf("   Cleanup: skipping %s (still referenced by another version)\n", ca.Path)
			continue
		}
		m.executeSingleCleanup(ca)
		if shell := shellFromCleanupPath(ca.Path); shell != "" {
			affectedShells[shell] = true
		}
	}
	return affectedShells
}

// executeSingleCleanup performs one cleanup action. Failures log a warning
// and never block removal.
func (m *Manager) executeSingleCleanup(ca CleanupAction) {
	absPath := filepath.Join(m.config.HomeDir, ca.Path)

	var err error
	switch ca.Action {
	case "delete_file":
		err = os.Remove(absPath)
	case "delete_dir":
		err = os.RemoveAll(absPath)
	default:
		fmt.Printf("   Warning: unknown cleanup action %q for %s\n", ca.Action, ca.Path)
		return
	}

	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("   Warning: cleanup %s %s failed: %v\n", ca.Action, ca.Path, err)
	}
}

// shellFromCleanupPath extracts the shell extension from a shell.d cleanup path.
// Returns "" if the path doesn't look like a shell.d file.
func shellFromCleanupPath(path string) string {
	if !strings.HasPrefix(path, "share/shell.d/") {
		return ""
	}
	ext := filepath.Ext(path)
	if ext == "" {
		return ""
	}
	return ext[1:] // strip leading dot
}

// rebuildShellCaches rebuilds shell init caches for the given set of shells.
func (m *Manager) rebuildShellCaches(shells map[string]bool) {
	for shell := range shells {
		if err := shellenv.RebuildShellCache(m.config.HomeDir, shell); err != nil {
			fmt.Printf("   Warning: failed to rebuild shell cache for %s: %v\n", shell, err)
		}
	}
}
