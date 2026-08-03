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

// CleanupAction describes a file-system cleanup operation to perform when
// a tool version is removed. Paths are relative to $TSUKU_HOME.
type CleanupAction struct {
	Action      string `json:"action"`                 // "delete_file", "delete_dir"
	Path        string `json:"path"`                   // relative to $TSUKU_HOME
	ContentHash string `json:"content_hash,omitempty"` // SHA-256 hex digest of file content at install time
}

// VersionState holds per-version metadata for an installed tool version.
type VersionState struct {
	Requested          string            `json:"requested"`                     // What user asked for ("17", "@lts", "")
	Binaries           []string          `json:"binaries,omitempty"`            // Binary names this version provides
	BinaryChecksums    map[string]string `json:"binary_checksums,omitempty"`    // SHA256 checksums of installed binaries (path -> hex hash)
	InstalledAt        time.Time         `json:"installed_at"`                  // When this version was installed
	Plan               *Plan             `json:"plan,omitempty"`                // Installation plan (if generated)
	AppPath            string            `json:"app_path,omitempty"`            // Path to installed .app bundle (macOS only)
	ApplicationSymlink string            `json:"application_symlink,omitempty"` // Path to ~/Applications symlink (if created)
	CleanupActions     []CleanupAction   `json:"cleanup_actions,omitempty"`     // Files to delete on remove (recorded by post-install actions)
}

// PlanStorageVersion versions the conversion of an executor plan into a
// state.json record. It is not the plan format version -- Plan.FormatVersion
// is that, and it tracks the shape of the plan itself.
//
// Version 1 is the first conversion that carries the whole plan: its
// dependency tree, verify block, recipe type, binaries, per-step phases, and
// Linux family. Earlier conversions dropped all of those, so a record written
// before version 1 existed decodes as 0 and cannot be trusted to be complete.
// Readers treat a record below the current version as untrusted rather than
// as a plan with nothing to declare -- an absent dependency tree and a tool
// with no dependencies look identical on disk.
const PlanStorageVersion = 1

// Plan represents a stored installation plan: the executor's InstallationPlan
// in the shape state.json holds it. Its fields mirror the executor types
// rather than referencing them, because internal/executor imports this
// package and the dependency cannot run the other way.
type Plan struct {
	// StorageVersion records which conversion wrote this record.
	// See PlanStorageVersion. Absent (0) means the record predates the
	// conversion carrying every field.
	StorageVersion int `json:"storage_version,omitempty"`

	FormatVersion int          `json:"format_version"`
	Tool          string       `json:"tool"`
	Version       string       `json:"version"`
	Platform      PlanPlatform `json:"platform"`
	GeneratedAt   time.Time    `json:"generated_at"`
	// RecipeSource is the raw provider tag ("registry", "embedded", "local").
	// See recipeSourceFromProvider in cmd/tsuku/helpers.go for the mapping
	// to the normalized ToolState.Source values ("central", "local", "owner/repo").
	RecipeSource  string     `json:"recipe_source"`
	Deterministic bool       `json:"deterministic"`
	Steps         []PlanStep `json:"steps"`

	// Dependencies holds the nested install-time dependency plans. On the
	// plan-based install path there is no recipe to walk, so this is the only
	// record of what the tool needs installed alongside it.
	Dependencies []PlanDependency `json:"dependencies,omitempty"`

	// Verify is the recipe's verification block. A nil Verify means
	// verification is skipped rather than failed, so losing it is silent.
	Verify *PlanVerify `json:"verify,omitempty"`

	// RecipeType is "tool" or "library". Empty means tool.
	RecipeType string `json:"recipe_type,omitempty"`

	// Binaries mirrors the recipe's declared binary names. It is the fallback
	// for recipes whose actions leave no install_binaries step to infer from.
	Binaries []string `json:"binaries,omitempty"`
}

// PlanPlatform identifies the target platform for a plan.
type PlanPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	// LinuxFamily is set only when the plan targets a specific Linux family
	// (e.g., "debian", "fedora"), which happens when the recipe uses
	// family-specific steps.
	LinuxFamily string `json:"linux_family,omitempty"`
}

