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
// A path any installed version still records is never deleted, the same guard
// executeCleanupActions applies on removal. Version-keyed shell.d filenames
// make that guard load-bearing here: the old and new versions share no path, so
// every one of the old version's fragments reads as stale even though the old
// version is still installed and is the rollback target. Deleting them would
// fail silently, because CheckShellD iterates directory entries and a file that
// is simply gone produces no mismatch.
//
// Failures log warnings and never block the update.
func (m *Manager) ExecuteStaleCleanup(staleActions []CleanupAction) {
	if len(staleActions) == 0 {
		return
	}

	retained := m.recordedPaths()

	affectedShells := make(map[string]bool)
	for _, ca := range staleActions {
		if shell := shellFromCleanupPath(ca.Path); shell != "" {
			// Affected whether or not the file goes: a retained path may still
			// have changed which version owns it.
			affectedShells[shell] = true
		}
		if retained[ca.Path] {
			continue
		}
		m.executeSingleCleanup(ca)
	}

	// Rebuild shell caches for any shells whose init scripts changed
	sel := m.ShellDSelection()
	for shell := range affectedShells {
		if err := shellenv.RebuildShellCache(m.config.HomeDir, shell, sel); err != nil {
			fmt.Printf("   Warning: failed to rebuild shell cache for %s: %v\n", shell, err)
		}
	}
}

// recordedPaths is the set of cleanup paths every installed version of every
// tool still references. A path in this set belongs to something that is still
// installed and must not be deleted.
func (m *Manager) recordedPaths() map[string]bool {
	paths := make(map[string]bool)
	state, err := m.state.Load()
	if err != nil || state == nil {
		return paths
	}
	for _, ts := range state.Installed {
		for _, vs := range ts.Versions {
			for _, ca := range vs.CleanupActions {
				paths[ca.Path] = true
			}
		}
	}
	return paths
}
