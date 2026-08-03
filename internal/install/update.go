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

// WarnFunc reports a non-fatal finding to the user. The update paths print
// through different machinery -- a TTY reporter in the foreground, a notice
// inbox in the background -- so the shared pass takes the printer rather than
// picking one.
type WarnFunc func(format string, args ...any)

// CleanupSnapshot is what one version of a tool recorded writing outside its own
// tool directory, captured before an update replaces that version.
//
// The zero value is inert: reconciling it does nothing.
type CleanupSnapshot struct {
	Tool    string
	Version string
	Actions []CleanupAction
}

// SnapshotCleanup captures the tool's active version and the cleanup actions
// that version recorded. Call it before the install that replaces the version --
// afterwards the active version is the new one and there is nothing left to
// compare against.
//
// A tool that is not installed, or whose active version records nothing, yields
// the zero snapshot.
func (m *Manager) SnapshotCleanup(tool string) CleanupSnapshot {
	state, err := m.state.Load()
	if err != nil || state == nil {
		return CleanupSnapshot{}
	}
	ts, ok := state.Installed[tool]
	if !ok {
		return CleanupSnapshot{}
	}
	version := activeVersionOf(ts)
	vs, ok := ts.Versions[version]
	if !ok {
		return CleanupSnapshot{}
	}
	return CleanupSnapshot{Tool: tool, Version: version, Actions: vs.CleanupActions}
}

// ReconcileUpdate settles what the replaced version left behind, once an update
// has made a different version active. It does two things:
//
//   - Deletes the paths the old version wrote that the new one does not, and
//     that no installed version still records, rebuilding the shell caches those
//     paths belong to. Right after an update the replaced version is still
//     installed as the rollback target, so its own paths are exempt and the
//     deletion waits for garbage collection to reap it -- see ExecuteStaleCleanup.
//   - Warns when a shell init fragment's content changed across the upgrade.
//     That is the only signal a user gets that a tool rewrote the script their
//     shell sources on every start.
//
// The new active version is read back out of state rather than taken as an
// argument. Every caller would otherwise have to re-derive it, and re-deriving
// it differently is how these paths drifted apart in the first place.
//
// warn may be nil, which drops the warning half.
func (m *Manager) ReconcileUpdate(snap CleanupSnapshot, warn WarnFunc) {
	// A snapshot with nothing in it produces no stale actions and no warnings
	// either way; this only saves the state read below, which `--all` would
	// otherwise pay once per tool that writes nothing outside its own directory.
	if snap.Tool == "" || len(snap.Actions) == 0 {
		return
	}

	state, err := m.state.Load()
	if err != nil || state == nil {
		return
	}
	ts, ok := state.Installed[snap.Tool]
	if !ok {
		return
	}
	newVersion := activeVersionOf(ts)
	if newVersion == "" || newVersion == snap.Version {
		return
	}
	vs, ok := ts.Versions[newVersion]
	if !ok {
		return
	}

	m.ExecuteStaleCleanup(StaleCleanupActions(snap.Actions, vs.CleanupActions))
	warnShellInitChanges(snap.Tool, snap.Actions, vs.CleanupActions, warn)
}

// activeVersionOf reads the active version out of a tool's state, falling back
// to the pre-multi-version field that legacy state files carry.
func activeVersionOf(ts ToolState) string {
	if ts.ActiveVersion != "" {
		return ts.ActiveVersion
	}
	return ts.Version
}

// warnShellInitChanges compares content hashes between the old and new versions'
// cleanup actions for shell.d fragments. When the same (target, shell) has
// different hashes across versions, the tool's shell init output changed during
// the update -- a signal worth surfacing, since an upstream binary that alters
// its init output would otherwise do so unannounced.
//
// The comparison is keyed on (target, shell) rather than on the raw path
// because shell.d filenames carry a version key, so every fragment has a new
// path in every version and a path-keyed comparison would never match.
func warnShellInitChanges(toolName string, old, new []CleanupAction, warn WarnFunc) {
	if warn == nil {
		return
	}

	type fragment struct{ target, shell string }

	oldHashes := make(map[fragment]string)
	for _, ca := range old {
		shell := shellFromCleanupPath(ca.Path)
		if shell == "" || ca.ContentHash == "" {
			continue
		}
		oldHashes[fragment{TargetFromCleanupPath(ca.Path), shell}] = ca.ContentHash
	}

	for _, ca := range new {
		shell := shellFromCleanupPath(ca.Path)
		if shell == "" || ca.ContentHash == "" {
			continue
		}
		oldHash, exists := oldHashes[fragment{TargetFromCleanupPath(ca.Path), shell}]
		if !exists {
			// Not present in the old version -- a newly added shell, not a change.
			continue
		}
		if oldHash != ca.ContentHash {
			warn("shell init changed for %s (%s)", toolName, shell)
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
