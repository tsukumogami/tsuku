package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstalledTool represents an installed tool
type InstalledTool struct {
	Name    string
	Version string
	Path    string
}

// List returns a list of all installed tools (excluding hidden tools)
func (m *Manager) List() ([]InstalledTool, error) {
	return m.ListWithOptions(false)
}

// ListAll returns a list of all installed tools including hidden ones
func (m *Manager) ListAll() ([]InstalledTool, error) {
	return m.ListWithOptions(true)
}

// ListWithOptions returns a list of installed tools with option to include hidden
func (m *Manager) ListWithOptions(includeHidden bool) ([]InstalledTool, error) {
	// Ensure tools directory exists
	if _, err := os.Stat(m.config.ToolsDir); os.IsNotExist(err) {
		return []InstalledTool{}, nil
	}

	// Load state to check for hidden tools
	state, err := m.state.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	entries, err := os.ReadDir(m.config.ToolsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools directory: %w", err)
	}

	var tools []InstalledTool

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "current" {
			continue
		}

		// Expected format: name-version
		// We need to find the last hyphen to separate name and version
		name := entry.Name()
		lastHyphen := strings.LastIndex(name, "-")

		if lastHyphen == -1 || lastHyphen == 0 || lastHyphen == len(name)-1 {
			// Invalid format, skip
			continue
		}

		toolName := name[:lastHyphen]
		toolVersion := name[lastHyphen+1:]

		// Check if tool is hidden (unless we're including hidden)
		if !includeHidden {
			if toolState, exists := state.Installed[toolName]; exists && toolState.IsHidden {
				continue
			}
		}

		tools = append(tools, InstalledTool{
			Name:    toolName,
			Version: toolVersion,
			Path:    filepath.Join(m.config.ToolsDir, name),
		})
	}

	return tools, nil
}
