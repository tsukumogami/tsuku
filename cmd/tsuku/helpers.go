package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/buildinfo"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/errmsg"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/registry"
	"github.com/tsukumogami/tsuku/internal/validate"
	"github.com/tsukumogami/tsuku/internal/version"
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

// deprecationWarningOnce ensures the deprecation warning fires at most once
// per CLI invocation.
var deprecationWarningOnce sync.Once

// resetDeprecationWarning resets the sync.Once for testing purposes.
func resetDeprecationWarning() {
	deprecationWarningOnce = sync.Once{}
}

// printWarning prints a warning message to stderr unless quiet mode is enabled.
func printWarning(msg string) {
	if !quietFlag {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// isDevBuild returns true if the given version string represents a development build.
// Dev builds use version strings like "dev-<hash>", "dev", or "unknown".
func isDevBuild(ver string) bool {
	return ver == "dev" || ver == "unknown" || strings.HasPrefix(ver, "dev-")
}

// checkDeprecationWarning checks a manifest for deprecation notices and prints
// a warning to stderr at most once per CLI invocation. The registryURL identifies
// which registry issued the notice.
func checkDeprecationWarning(manifest *registry.Manifest, registryURL string) {
	if manifest == nil || manifest.Deprecation == nil {
		return
	}

	deprecationWarningOnce.Do(func() {
		msg := formatDeprecationWarning(manifest.Deprecation, registryURL, buildinfo.Version())
		printWarning(msg)
	})
}

// formatDeprecationWarning builds the deprecation warning string. Accepts the
// CLI version as a parameter so the version comparison branches are testable.
//
// The warning leads with the actionable information (what version to upgrade to,
// by when) rather than internal details like schema version numbers.
func formatDeprecationWarning(dep *registry.DeprecationNotice, registryURL, cliVersion string) string {
	var b strings.Builder

	if isDevBuild(cliVersion) {
		// Dev builds: show the registry message as-is, no version guidance
		fmt.Fprintf(&b, "Warning: %s", dep.Message)
		if dep.SunsetDate != "" {
			fmt.Fprintf(&b, " (by %s)", dep.SunsetDate)
		}
		return b.String()
	}

	if dep.MinCLIVersion != "" {
		cmp := version.CompareVersions(cliVersion, dep.MinCLIVersion)
		if cmp >= 0 {
			// CLI already meets the minimum version
			fmt.Fprintf(&b, "Warning: %s", dep.Message)
			if dep.SunsetDate != "" {
				fmt.Fprintf(&b, " (by %s)", dep.SunsetDate)
			}
			fmt.Fprintf(&b, "\n  Your CLI (%s) is already compatible. Run 'tsuku update-registry' after the migration.", cliVersion)
		} else {
			// CLI needs an upgrade
			fmt.Fprintf(&b, "Warning: tsuku %s or later is required by %s", dep.MinCLIVersion, registryURL)
			if dep.SunsetDate != "" {
				fmt.Fprintf(&b, " after %s", dep.SunsetDate)
			}
			fmt.Fprint(&b, ".")
			fmt.Fprint(&b, "\n  Upgrade: curl -fsSL https://get.tsuku.dev/now | bash")
		}
	} else {
		// No min version specified, just show the message
		fmt.Fprintf(&b, "Warning: %s", dep.Message)
		if dep.SunsetDate != "" {
			fmt.Fprintf(&b, " (by %s)", dep.SunsetDate)
		}
	}

	return b.String()
}

// recipeSourceFromProvider maps a recipe.RecipeSource to the source string
// stored in ToolState.Source and Plan.RecipeSource. The mapping normalizes
// provider-level source tags to the user-facing values used for source tracking:
//   - SourceRegistry ("registry") -> "central"
//   - SourceEmbedded ("embedded") -> "central" (embedded is part of the central registry)
//   - SourceLocal ("local") -> "local"
//
// Embedded recipes are treated as "central" because they're a cached subset of the
// central registry bundled into the binary. For update/outdated/verify purposes,
// they should check the central registry for newer versions.
func recipeSourceFromProvider(source recipe.RecipeSource) string {
	switch source {
	case recipe.SourceLocal:
		return string(recipe.SourceLocal)
	case recipe.SourceEmbedded:
		return recipe.SourceCentral
	case recipe.SourceRegistry:
		return recipe.SourceCentral
	default:
		// Distributed sources pass through as "owner/repo"
		return string(source)
	}
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
//   - linuxFamily: Target Linux family override (empty means auto-detect from host)
//
// Returns an InstallationPlan with dependencies embedded (if RecipeLoader is available).
func generateInstallPlan(
	ctx context.Context,
	toolName string,
	version string,
	recipePath string,
	cfg *config.Config,
	linuxFamily string,
) (*executor.InstallationPlan, error) {
	var r *recipe.Recipe
	var err error
	var recipeSource string

	// Load recipe from file or registry
	if recipePath != "" {
		r, err = recipe.ParseFile(recipePath, constraintLookup)
		recipeSource = "local"
	} else {
		var source recipe.RecipeSource
		r, source, err = loader.GetWithSource(toolName, recipe.LoaderOptions{})
		if err == nil {
			recipeSource = recipeSourceFromProvider(source)
		}
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
	exec.SetSkipCacheSecurityChecks(installSkipSecurity)
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Set up downloader and cache for plan generation
	// Downloader enables Decompose to download files (e.g., GHCR bottles with auth)
	// DownloadCache persists downloads for reuse during plan execution
	predownloader := validate.NewPreDownloader()
	downloader := validate.NewPreDownloaderAdapter(predownloader)
	downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)
	downloadCache.SetSkipSecurityChecks(installSkipSecurity)

	// Generate plan with RecipeLoader to enable dependency embedding
	return exec.GeneratePlan(ctx, executor.PlanConfig{
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		LinuxFamily:        linuxFamily,
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
