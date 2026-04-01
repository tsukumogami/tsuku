package install

import "fmt"

// UpdateTool updates the state for a single tool atomically.
// The exclusive lock is held for the entire read-modify-write cycle.
func (sm *StateManager) UpdateTool(name string, update func(*ToolState)) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Acquire exclusive file lock for the entire operation
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for update: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Load state (without acquiring lock again)
	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	toolState, exists := state.Installed[name]
	if !exists {
		toolState = ToolState{
			RequiredBy: []string{},
		}
	}

	update(&toolState)
	state.Installed[name] = toolState

	return sm.saveWithoutLock(state)
}

// RemoveTool removes a tool from the state atomically.
// The exclusive lock is held for the entire read-modify-write cycle.
func (sm *StateManager) RemoveTool(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Acquire exclusive file lock for the entire operation
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for removal: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	delete(state.Installed, name)

	return sm.saveWithoutLock(state)
}

// AddRequiredBy adds a dependent tool to the RequiredBy list
func (sm *StateManager) AddRequiredBy(dependency, dependent string) error {
	return sm.UpdateTool(dependency, func(ts *ToolState) {
		for _, r := range ts.RequiredBy {
			if r == dependent {
				return
			}
		}
		ts.RequiredBy = append(ts.RequiredBy, dependent)
	})
}

// RemoveRequiredBy removes a dependent tool from the RequiredBy list
func (sm *StateManager) RemoveRequiredBy(dependency, dependent string) error {
	return sm.UpdateTool(dependency, func(ts *ToolState) {
		newRequiredBy := []string{}
		for _, r := range ts.RequiredBy {
			if r != dependent {
				newRequiredBy = append(newRequiredBy, r)
			}
		}
		ts.RequiredBy = newRequiredBy
	})
}

// UpdateToolWithoutLock updates the state for a single tool without acquiring
// the file lock. The caller must already hold the exclusive file lock and sm.mu
// write lock. Used by auto-apply which holds a TryLockExclusive for the entire
// apply cycle.
func (sm *StateManager) UpdateToolWithoutLock(name string, update func(*ToolState)) error {
	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	toolState, exists := state.Installed[name]
	if !exists {
		toolState = ToolState{
			RequiredBy: []string{},
		}
	}

	update(&toolState)
	state.Installed[name] = toolState

	return sm.saveWithoutLock(state)
}

// GetToolStateWithoutLock reads the state for a specific tool without acquiring
// the file lock. The caller must already hold the file lock.
func (sm *StateManager) GetToolStateWithoutLock(name string) (*ToolState, error) {
	state, err := sm.loadWithoutLock()
	if err != nil {
		return nil, err
	}

	toolState, exists := state.Installed[name]
	if !exists {
		return nil, nil
	}

	return &toolState, nil
}

// GetToolState returns the state for a specific tool, or nil if not found
func (sm *StateManager) GetToolState(name string) (*ToolState, error) {
	state, err := sm.Load()
	if err != nil {
		return nil, err
	}

	toolState, exists := state.Installed[name]
	if !exists {
		return nil, nil
	}

	return &toolState, nil
}

// GetCachedPlan returns the cached installation plan for a specific tool and version.
// Returns nil if the tool is not installed, the version is not installed, or no plan
// was cached for that version.
func (sm *StateManager) GetCachedPlan(tool, version string) (*Plan, error) {
	state, err := sm.Load()
	if err != nil {
		return nil, err
	}

	toolState, exists := state.Installed[tool]
	if !exists {
		return nil, nil
	}

	versionState, exists := toolState.Versions[version]
	if !exists {
		return nil, nil
	}

	return versionState.Plan, nil
}

// migrateSourceTracking populates empty Source fields on ToolState entries.
// For entries with a cached plan, the source is inferred from Plan.RecipeSource:
//   - "registry" -> "central" (the central registry)
//   - "embedded" -> "central" (embedded recipes are a bundled subset of central)
//   - "local" -> "local"
//
// Entries without a plan or with an unrecognized RecipeSource default to "central",
// since all tools installed before source tracking was introduced came from the
// central registry or embedded recipes (both are "central" for update purposes).
//
// This migration is idempotent: entries that already have a Source value are skipped.
func (s *State) migrateSourceTracking() {
	for name, tool := range s.Installed {
		if tool.Source != "" {
			continue
		}

		source := "central"

		// Try to infer from the active version's cached plan
		if tool.ActiveVersion != "" {
			if vs, ok := tool.Versions[tool.ActiveVersion]; ok && vs.Plan != nil {
				switch vs.Plan.RecipeSource {
				case "local":
					source = "local"
					// "registry", "embedded", and anything else -> "central"
				}
			}
		}

		tool.Source = source
		s.Installed[name] = tool
	}
}

// migrateToMultiVersion migrates old single-version state entries to the new multi-version format.
// Old format: ToolState.Version = "1.0.0", ToolState.Binaries = ["foo"]
// New format: ToolState.ActiveVersion = "1.0.0", ToolState.Versions = {"1.0.0": {Binaries: ["foo"], ...}}
func (s *State) migrateToMultiVersion() {
	for name, tool := range s.Installed {
		// Detect old format: has Version but no ActiveVersion
		if tool.Version != "" && tool.ActiveVersion == "" {
			// Migrate to new format
			tool.ActiveVersion = tool.Version
			tool.Versions = map[string]VersionState{
				tool.Version: {
					Requested:   "",            // Unknown for migrated entries
					Binaries:    tool.Binaries, // Copy binaries to version state
					InstalledAt: timeNow(),     // Best effort timestamp
				},
			}
			// Keep tool.Version and tool.Binaries for backward compat
			// Later issues will update callers to use ActiveVersion, then Version can be cleared

			s.Installed[name] = tool
		}
	}
}
