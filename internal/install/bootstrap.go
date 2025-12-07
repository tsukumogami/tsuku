package install

import (
	"fmt"
)

// CheckAndExposeHidden checks if a tool is installed as hidden and exposes it if requested
// This is used when user explicitly runs: tsuku install npm
func CheckAndExposeHidden(mgr *Manager, toolName string) (bool, error) {
	hidden, err := IsHidden(mgr.config, toolName)
	if err != nil {
		return false, err
	}

	if !hidden {
		return false, nil
	}

	// Tool is hidden, expose it
	if err := ExposeHidden(mgr, toolName); err != nil {
		return false, fmt.Errorf("failed to expose hidden tool: %w", err)
	}

	fmt.Printf("Exposed %s - now available\n", toolName)
	return true, nil
}