// PlanStep represents a resolved installation step.
type PlanStep struct {
	Action string `json:"action"`
	// Phase is the lifecycle phase the recipe named for this step. Empty
	// means the recipe named none and the action decides, which is what
	// executor.StepPhase implements.
	Phase         string                 `json:"phase,omitempty"`
	Params        map[string]interface{} `json:"params"`
	Evaluable     bool                   `json:"evaluable"`
	Deterministic bool                   `json:"deterministic"`
	URL           string                 `json:"url,omitempty"`
	Checksum      string                 `json:"checksum,omitempty"`
	Size          int64                  `json:"size,omitempty"`
}

// PlanDependency is a nested installation plan for one dependency. Mirrors
// executor.DependencyPlan, including the recursion.
type PlanDependency struct {
	Tool         string           `json:"tool"`
	Version      string           `json:"version"`
	Dependencies []PlanDependency `json:"dependencies,omitempty"`
	Steps        []PlanStep       `json:"steps"`
	Verify       *PlanVerify      `json:"verify,omitempty"`
	RecipeType   string           `json:"recipe_type,omitempty"`
}

// PlanVerify captures verification information from the recipe. Mirrors
// executor.PlanVerify: Pattern is the legacy single-substring form, Patterns
// is the multi-substring AND-list, and a recipe sets one or the other.
type PlanVerify struct {
	Command  string   `json:"command,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	ExitCode *int     `json:"exit_code,omitempty"`
	// Additional carries the recipe's secondary verification commands.
	Additional []PlanAdditionalVerify `json:"additional,omitempty"`
}

// PlanAdditionalVerify is one secondary verification command carried in a
// stored plan. Mirrors executor.PlanAdditionalVerify.
type PlanAdditionalVerify struct {
	Command string `json:"command"`
	Pattern string `json:"pattern"`
}

// NewPlanFromExecutor creates a Plan from executor plan types.
// This is a conversion helper that preserves all plan data for storage.
func NewPlanFromExecutor(formatVersion int, tool, version string, platform PlanPlatform,
	generatedAt time.Time, recipeSource string, deterministic bool, steps []PlanStep) *Plan {
	return &Plan{
		FormatVersion: formatVersion,
		Tool:          tool,
		Version:       version,
		Platform:      platform,
		GeneratedAt:   generatedAt,
		RecipeSource:  recipeSource,
		Deterministic: deterministic,
		Steps:         steps,
	}
}

// ToolState represents the state of an installed tool
type ToolState struct {
	// ActiveVersion is the currently symlinked version (new multi-version field)
	ActiveVersion string `json:"active_version,omitempty"`
	// Versions contains all installed versions for this tool (new multi-version field)
	Versions map[string]VersionState `json:"versions,omitempty"`

	// Version is deprecated: use ActiveVersion instead. Kept for migration from old state files.
	Version string `json:"version,omitempty"`

	// Source records where this tool's recipe came from.
	// Values: "central" (default registry and embedded), "local", or "owner/repo" (distributed).
	// Populated on new installs; lazily migrated for existing entries.
	Source string `json:"source,omitempty"`

	// RecipeHash is the SHA256 hash of the recipe TOML bytes at install time.
	// Provides an audit trail for what recipe content was used to install the tool.
	RecipeHash string `json:"recipe_hash,omitempty"`

	// PreviousVersion is the version that was active before the current one.
	// Set when ActiveVersion changes during install or activate. Used by
	// tsuku rollback to revert to the prior version. One level deep per R9.
	// Cleared on explicit install with a pinned version.
	PreviousVersion string `json:"previous_version,omitempty"`

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
	UsedBy    []string          `json:"used_by"`             // Tools that depend on this library version (e.g., ["ruby-3.4.0", "python-3.12"])
	Checksums map[string]string `json:"checksums,omitempty"` // SHA256 checksums of library files (relative path -> hex hash)
	Sonames   []string          `json:"sonames,omitempty"`   // Auto-discovered sonames (e.g., ["libssl.so.3", "libyaml-0.so.2"])
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
	Libs      map[string]map[string]LibraryVersionState `json:"libs,omitempty"`      // map[libName]map[version]LibraryVersionState
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

	// Migrate empty Source fields to default values
	state.migrateSourceTracking()

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
// LoadWithoutLock reads state without acquiring the file lock.
// The caller must already hold the file lock.
func (sm *StateManager) LoadWithoutLock() (*State, error) {
	return sm.loadWithoutLock()
}

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

	// Migrate empty Source fields to default values
	state.migrateSourceTracking()

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
