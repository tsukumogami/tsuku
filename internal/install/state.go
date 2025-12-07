package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tsukumogami/tsuku/internal/config"
)

// ToolState represents the state of an installed tool
type ToolState struct {
	Version               string   `json:"version"`
	IsExplicit            bool     `json:"is_explicit"`             // User requested this tool directly
	RequiredBy            []string `json:"required_by"`             // Tools that depend on this tool
	IsHidden              bool     `json:"is_hidden"`               // Hidden from PATH and default list output
	IsExecutionDependency bool     `json:"is_execution_dependency"` // Installed by tsuku for internal use (npm, Python, cargo)
	InstalledVia          string   `json:"installed_via,omitempty"` // Package manager used to install (npm, pip, cargo, etc.)
	Binaries              []string `json:"binaries,omitempty"`      // List of binary names this tool provides
}

// LibraryVersionState represents the state of a specific library version
type LibraryVersionState struct {
	UsedBy []string `json:"used_by"` // Tools that depend on this library version (e.g., ["ruby-3.4.0", "python-3.12"])
}

// State represents the global state of installed tools and libraries
type State struct {
	Installed map[string]ToolState                      `json:"installed"`
	Libs      map[string]map[string]LibraryVersionState `json:"libs,omitempty"` // map[libName]map[version]LibraryVersionState
}

// StateManager handles reading and writing the state file
type StateManager struct {
	config *config.Config
	mu     sync.RWMutex
}

// NewStateManager creates a new state manager
func NewStateManager(cfg *config.Config) *StateManager {
	return &StateManager{
		config: cfg,
	}
}

// statePath returns the path to the state file
func (sm *StateManager) statePath() string {
	return filepath.Join(sm.config.HomeDir, "state.json")
}

// Load reads the state from disk
func (sm *StateManager) Load() (*State, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	path := sm.statePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &State{
			Installed: make(map[string]ToolState),
			Libs:      make(map[string]map[string]LibraryVersionState),
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Initialize maps if nil (backward compatibility)
	if state.Installed == nil {
		state.Installed = make(map[string]ToolState)
	}
	if state.Libs == nil {
		state.Libs = make(map[string]map[string]LibraryVersionState)
	}

	return &state, nil
}

// Save writes the state to disk
func (sm *StateManager) Save(state *State) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(sm.statePath(), data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// UpdateTool updates the state for a single tool
func (sm *StateManager) UpdateTool(name string, update func(*ToolState)) error {
	state, err := sm.Load()
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

	return sm.Save(state)
}

// RemoveTool removes a tool from the state
func (sm *StateManager) RemoveTool(name string) error {
	state, err := sm.Load()
	if err != nil {
		return err
	}

	delete(state.Installed, name)

	return sm.Save(state)
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

// UpdateLibrary updates the state for a specific library version
func (sm *StateManager) UpdateLibrary(name, version string, update func(*LibraryVersionState)) error {
	state, err := sm.Load()
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

	return sm.Save(state)
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

// RemoveLibraryVersion removes a specific library version from state
func (sm *StateManager) RemoveLibraryVersion(libName, libVersion string) error {
	state, err := sm.Load()
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

	return sm.Save(state)
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
