package install

import (
	"fmt"

	"github.com/tsukumogami/tsuku/internal/config"
)

// HiddenInstallOptions returns options for hidden installation
func HiddenInstallOptions() InstallOptions {
	return InstallOptions{
		CreateSymlinks: false,
		IsHidden:       true,
	}
}

// ExposeHidden exposes a previously hidden tool by creating symlinks
// This is called when user explicitly requests a tool that's already installed as hidden
func ExposeHidden(mgr *Manager, toolName string) error {
	sm := mgr.state

	state, err := sm.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	toolState, exists := state.Installed[toolName]
	if !exists {
		return fmt.Errorf("tool %s is not installed", toolName)
	}

	if !toolState.IsHidden {
		return fmt.Errorf("tool %s is already visible", toolName)
	}

	// Create symlinks for all binaries this tool provides
	if err := mgr.createSymlinksForBinaries(toolName, toolState.Version, toolState.Binaries); err != nil {
		return fmt.Errorf("failed to create symlinks: %w", err)
	}

	// Update state to mark as no longer hidden and explicitly requested
	return sm.UpdateTool(toolName, func(ts *ToolState) {
		ts.IsHidden = false
		ts.IsExplicit = true // Now explicitly requested by user
	})
}

// IsHidden checks if a tool is installed as a hidden execution dependency
func IsHidden(cfg *config.Config, toolName string) (bool, error) {
	sm := NewStateManager(cfg)
	state, err := sm.Load()
	if err != nil {
		return false, fmt.Errorf("failed to load state: %w", err)
	}

	toolState, exists := state.Installed[toolName]
	if !exists {
		return false, nil
	}

	return toolState.IsHidden, nil
}
