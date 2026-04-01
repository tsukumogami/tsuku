package install

import (
	"fmt"

	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// StaleCleanupActions computes the set difference of old CleanupActions minus
// new CleanupActions. These are paths that the previous version created but the
// new version no longer needs, so they should be deleted during update.
//
// Returns nil when old is empty or all old actions are covered by new.
func StaleCleanupActions(old, new []CleanupAction) []CleanupAction {
	if len(old) == 0 {
		return nil
	}

	// Build a set of (action, path) pairs from the new version
	type key struct {
		action string
		path   string
	}
	newSet := make(map[key]bool, len(new))
	for _, ca := range new {
		newSet[key{ca.Action, ca.Path}] = true
	}

	var stale []CleanupAction
	for _, ca := range old {
		if !newSet[key{ca.Action, ca.Path}] {
			stale = append(stale, ca)
		}
	}
	return stale
}

// ExecuteStaleCleanup runs stale cleanup actions and rebuilds shell caches
// for affected shells. This is used during update to clean up files that the
// old version created but the new version no longer needs.
//
// Failures log warnings and never block the update.
func (m *Manager) ExecuteStaleCleanup(staleActions []CleanupAction) {
	if len(staleActions) == 0 {
		return
	}

	affectedShells := make(map[string]bool)
	for _, ca := range staleActions {
		m.executeSingleCleanup(ca)
		if shell := shellFromCleanupPath(ca.Path); shell != "" {
			affectedShells[shell] = true
		}
	}

	// Rebuild shell caches for any shells whose init scripts changed
	for shell := range affectedShells {
		if err := shellenv.RebuildShellCache(m.config.HomeDir, shell); err != nil {
			fmt.Printf("   Warning: failed to rebuild shell cache for %s: %v\n", shell, err)
		}
	}
}
