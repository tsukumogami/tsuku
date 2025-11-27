package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tsuku-dev/tsuku/internal/config"
)

// ToolState represents the state of an installed tool
type ToolState struct {
	Version              string   `json:"version"`
	IsExplicit           bool     `json:"is_explicit"`            // User requested this tool directly
	RequiredBy           []string `json:"required_by"`            // Tools that depend on this tool
	IsHidden             bool     `json:"is_hidden"`              // Hidden from PATH and default list output
	IsExecutionDependency bool     `json:"is_execution_dependency"` // Installed by tsuku for internal use (npm, Python, cargo)
	InstalledVia         string   `json:"installed_via,omitempty"` // Package manager used to install (npm, pip, cargo, etc.)
	Binaries             []string `json:"binaries,omitempty"`     // List of binary names this tool provides
}

// State represents the global state of installed tools
type State struct {
	Installed map[string]ToolState `json:"installed"`
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
		return &State{Installed: make(map[string]ToolState)}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	if state.Installed == nil {
		state.Installed = make(map[string]ToolState)
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
