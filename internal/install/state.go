package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
)

// VersionState holds per-version metadata for an installed tool version.
type VersionState struct {
	Requested   string    `json:"requested"`          // What user asked for ("17", "@lts", "")
	Binaries    []string  `json:"binaries,omitempty"` // Binary names this version provides
	InstalledAt time.Time `json:"installed_at"`       // When this version was installed
}

// ToolState represents the state of an installed tool
type ToolState struct {
	// ActiveVersion is the currently symlinked version (new multi-version field)
	ActiveVersion string `json:"active_version,omitempty"`
	// Versions contains all installed versions for this tool (new multi-version field)
	Versions map[string]VersionState `json:"versions,omitempty"`

	// Version is deprecated: use ActiveVersion instead. Kept for migration from old state files.
	Version string `json:"version,omitempty"`

	IsExplicit            bool     `json:"is_explicit"`                    // User requested this tool directly
	RequiredBy            []string `json:"required_by"`                    // Tools that depend on this tool
	IsHidden              bool     `json:"is_hidden"`                      // Hidden from PATH and default list output
	IsExecutionDependency bool     `json:"is_execution_dependency"`        // Installed by tsuku for internal use (npm, Python, cargo)
	InstalledVia          string   `json:"installed_via,omitempty"`        // Package manager used to install (npm, pip, cargo, etc.)
	Binaries              []string `json:"binaries,omitempty"`             // List of binary names this tool provides (deprecated: use Versions[v].Binaries)
	InstallDependencies   []string `json:"install_dependencies,omitempty"` // Dependencies needed during installation
	RuntimeDependencies   []string `json:"runtime_dependencies,omitempty"` // Dependencies needed when the tool runs
}

// LibraryVersionState represents the state of a specific library version
type LibraryVersionState struct {
	UsedBy []string `json:"used_by"` // Tools that depend on this library version (e.g., ["ruby-3.4.0", "python-3.12"])
}

// LLMUsage tracks LLM generation history for rate limiting and budget enforcement.
type LLMUsage struct {
	// GenerationTimestamps holds recent generation timestamps for rate limiting.
	// Timestamps older than 1 hour are pruned on access.
	GenerationTimestamps []time.Time `json:"generation_timestamps,omitempty"`

	// DailyCost tracks total LLM cost for the current day in USD.
	DailyCost float64 `json:"daily_cost,omitempty"`

	// DailyCostDate is the date (YYYY-MM-DD in UTC) for DailyCost tracking.
	// When the current date differs, DailyCost is reset to 0.
	DailyCostDate string `json:"daily_cost_date,omitempty"`
}

// State represents the global state of installed tools and libraries
type State struct {
	Installed map[string]ToolState                      `json:"installed"`
	Libs      map[string]map[string]LibraryVersionState `json:"libs,omitempty"`     // map[libName]map[version]LibraryVersionState
	LLMUsage  *LLMUsage                                 `json:"llm_usage,omitempty"` // LLM generation tracking
}

// StateManager handles reading and writing the state file
type StateManager struct {
	config *config.Config
	mu     sync.RWMutex // Protects in-process concurrent access
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

// lockPath returns the path to the lock file
func (sm *StateManager) lockPath() string {
	return filepath.Join(sm.config.HomeDir, "state.json.lock")
}

// Load reads the state from disk
func (sm *StateManager) Load() (*State, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.loadWithLock()
}

// loadWithLock reads the state from disk with file locking.
// Caller must hold sm.mu (read or write lock).
func (sm *StateManager) loadWithLock() (*State, error) {
	path := sm.statePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &State{
			Installed: make(map[string]ToolState),
			Libs:      make(map[string]map[string]LibraryVersionState),
		}, nil
	}

	// Acquire shared file lock for reading
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockShared(); err != nil {
		return nil, fmt.Errorf("failed to acquire read lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

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

	// Migrate old single-version format to new multi-version format
	state.migrateToMultiVersion()

	return &state, nil
}

// Save writes the state to disk
func (sm *StateManager) Save(state *State) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.saveWithLock(state)
}

// saveWithLock writes the state to disk with file locking and atomic write.
// Caller must hold sm.mu write lock.
func (sm *StateManager) saveWithLock(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Acquire exclusive file lock for writing
	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Atomic write: write to temp file, then rename
	path := sm.statePath()
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// loadWithoutLock reads the state from disk without acquiring the file lock.
// Caller must already hold both sm.mu and the file lock.
func (sm *StateManager) loadWithoutLock() (*State, error) {
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

	// Migrate old single-version format to new multi-version format
	state.migrateToMultiVersion()

	return &state, nil
}

// saveWithoutLock writes the state to disk without acquiring the file lock.
// Caller must already hold both sm.mu and the file lock.
func (sm *StateManager) saveWithoutLock(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Atomic write: write to temp file, then rename
	path := sm.statePath()
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

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

// ValidateVersionString validates a version string to prevent path traversal attacks.
// Returns an error if the version contains characters that could be used for path traversal.
func ValidateVersionString(version string) error {
	if strings.Contains(version, "..") {
		return fmt.Errorf("invalid version string: contains '..'")
	}
	if strings.Contains(version, "/") {
		return fmt.Errorf("invalid version string: contains '/'")
	}
	if strings.Contains(version, "\\") {
		return fmt.Errorf("invalid version string: contains '\\'")
	}
	return nil
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
					InstalledAt: time.Now(),    // Best effort timestamp
				},
			}
			// Keep tool.Version and tool.Binaries for backward compat
			// Later issues will update callers to use ActiveVersion, then Version can be cleared

			s.Installed[name] = tool
		}
	}
}

