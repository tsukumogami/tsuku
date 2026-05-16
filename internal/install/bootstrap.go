package install

import (
	"context"
	"fmt"
)

// CheckAndExposeHidden checks if a tool is installed as hidden and exposes it
// if requested. This is used when the user explicitly runs `tsuku install npm`.
// ctx is threaded through to ExposeHidden for cancellation hooks.
func CheckAndExposeHidden(ctx context.Context, mgr *Manager, toolName string) (bool, error) {
	hidden, err := IsHidden(mgr.config, toolName)
	if err != nil {
		return false, err
	}

	if !hidden {
		return false, nil
	}

	// Tool is hidden, expose it
	if err := ExposeHidden(ctx, mgr, toolName); err != nil {
		return false, fmt.Errorf("failed to expose hidden tool: %w", err)
	}

	return true, nil
}
