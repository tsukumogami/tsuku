package install

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
		if ts.ActiveVersion == "" || ts.Versions == nil {
			return
		}
		vs, ok := ts.Versions[ts.ActiveVersion]
		if !ok {
			return
		}
		vs.CleanupActions = actions
		ts.Versions[ts.ActiveVersion] = vs
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