// RecordGeneration records an LLM generation with its cost.
// It adds the current timestamp to the generation history and updates the daily cost.
// Timestamps older than 1 hour are pruned. Daily cost resets at UTC midnight.
func (sm *StateManager) RecordGeneration(cost float64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	lock := NewFileLock(sm.lockPath())
	if err := lock.LockExclusive(); err != nil {
		return fmt.Errorf("failed to acquire lock for LLM usage update: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	state, err := sm.loadWithoutLock()
	if err != nil {
		return err
	}

	if state.LLMUsage == nil {
		state.LLMUsage = &LLMUsage{}
	}

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	// Reset daily cost if date changed
	if state.LLMUsage.DailyCostDate != today {
		state.LLMUsage.DailyCost = 0
		state.LLMUsage.DailyCostDate = today
	}

	// Add current timestamp
	state.LLMUsage.GenerationTimestamps = append(state.LLMUsage.GenerationTimestamps, now)

	// Prune timestamps older than 1 hour
	oneHourAgo := now.Add(-time.Hour)
	pruned := make([]time.Time, 0, len(state.LLMUsage.GenerationTimestamps))
	for _, ts := range state.LLMUsage.GenerationTimestamps {
		if ts.After(oneHourAgo) {
			pruned = append(pruned, ts)
		}
	}
	state.LLMUsage.GenerationTimestamps = pruned

	// Update daily cost
	state.LLMUsage.DailyCost += cost

	return sm.saveWithoutLock(state)
}

// CanGenerate checks if a new LLM generation is allowed based on rate limit and daily budget.
// Returns (allowed, reason) where reason explains why generation is denied.
// A zero or negative limit means unlimited.
func (sm *StateManager) CanGenerate(hourlyLimit int, dailyBudget float64) (bool, string) {
	state, err := sm.Load()
	if err != nil {
		// If we can't load state, allow generation but log warning
		return true, ""
	}

	if state.LLMUsage == nil {
		return true, ""
	}

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	// Check rate limit (if limit > 0)
	if hourlyLimit > 0 {
		oneHourAgo := now.Add(-time.Hour)
		recentCount := 0
		for _, ts := range state.LLMUsage.GenerationTimestamps {
			if ts.After(oneHourAgo) {
				recentCount++
			}
		}
		if recentCount >= hourlyLimit {
			return false, fmt.Sprintf("rate limit exceeded: %d generations in the last hour (limit: %d)", recentCount, hourlyLimit)
		}
	}

	// Check daily budget (if budget > 0)
	if dailyBudget > 0 && state.LLMUsage.DailyCostDate == today {
		if state.LLMUsage.DailyCost >= dailyBudget {
			return false, fmt.Sprintf("daily budget exceeded: $%.2f spent today (budget: $%.2f)", state.LLMUsage.DailyCost, dailyBudget)
		}
	}

	return true, ""
}

// DailySpent returns the total LLM cost spent today in USD.
// Returns 0 if no cost has been recorded today.
func (sm *StateManager) DailySpent() float64 {
	state, err := sm.Load()
	if err != nil {
		return 0
	}

	if state.LLMUsage == nil {
		return 0
	}

	today := time.Now().UTC().Format("2006-01-02")
	if state.LLMUsage.DailyCostDate != today {
		return 0
	}

	return state.LLMUsage.DailyCost
}

// RecentGenerationCount returns the number of LLM generations in the last hour.
func (sm *StateManager) RecentGenerationCount() int {
	state, err := sm.Load()
	if err != nil {
		return 0
	}

	if state.LLMUsage == nil {
		return 0
	}

	now := time.Now().UTC()
	oneHourAgo := now.Add(-time.Hour)
	count := 0
	for _, ts := range state.LLMUsage.GenerationTimestamps {
		if ts.After(oneHourAgo) {
			count++
		}
	}
	return count
}
