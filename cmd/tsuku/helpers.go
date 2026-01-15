package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/errmsg"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// loader holds the recipe loader (shared across all commands)
var loader *recipe.Loader

// constraintLookup holds the constraint lookup function (shared across all commands).
// This is set during initialization and used by loadLocalRecipe for step analysis.
var constraintLookup recipe.ConstraintLookup

// loadLocalRecipe loads a recipe from a local file path.
// This is a thin wrapper around recipe.ParseFile for use in CLI commands.
// It uses the global constraintLookup for step analysis (same as loader).
func loadLocalRecipe(path string) (*recipe.Recipe, error) {
	return recipe.ParseFile(path, constraintLookup)
}

// printInfo prints an informational message unless quiet mode is enabled
func printInfo(a ...interface{}) {
	if !quietFlag {
		fmt.Println(a...)
	}
}

// printInfof prints a formatted informational message unless quiet mode is enabled
func printInfof(format string, a ...interface{}) {
	if !quietFlag {
		fmt.Printf(format, a...)
	}
}

// printJSON marshals the given value to JSON and prints it to stdout
func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		exitWithCode(ExitGeneral)
	}
}

// printError prints an error to stderr with suggestions if available.
// This uses the errmsg package to format errors with actionable suggestions.
func printError(err error) {
	errmsg.Fprint(os.Stderr, err)
}

// generateInstallPlan generates an installation plan for a tool.
// It handles both tool-from-registry and recipe-from-file cases.
//
// Parameters:
//   - ctx: Context for plan generation
//   - toolName: Name of the tool (used for logging and registry lookup if recipePath is empty)
//   - version: Version constraint (empty string means latest)
//   - recipePath: Path to local recipe file (empty string means load from registry)
//   - cfg: Configuration with paths for tools, download cache, and key cache
//
// Returns an InstallationPlan with dependencies embedded (if RecipeLoader is available).
func generateInstallPlan(
	ctx context.Context,
	toolName string,
	version string,
	recipePath string,
	cfg *config.Config,
) (*executor.InstallationPlan, error) {
	var r *recipe.Recipe
	var err error
	var recipeSource string

	// Load recipe from file or registry
	if recipePath != "" {
		r, err = recipe.ParseFile(recipePath, constraintLookup)
		recipeSource = "local"
	} else {
		r, err = loader.Get(toolName)
		recipeSource = "registry"
	}
	if err != nil {
		return nil, err
	}

	// Create executor
	exec, err := executor.New(r)
	if err != nil {
		return nil, err
	}
	defer exec.Cleanup()

	// Configure executor paths
	exec.SetToolsDir(cfg.ToolsDir)
	exec.SetAppsDir(cfg.AppsDir)
	exec.SetCurrentDir(cfg.CurrentDir)
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Set up downloader and cache for plan generation
	// Downloader enables Decompose to download files (e.g., GHCR bottles with auth)
	// DownloadCache persists downloads for reuse during plan execution
	predownloader := validate.NewPreDownloader()
	downloader := validate.NewPreDownloaderAdapter(predownloader)
	downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)

	// Generate plan with RecipeLoader to enable dependency embedding
	return exec.GeneratePlan(ctx, executor.PlanConfig{
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		RecipeSource:       recipeSource,
		Downloader:         downloader,
		DownloadCache:      downloadCache,
		RecipeLoader:       loader, // Enables self-contained plans with dependencies
		AutoAcceptEvalDeps: true,   // Auto-install eval-time deps (non-interactive)
		OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
			return installEvalDeps(deps, autoAccept)
		},
	})
}
