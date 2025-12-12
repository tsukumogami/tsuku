package install

import "fmt"

// UpdateLibrary updates the state for a specific library version atomically.
// The exclusive lock is held for the entire read-modify-write cycle.
func (sm *StateManager) UpdateLibrary(name, version string, update func(*LibraryVersionState)) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Acquire exclusive file lock for the entire operation
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for library update: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	// Initialize nested map if needed
	if state.Libs[name] == nil {
		state.Libs[name] = make(map[string]LibraryVersionState)
	}

	libState := state.Libs[name][version]
	if libState.UsedBy == nil {
		libState.UsedBy = []string{}
	}

	update(&libState)
	state.Libs[name][version] = libState

	return sm.saveWithoutLock(state)
}

// AddLibraryUsedBy adds a dependent tool to the UsedBy list for a library version
func (sm *StateManager) AddLibraryUsedBy(libName, libVersion, toolNameVersion string) error {
	return sm.UpdateLibrary(libName, libVersion, func(ls *LibraryVersionState) {
		for _, u := range ls.UsedBy {
			if u == toolNameVersion {
				return // Already in list
			}
		}
		ls.UsedBy = append(ls.UsedBy, toolNameVersion)
	})
}

// RemoveLibraryUsedBy removes a dependent tool from the UsedBy list for a library version
func (sm *StateManager) RemoveLibraryUsedBy(libName, libVersion, toolNameVersion string) error {
	return sm.UpdateLibrary(libName, libVersion, func(ls *LibraryVersionState) {
		newUsedBy := []string{}
		for _, u := range ls.UsedBy {
			if u != toolNameVersion {
				newUsedBy = append(newUsedBy, u)
			}
		}
		ls.UsedBy = newUsedBy
	})
}

// RemoveLibraryVersion removes a specific library version from state atomically.
// The exclusive lock is held for the entire read-modify-write cycle.
func (sm *StateManager) RemoveLibraryVersion(libName, libVersion string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Acquire exclusive file lock for the entire operation
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for library removal: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	if state.Libs[libName] != nil {
		delete(state.Libs[libName], libVersion)
		// Clean up empty library entry
		if len(state.Libs[libName]) == 0 {
			delete(state.Libs, libName)
		}
	}

	return sm.saveWithoutLock(state)
}

// GetLibraryState returns the state for a specific library version, or nil if not found
func (sm *StateManager) GetLibraryState(libName, libVersion string) (*LibraryVersionState, error) {
	state, err := sm.Load()
	if err != nil {
		return nil, err
	}

	if state.Libs[libName] == nil {
		return nil, nil
	}

	libState, exists := state.Libs[libName][libVersion]
	if !exists {
		return nil, nil
	}

	return &libState, nil
}
