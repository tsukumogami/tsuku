package actions

import (
	"context"

	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/version"
)

// ExecutionContext provides context for action execution
type ExecutionContext struct {
	Context        context.Context   // Context for cancellation, timeouts, and deadlines
	WorkDir        string            // Temporary work directory
	InstallDir     string            // Installation directory (~/.tsuku/tools/.install/)
	ToolInstallDir string            // Tool-specific directory for directory-based installations (~/.tsuku/tools/{name}-{version}/)
	ToolsDir       string            // Tools directory (~/.tsuku/tools/) for finding other installed tools
	Version        string            // Resolved version (e.g., "1.29.3")
	VersionTag     string            // Original version tag (e.g., "v1.29.3" or "1.29.3")
	OS             string            // Target OS (runtime.GOOS)
	Arch           string            // Target architecture (runtime.GOARCH)
	Recipe         *recipe.Recipe    // Full recipe (for reference)
	ExecPaths      []string          // Additional bin paths needed for execution (e.g., nodejs bin for npm tools)
	Resolver       *version.Resolver // Version resolver (for GitHub API access, asset resolution)
}

// Action represents an executable action
type Action interface {
	// Name returns the action name (e.g., "download", "extract")
	Name() string

	// Execute performs the action
	Execute(ctx *ExecutionContext, params map[string]interface{}) error
}

// Registry holds all available actions
var registry = make(map[string]Action)

// Register adds an action to the registry
func Register(action Action) {
	registry[action.Name()] = action
}

// Get retrieves an action by name
func Get(name string) Action {
	return registry[name]
}

// init registers all core actions
func init() {
	// Core actions
	Register(&DownloadAction{})
	Register(&ExtractAction{})
	Register(&ChmodAction{})
	Register(&InstallBinariesAction{})
	Register(&SetEnvAction{})
	Register(&RunCommandAction{})
	Register(&AptInstallAction{})
	Register(&YumInstallAction{})
	Register(&BrewInstallAction{})

	// Package manager actions
	Register(&NpmInstallAction{})
	Register(&PipxInstallAction{})
	Register(&CargoInstallAction{})
	Register(&GemInstallAction{})
	Register(&CpanInstallAction{})
	Register(&GoInstallAction{})
	Register(&NixInstallAction{})

	// Composite actions
	Register(&DownloadArchiveAction{})
	Register(&GitHubArchiveAction{})
	Register(&GitHubFileAction{})
	Register(&HashiCorpReleaseAction{})
	Register(&HomebrewBottleAction{})
}
