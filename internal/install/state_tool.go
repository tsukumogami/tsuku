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
