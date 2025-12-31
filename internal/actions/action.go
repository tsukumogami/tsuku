package actions

import (
	"context"
	"sync"

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// ExecutionContext provides context for action execution
type ExecutionContext struct {
	Context          context.Context   // Context for cancellation, timeouts, and deadlines
	WorkDir          string            // Temporary work directory
	InstallDir       string            // Installation directory (~/.tsuku/tools/.install/)
	ToolInstallDir   string            // Tool-specific directory for directory-based installations (~/.tsuku/tools/{name}-{version}/)
	ToolsDir         string            // Tools directory (~/.tsuku/tools/) for finding other installed tools
	LibsDir          string            // Libraries directory (~/.tsuku/libs/) for finding installed libraries
	DownloadCacheDir string            // Download cache directory (~/.tsuku/cache/downloads/)
	KeyCacheDir      string            // PGP key cache directory (~/.tsuku/cache/keys/)
	Version          string            // Resolved version (e.g., "1.29.3")
	VersionTag       string            // Original version tag (e.g., "v1.29.3" or "1.29.3")
	OS               string            // Target OS (runtime.GOOS)
	Arch             string            // Target architecture (runtime.GOARCH)
	Recipe           *recipe.Recipe    // Full recipe (for reference)
	ExecPaths        []string          // Additional bin paths needed for execution (e.g., nodejs bin for npm tools)
	Resolver         *version.Resolver // Version resolver (for GitHub API access, asset resolution)
	Logger           log.Logger        // Logger for structured logging (optional, falls back to log.Default())
	Dependencies     ResolvedDeps      // Resolved dependencies with their versions
	Env              []string          // Shared environment variables set by setup_build_env, used by build actions
}

// Log returns the logger for this context.
// If no logger is set, it falls back to log.Default().
func (ctx *ExecutionContext) Log() log.Logger {
	if ctx.Logger != nil {
		return ctx.Logger
	}
	return log.Default()
}

// ActionDeps defines what dependencies an action needs.
// InstallTime deps are needed during `tsuku install`.
// Runtime deps are needed when the installed tool runs.
// EvalTime deps are needed during `tsuku eval` for actions that implement Decomposable.
//
// Platform-specific fields (LinuxInstallTime, DarwinInstallTime, etc.) are only
// applied when the target OS matches. This allows actions to declare dependencies
// that are only needed on certain platforms, reducing unnecessary installations.
type ActionDeps struct {
	InstallTime []string // Needed during tsuku install (all platforms)
	Runtime     []string // Needed when tool runs (all platforms)
	EvalTime    []string // Needed during tsuku eval (for Decompose)

	// Platform-specific install-time dependencies.
	// Only applied when runtime.GOOS matches the platform.
	LinuxInstallTime  []string // Linux-only install deps
	DarwinInstallTime []string // macOS-only install deps

	// Platform-specific runtime dependencies.
	// Only applied when runtime.GOOS matches the platform.
	LinuxRuntime  []string // Linux-only runtime deps
	DarwinRuntime []string // macOS-only runtime deps
}

// Action represents an executable action with metadata.
type Action interface {
	// Name returns the action name (e.g., "download", "extract")
	Name() string

	// Execute performs the action
	Execute(ctx *ExecutionContext, params map[string]interface{}) error

	// IsDeterministic returns true if the action produces identical results
	// given identical inputs. Core primitives are deterministic. Ecosystem
	// primitives have residual non-determinism and return false.
	IsDeterministic() bool

	// Dependencies returns the install-time and runtime dependencies for this action.
	Dependencies() ActionDeps
}

// NetworkValidator is implemented by actions that can declare network requirements.
// Actions that fetch external dependencies (cargo_build, go_build, npm_install, etc.)
// return true. Actions that work with cached or pre-downloaded content return false.
//
// This interface enables sandbox testing to configure container network access
// appropriately - offline for binary installations, network-enabled for ecosystem builds.
type NetworkValidator interface {
	RequiresNetwork() bool
}

// BaseAction provides default implementations for Action metadata methods.
// Embed this in action types to inherit defaults:
//   - IsDeterministic() returns false (safe default)
//   - Dependencies() returns empty ActionDeps
//   - RequiresNetwork() returns false (most actions work offline)
//
// Actions override these methods when they have non-default values.
type BaseAction struct{}

// IsDeterministic returns false by default.
// Actions that produce identical results given identical inputs should override this.
func (BaseAction) IsDeterministic() bool { return false }

// Dependencies returns empty ActionDeps by default.
// Actions with install-time or runtime dependencies should override this.
func (BaseAction) Dependencies() ActionDeps { return ActionDeps{} }

// RequiresNetwork returns false by default.
// Actions that fetch external dependencies should override this to return true.
func (BaseAction) RequiresNetwork() bool { return false }

// Registry holds all available actions
var (
	registry   = make(map[string]Action)
	registryMu sync.RWMutex
)

// Register adds an action to the registry
func Register(action Action) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[action.Name()] = action
}

// Get retrieves an action by name
func Get(name string) Action {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// init registers all core actions
func init() {
	// Core actions
	Register(&DownloadAction{})
	Register(&DownloadFileAction{})
	Register(&ExtractAction{})
	Register(&ChmodAction{})
	Register(&InstallBinariesAction{})
	Register(&SetEnvAction{})
	Register(&RunCommandAction{})
	Register(&SetRpathAction{})
	Register(&InstallLibrariesAction{})
	Register(&LinkDependenciesAction{})
	Register(&RequireSystemAction{})

	// System package actions (implement SystemAction interface)
	Register(&AptInstallAction{})
	Register(&AptRepoAction{})
	Register(&AptPPAAction{})
	Register(&BrewInstallAction{})
	Register(&BrewCaskAction{})
	Register(&DnfInstallAction{})
	Register(&DnfRepoAction{})
	Register(&PacmanInstallAction{})
	Register(&ApkInstallAction{})
	Register(&ZypperInstallAction{})

	// Package manager actions (composite)
	Register(&NpmInstallAction{})
	Register(&NpmExecAction{})
	Register(&PipxInstallAction{})
	Register(&PipInstallAction{})
	Register(&CargoInstallAction{})
	Register(&CargoBuildAction{})
	Register(&GemInstallAction{})
	Register(&CpanInstallAction{})
	Register(&GoInstallAction{})
	Register(&NixInstallAction{})

	// Ecosystem primitives
	Register(&GoBuildAction{})
	Register(&NixRealizeAction{})
	Register(&ConfigureMakeAction{})
	Register(&CMakeBuildAction{})
	Register(&MesonBuildAction{})
	Register(&PipExecAction{})
	Register(&SetupBuildEnvAction{})

	// Homebrew actions
	Register(&HomebrewAction{})
	Register(&HomebrewRelocateAction{})

	// Composite actions
	Register(&DownloadArchiveAction{})
	Register(&GitHubArchiveAction{})
	Register(&GitHubFileAction{})
	Register(&FossilArchiveAction{})

	// System configuration actions
	Register(&GroupAddAction{})
	Register(&ServiceEnableAction{})
	Register(&ServiceStartAction{})
	Register(&RequireCommandAction{})
	Register(&ManualAction{})
}
