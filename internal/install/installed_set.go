package install

// InstalledVersionsFor returns the versions installation state records for each
// named tool, from a single decode, without taking the state lock.
//
// It exists for the activation path, which runs from a shell prompt hook and so
// has two constraints the ordinary accessors do not meet. GetToolState decodes
// the whole state file to answer about one tool, so a project declaring N tools
// would pay N decodes; this answers about all of them at once. And Load takes a
// shared file lock that has no non-blocking variant here, so a prompt firing
// while an install held the exclusive lock would wait on it. Reading without
// the lock is safe because Save publishes by atomic rename -- see
// loadWithoutLock.
//
// Versions come back in whatever order the decode produced. Callers that need
// the newest must sort with version comparison, not string comparison: string
// order puts "10.0.0" before "9.0.0" and "1.0.0" before "2.0.0", so a caller
// taking the first or last element gets the wrong answer in one direction or
// the other. This deliberately does not sort, so that no caller can inherit a
// wrong ordering from it by accident.
//
// A tool with no state entry yields no map entry and no error. A missing state
// file yields an empty map and no error: the loader cannot distinguish a
// machine whose state was deleted from one that has installed nothing, and
// neither can this.
//
// Hidden tools are included. A tool installed as another tool's dependency is
// still installed, and a project that declares it should activate it.
func (sm *StateManager) InstalledVersionsFor(names []string) (map[string][]string, error) {
	if len(names) == 0 {
		return map[string][]string{}, nil
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return nil, err
	}

	out := make(map[string][]string, len(names))
	for _, name := range names {
		toolState, ok := state.Installed[name]
		if !ok {
			continue
		}
		versions := make([]string, 0, len(toolState.Versions))
		for version := range toolState.Versions {
			versions = append(versions, version)
		}
		if len(versions) > 0 {
			out[name] = versions
		}
	}
	return out, nil
}
