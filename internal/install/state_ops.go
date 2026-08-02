package install

import "time"

// state_ops.go contains semantic state-mutation methods on Manager that
// replace the `mgr.GetState().UpdateTool(name, func(ts *ToolState){...})`
// lambda pattern used in CLI dependency-walk code. Each method is a thin
// wrapper around StateManager.UpdateTool with one focused operation.
//
// These methods do NOT publish lifecycle events. State-fragment writes
// (marking a tool as explicit, recording a dep, recording cleanup actions)
// are bookkeeping updates, not lifecycle transitions. Install / Remove /
// Activate own the event-publishing responsibility.
//
// If a future Service layer (Candidate B in DESIGN-install-state-abstraction)
// lands, these methods migrate cleanly: rename receiver, update call sites.
// The bodies are intentionally small so relocating them is mechanical.

// MarkExplicit marks the tool as explicitly requested by the user.
// Always sets ts.IsExplicit = true. If parent is non-empty and not
// already present in ts.RequiredBy, appends parent (dedupe). Safe to
// call on a fresh (uninstalled) tool: StateManager.UpdateTool creates
// the ToolState entry if absent.
func (m *Manager) MarkExplicit(name, parent string) error {
	return m.state.UpdateTool(name, func(ts *ToolState) {
		ts.IsExplicit = true
		if parent == "" {
			return
		}
		for _, r := range ts.RequiredBy {
			if r == parent {
				return
			}
		}
		ts.RequiredBy = append(ts.RequiredBy, parent)
	})
}

// RecordDependency appends dep to ts.InstallDependencies. Existing
// entries equal to dep are deduped (no duplicate appended). Empty dep
// is a no-op (no state write performed beyond the lock cycle).
func (m *Manager) RecordDependency(name, dep string) error {
	if dep == "" {
		return nil
	}
	return m.state.UpdateTool(name, func(ts *ToolState) {
		for _, d := range ts.InstallDependencies {
			if d == dep {
				return
			}
		}
		ts.InstallDependencies = append(ts.InstallDependencies, dep)
	})
}

// SetInstallDependencies overwrites ts.InstallDependencies with deps.
// Use when the full install-dep set is known at once (e.g., after a
// recipe-driven install resolves all deps). For incremental append
// semantics during a dep walk, use RecordDependency.
func (m *Manager) SetInstallDependencies(name string, deps []string) error {
	return m.state.UpdateTool(name, func(ts *ToolState) {
		ts.InstallDependencies = deps
	})
}

// SetRuntimeDependencies overwrites ts.RuntimeDependencies with deps.
func (m *Manager) SetRuntimeDependencies(name string, deps []string) error {
	return m.state.UpdateTool(name, func(ts *ToolState) {
		ts.RuntimeDependencies = deps
	})
}

// RecordCleanup stores actions on the tool's active version state.
// CleanupActions live on VersionState (not ToolState directly), so this
// method resolves the active version internally via ts.ActiveVersion.
//
// No-op when actions is empty. No-op when ts.ActiveVersion is unset or
// ts.Versions does not contain an entry for the active version — both
// indicate the tool has not yet been installed in this lifecycle and
// there is no version-state to attach cleanup actions to.
func (m *Manager) RecordCleanup(name string, actions []CleanupAction) error {
	if len(actions) == 0 {
		return nil
	}
	return m.state.UpdateTool(name, func(ts *ToolState) {
		if ts.ActiveVersion == "" {
			return
		}
		setVersionCleanup(ts, ts.ActiveVersion, actions)
	})
}

// RecordCleanupForVersion stores actions on a named version's state rather
// than on whichever version is active.
//
// The distinction matters for a dependency. A plan can pin a dependency version
// that is not the one the user has active, and installing it must not move the
// active version's cleanup record out from under it. Callers that just installed
// the version they are recording against can use RecordCleanup instead.
//
// No-op when actions is empty, when version is empty, or when the tool has no
// state entry for that version.
func (m *Manager) RecordCleanupForVersion(name, version string, actions []CleanupAction) error {
	if len(actions) == 0 || version == "" {
		return nil
	}
	return m.state.UpdateTool(name, func(ts *ToolState) {
		setVersionCleanup(ts, version, actions)
	})
}

// setVersionCleanup writes actions onto one version's state, leaving the tool
// untouched when that version is not recorded.
func setVersionCleanup(ts *ToolState, version string, actions []CleanupAction) {
	if ts.Versions == nil {
		return
	}
	vs, ok := ts.Versions[version]
	if !ok {
		return
	}
	vs.CleanupActions = actions
	ts.Versions[version] = vs
}

// RecordDependencyInstall gives a dependency the executor installed itself a
// state entry, so the files it wrote are attributable to it rather than
// invisible.
//
// Executor.installSingleDependency copies a dependency straight into
// $TSUKU_HOME/tools/<name>-<version> without going through Install: no
// symlinks, no wrappers, no binaries registered. This records exactly the part
// that has to exist for removal and the shell.d projection to work -- the
// version entry and its install time -- and nothing that would imply the
// dependency is on PATH.
//
// A dependency is hidden and marked an execution dependency, matching what
// HiddenInstallOptions produces on the normal path. Both flags are set only
// when the tool has no state yet: a tool the user installed explicitly and
// which some later recipe happens to depend on must not become hidden.
//
// The active version is likewise only claimed when there is none. Installing a
// dependency is not a request to switch the user's active version, and the
// shell.d projection reads the active version to decide which fragment belongs
// in the cache.
//
// binaries names what the dependency provides. Recording it is not optional
// bookkeeping: hidden is a state a tool leaves through ExposeHidden, which
// links exactly the binaries the entry records, so an entry that records none
// is one `tsuku install <dep>` away from a dangling symlink. When the
// dependency's steps do not name any, CheckAndExposeHidden declines to expose
// it and the user gets a real install instead.
func (m *Manager) RecordDependencyInstall(name, version, parent string, binaries []string) error {
	if version == "" {
		return nil
	}
	return m.state.UpdateTool(name, func(ts *ToolState) {
		fresh := ts.ActiveVersion == "" && len(ts.Versions) == 0

		if ts.Versions == nil {
			ts.Versions = make(map[string]VersionState)
		}
		if _, exists := ts.Versions[version]; !exists {
			ts.Versions[version] = VersionState{InstalledAt: time.Now(), Binaries: binaries}
		}

		if fresh {
			ts.ActiveVersion = version
			ts.Version = version
			ts.Binaries = binaries
			ts.IsHidden = true
			ts.IsExecutionDependency = true
		}

		if parent == "" {
			return
		}
		for _, r := range ts.RequiredBy {
			if r == parent {
				return
			}
		}
		ts.RequiredBy = append(ts.RequiredBy, parent)
	})
}

// GetToolState returns the state for a single tool, or nil if the tool
// is not present in state.json. Thin pass-through to StateManager so
// callers can avoid the broader GetState() escape hatch.
func (m *Manager) GetToolState(name string) (*ToolState, error) {
	return m.state.GetToolState(name)
}

// LoadState returns the full state snapshot. Read-only accessor; callers
// must not mutate the returned value (use Manager mutation methods or
// StateManager.UpdateTool for writes).
func (m *Manager) LoadState() (*State, error) {
	return m.state.Load()
}
