package install

import (
	"context"
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

// ExposeHidden exposes a previously hidden tool by creating symlinks.
// This is called when the user explicitly requests a tool that's already
// installed as hidden. ctx is accepted so future cancellation hooks can be
// added incrementally; callers should pass the request-scoped ctx with
// installevents.WithSource set so any downstream publish callsites see it.
func ExposeHidden(ctx context.Context, mgr *Manager, toolName string) error {
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
	if err := mgr.createSymlinksForBinaries(ctx, toolName, toolState.Version, toolState.Binaries); err != nil {
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
