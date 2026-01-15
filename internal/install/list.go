package install

import (
	"fmt"
	"os"
	"sort"
)

// InstalledTool represents an installed tool version
type InstalledTool struct {
	Name     string
	Version  string
	Path     string
	IsActive bool // Whether this is the currently active version
}

// List returns a list of all installed tool versions (excluding hidden tools)
func (m *Manager) List() ([]InstalledTool, error) {
	return m.ListWithOptions(false)
}

// ListAll returns a list of all installed tool versions including hidden ones
func (m *Manager) ListAll() ([]InstalledTool, error) {
	return m.ListWithOptions(true)
}

// ListWithOptions returns a list of installed tool versions with option to include hidden.
// Returns one entry per installed version, sorted by tool name then version.
func (m *Manager) ListWithOptions(includeHidden bool) ([]InstalledTool, error) {
	// Load state to get version information
	state, err := m.state.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	var tools []InstalledTool

	// Iterate over all tools in state
	for toolName, toolState := range state.Installed {
		// Check if tool is hidden (unless we're including hidden)
		if !includeHidden && toolState.IsHidden {
			continue
		}

		// Add entry for each installed version
		for version := range toolState.Versions {
			toolDir := m.config.ToolDir(toolName, version)

			// Verify directory exists (skip stale state entries)
			if _, err := os.Stat(toolDir); os.IsNotExist(err) {
				continue
			}

			tools = append(tools, InstalledTool{
				Name:     toolName,
				Version:  version,
				Path:     toolDir,
				IsActive: version == toolState.ActiveVersion,
			})
		}
	}

	// Sort by tool name, then by version
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Name != tools[j].Name {
			return tools[i].Name < tools[j].Name
		}
		return tools[i].Version < tools[j].Version
	})

	return tools, nil
}

// InstalledApp represents an installed macOS application bundle
type InstalledApp struct {
	Name               string // Tool name from recipe
	Version            string // Installed version
	AppPath            string // Path to installed .app bundle ($TSUKU_HOME/apps/)
	ApplicationSymlink string // Path to ~/Applications symlink (if created)
	IsActive           bool   // Whether this is the currently active version
}

// ListApps returns a list of installed macOS application bundles.
// Apps are identified by having a non-empty AppPath in their VersionState.
func (m *Manager) ListApps() ([]InstalledApp, error) {
	state, err := m.state.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	var apps []InstalledApp

	for toolName, toolState := range state.Installed {
		for version, versionState := range toolState.Versions {
			// Skip entries without AppPath (not an app)
			if versionState.AppPath == "" {
				continue
			}

			// Verify .app bundle exists (skip stale state entries)
			if _, err := os.Stat(versionState.AppPath); os.IsNotExist(err) {
				continue
			}

			apps = append(apps, InstalledApp{
				Name:               toolName,
				Version:            version,
				AppPath:            versionState.AppPath,
				ApplicationSymlink: versionState.ApplicationSymlink,
				IsActive:           version == toolState.ActiveVersion,
			})
		}
	}

	// Sort by name, then version
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Name != apps[j].Name {
			return apps[i].Name < apps[j].Name
		}
		return apps[i].Version < apps[j].Version
	})

	return apps, nil
}
