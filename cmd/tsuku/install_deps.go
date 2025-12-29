package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// planRetrievalConfig configures the plan retrieval flow.
type planRetrievalConfig struct {
	Tool              string // Tool name
	VersionConstraint string // User's version constraint (e.g., "14.1.0", "", "@lts")
	Fresh             bool   // If true, skip cache and regenerate plan
	OS                string // Target OS (defaults to runtime.GOOS)
	Arch              string // Target arch (defaults to runtime.GOARCH)
	RecipeHash        string // SHA256 hash of recipe TOML
	DownloadCacheDir  string // Directory for download cache (enables caching during Decompose)
}

// versionResolver abstracts version resolution for testing.
type versionResolver interface {
	ResolveVersion(ctx context.Context, constraint string) (string, error)
}

// planGenerator abstracts plan generation for testing.
type planGenerator interface {
	GeneratePlan(ctx context.Context, cfg executor.PlanConfig) (*executor.InstallationPlan, error)
}

// planCacheReader abstracts reading cached plans for testing.
type planCacheReader interface {
	GetCachedPlan(tool, version string) (*install.Plan, error)
}

// getOrGeneratePlan implements the two-phase plan retrieval flow:
// Phase 1 (resolution) always runs, then checks cache, then generates if needed.
// Returns a plan ready for ExecutePlan().
func getOrGeneratePlan(
	ctx context.Context,
	exec *executor.Executor,
	stateMgr *install.StateManager,
	cfg planRetrievalConfig,
) (*executor.InstallationPlan, error) {
	return getOrGeneratePlanWith(ctx, exec, exec, stateMgr, cfg)
}

// getOrGeneratePlanWith is the testable implementation that accepts interfaces.
func getOrGeneratePlanWith(
	ctx context.Context,
	resolver versionResolver,
	generator planGenerator,
	cacheReader planCacheReader,
	cfg planRetrievalConfig,
) (*executor.InstallationPlan, error) {
	// Apply defaults
	targetOS := cfg.OS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := cfg.Arch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}

	// Phase 1: Version Resolution (ALWAYS runs)
	resolvedVersion, err := resolver.ResolveVersion(ctx, cfg.VersionConstraint)
	if err != nil {
		// Fall back to "dev" version for recipes without proper version sources
		// This matches the behavior in executor.Execute() for backward compatibility
		printInfof("Warning: version resolution failed: %v, using 'dev'\n", err)
		resolvedVersion = "dev"
	}

	// Generate cache key from resolution output
	cacheKey := executor.CacheKeyFor(cfg.Tool, resolvedVersion, targetOS, targetArch, cfg.RecipeHash)

	// Check cache (unless --fresh)
	if !cfg.Fresh {
		cachedPlan, err := cacheReader.GetCachedPlan(cfg.Tool, resolvedVersion)
		if err == nil && cachedPlan != nil {
			// Convert storage plan to executor plan for validation
			execPlan := executor.FromStoragePlan(cachedPlan)
			if execPlan != nil {
				if err := executor.ValidateCachedPlan(execPlan, cacheKey); err == nil {
					printInfof("Using cached plan for %s@%s\n", cfg.Tool, resolvedVersion)
					return execPlan, nil
				}
				printInfof("Cached plan invalid, regenerating...\n")
			}
		}
	}

	// Generate fresh plan
	printInfof("Generating plan for %s@%s\n", cfg.Tool, resolvedVersion)

	// Create downloader and cache for plan generation
	// Downloader enables Decompose to download files (e.g., GHCR bottles with auth)
	// DownloadCache persists downloads for reuse during plan execution
	var downloadCache *actions.DownloadCache
	var downloader actions.Downloader
	if cfg.DownloadCacheDir != "" {
		downloadCache = actions.NewDownloadCache(cfg.DownloadCacheDir)
		predownloader := validate.NewPreDownloader()
		downloader = validate.NewPreDownloaderAdapter(predownloader)
	}

	return generator.GeneratePlan(ctx, executor.PlanConfig{
		OS:            targetOS,
		Arch:          targetArch,
		RecipeSource:  "registry",
		Downloader:    downloader,
		DownloadCache: downloadCache,
	})
}

// computeRecipeHashForPlan computes SHA256 hash of the recipe's TOML content.
func computeRecipeHashForPlan(r *recipe.Recipe) (string, error) {
	data, err := r.ToTOML()
	if err != nil {
		return "", fmt.Errorf("failed to serialize recipe: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func runInstallWithTelemetry(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, client *telemetry.Client) error {
	return installWithDependencies(toolName, reqVersion, versionConstraint, isExplicit, parent, make(map[string]bool), client)
}

// ensurePackageManagersForRecipe checks if a recipe uses package managers
// and auto-bootstraps them as execution dependencies if needed.
// It uses the central dependency resolver (actions.ResolveDependencies) to determine
// which dependencies are needed, then installs them via installWithDependencies.
// Returns a list of bin paths that should be added to PATH for execution.
func ensurePackageManagersForRecipe(mgr *install.Manager, r *recipe.Recipe, visited map[string]bool, telemetryClient *telemetry.Client) ([]string, error) {
	// Use the central dependency resolver to get install-time deps
	resolvedDeps := actions.ResolveDependencies(r)

	var execPaths []string
	processedDeps := make(map[string]bool)

	// Load state once for checking installed deps
	state, _ := mgr.GetState().Load()

	for depName := range resolvedDeps.InstallTime {
		// Skip if already processed in this call
		if processedDeps[depName] {
			continue
		}
		processedDeps[depName] = true

		// Check if already installed - if so, just get the bin path
		if state != nil {
			if _, exists := state.Installed[depName]; exists {
				binPath, err := findDependencyBinPath(mgr, depName)
				if err == nil {
					execPaths = append(execPaths, binPath)
				}
				continue
			}
		}

		printInfof("Ensuring dependency '%s' for package manager action...\n", depName)

		// Install the dependency using the standard mechanism
		// Use a fresh visited map to avoid false positives from parent installations
		depVisited := make(map[string]bool)
		if err := installWithDependencies(depName, "", "", false, r.Metadata.Name, depVisited, telemetryClient); err != nil {
			return nil, fmt.Errorf("failed to install dependency '%s': %w", depName, err)
		}

		// Find the installed binary path to add to execPaths
		binPath, err := findDependencyBinPath(mgr, depName)
		if err != nil {
			printInfof("Warning: could not find bin path for %s: %v\n", depName, err)
			continue
		}
		execPaths = append(execPaths, binPath)
	}

	return execPaths, nil
}

// findDependencyBinPath finds the bin directory for an installed dependency
func findDependencyBinPath(mgr *install.Manager, depName string) (string, error) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	state, err := mgr.GetState().Load()
	if err != nil {
		return "", err
	}

	toolState, exists := state.Installed[depName]
	if !exists {
		return "", fmt.Errorf("dependency %s not found in state", depName)
	}

	toolDir := cfg.ToolDir(depName, toolState.Version)
	binDir := filepath.Join(toolDir, "bin")

	// Verify the bin directory exists
	if _, err := os.Stat(binDir); err != nil {
		return "", fmt.Errorf("bin directory not found: %s", binDir)
	}

	return binDir, nil
}

func installWithDependencies(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, visited map[string]bool, telemetryClient *telemetry.Client) error {
	// Initialize manager for state updates
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	mgr := install.New(cfg)

	// If explicit install, check if tool is hidden and just expose it
	if isExplicit && parent == "" {
		wasHidden, err := install.CheckAndExposeHidden(mgr, toolName)
		if err != nil {
			printInfof("Warning: failed to check hidden status: %v\n", err)
		}
		if wasHidden {
			// Tool was hidden and is now exposed, we're done
			return nil
		}
	}

	// Check if already installed BEFORE checking for circular dependencies
	// This prevents false positives when multiple tools share a dependency
	tools, _ := mgr.List()
	isInstalled := false
	for _, t := range tools {
		if t.Name == toolName {
			isInstalled = true
			break
		}
	}

	if isInstalled {
		// Update state
		err := mgr.GetState().UpdateTool(toolName, func(ts *install.ToolState) {
			if isExplicit {
				ts.IsExplicit = true
			}
			if parent != "" {
				// Add parent to RequiredBy if not present
				found := false
				for _, r := range ts.RequiredBy {
					if r == parent {
						found = true
						break
					}
				}
				if !found {
					ts.RequiredBy = append(ts.RequiredBy, parent)
				}
			}
		})
		if err != nil {
			printInfof("Warning: failed to update state for %s: %v\n", toolName, err)
		}

		// If explicit update requested, we might want to proceed with re-installation
		// But for dependency check, we just return WITHOUT marking as visited
		// This allows shared dependencies to be recognized as already installed
		if !isExplicit && reqVersion == "" {
			return nil
		}
		// If it's an explicit install/update, we proceed
	}

	// Check for circular dependencies AFTER confirming tool isn't already installed
	// This ensures we only mark tools as visited when they're about to be processed
	if visited[toolName] {
		return fmt.Errorf("circular dependency detected: %s", toolName)
	}
	visited[toolName] = true

	// Load recipe
	r, err := loader.Get(toolName)
	if err != nil {
		printError(err)
		fmt.Fprintf(os.Stderr, "\nTo create a recipe from a package ecosystem:\n")
		fmt.Fprintf(os.Stderr, "  tsuku create %s --from <ecosystem>\n", toolName)
		fmt.Fprintf(os.Stderr, "\nAvailable ecosystems: crates.io, rubygems, pypi, npm\n")
		return err
	}

	// Validate the recipe before attempting installation
	// This runs the same validation as `tsuku validate` to catch issues early
	validationResult := recipe.ValidateRecipe(r)

	// Check for shadowed dependencies (declared deps already inherited from actions)
	shadowed := actions.DetectShadowedDeps(r)
	for _, dep := range shadowed {
		msg := fmt.Sprintf("dependency '%s' is already inherited from action '%s' (remove this redundant declaration)",
			dep.Name, dep.Source)
		validationResult.Warnings = append(validationResult.Warnings, recipe.ValidationWarning{
			Field:   "dependencies",
			Message: msg,
		})
	}

	// Fail on validation errors
	if !validationResult.Valid {
		printError(fmt.Errorf("recipe validation failed for '%s'", toolName))
		for _, e := range validationResult.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return fmt.Errorf("recipe validation failed")
	}

	// Show warnings (non-fatal)
	if len(validationResult.Warnings) > 0 {
		printInfof("Warnings for %s:\n", toolName)
		for _, w := range validationResult.Warnings {
			printInfof("  - %s\n", w)
		}
	}

	// Check if this is a library recipe
	if r.IsLibrary() {
		return installLibrary(toolName, reqVersion, parent, mgr, telemetryClient)
	}

	// Check for checksum verification (only warn for explicit installs)
	if isExplicit && !r.HasChecksumVerification() {
		fmt.Fprintf(os.Stderr, "Warning: Recipe '%s' does not include checksum verification.\n", toolName)
		fmt.Fprintf(os.Stderr, "The downloaded binary cannot be verified for integrity.\n")

		if !installForce {
			if isInteractive() {
				if !confirmInstall() {
					return fmt.Errorf("installation canceled by user")
				}
			} else {
				fmt.Fprintf(os.Stderr, "Use --force to proceed without verification.\n")
				return fmt.Errorf("checksum verification required (use --force to override)")
			}
		}
	}

	// Check and install dependencies
	if len(r.Metadata.Dependencies) > 0 {
		printInfof("Checking dependencies for %s...\n", toolName)

		for _, dep := range r.Metadata.Dependencies {
			printInfof("  Resolving dependency '%s'...\n", dep)
			// Install dependency (not explicit, parent is current tool)
			// Dependencies don't have version constraints and are tracked for telemetry
			if err := installWithDependencies(dep, "", "", false, toolName, visited, telemetryClient); err != nil {
				return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
			}
		}
	}

	// Auto-bootstrap package managers if recipe uses them
	// This must happen BEFORE checking runtime dependencies so that if a package manager
	// (like npm/nodejs) is also a runtime dependency, we can expose it
	execPaths, err := ensurePackageManagersForRecipe(mgr, r, visited, telemetryClient)
	if err != nil {
		return fmt.Errorf("failed to ensure package managers: %w", err)
	}

	// Check and install runtime dependencies (these must be exposed, not hidden)
	// This happens AFTER package manager bootstrap so CheckAndExposeHidden can work
	if len(r.Metadata.RuntimeDependencies) > 0 {
		printInfof("Checking runtime dependencies for %s...\n", toolName)

		for _, dep := range r.Metadata.RuntimeDependencies {
			printInfof("  Resolving runtime dependency '%s'...\n", dep)
			// Install runtime dependency as explicit (exposed, not hidden)
			// No parent - these are top-level explicit installs
			if err := installWithDependencies(dep, "", "", true, "", visited, telemetryClient); err != nil {
				return fmt.Errorf("failed to install runtime dependency '%s': %w", dep, err)
			}
		}
	}

	// Compute recipe hash for cache key
	recipeHash, err := computeRecipeHashForPlan(r)
	if err != nil {
		printInfof("Warning: failed to compute recipe hash: %v\n", err)
		// Continue without hash - cache lookup will always miss
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create executor: %v\n", err)
		return err
	}
	defer exec.Cleanup()

	// Set execution paths (for package managers like npm, pip, cargo)
	exec.SetExecPaths(execPaths)

	// Set tools directory for finding other installed tools
	exec.SetToolsDir(cfg.ToolsDir)

	// Set libraries directory for finding installed libraries
	exec.SetLibsDir(cfg.LibsDir)

	// Set download cache directory
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)

	// Set key cache directory for PGP signature verification
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Look up resolved dependency versions for variable expansion.
	// This is needed because dependencies are installed before plan generation,
	// so plan.Dependencies will be empty at execution time.
	if len(r.Metadata.Dependencies) > 0 {
		resolvedDeps := actions.ResolvedDeps{
			InstallTime: make(map[string]string),
		}
		state, _ := mgr.GetState().Load()
		for _, depName := range r.Metadata.Dependencies {
			// First, check if it's a library (installed in libs/)
			if libVersion := mgr.GetInstalledLibraryVersion(depName); libVersion != "" {
				resolvedDeps.InstallTime[depName] = libVersion
				continue
			}
			// Otherwise, check if it's a tool (installed in tools/)
			if state != nil {
				if depState, exists := state.Installed[depName]; exists {
					if depState.ActiveVersion != "" {
						resolvedDeps.InstallTime[depName] = depState.ActiveVersion
					} else if depState.Version != "" {
						// Fall back to deprecated Version field for old state files
						resolvedDeps.InstallTime[depName] = depState.Version
					}
				}
			}
		}
		exec.SetResolvedDeps(resolvedDeps)
	}

	// Get or generate installation plan (two-phase flow)
	planCfg := planRetrievalConfig{
		Tool:              toolName,
		VersionConstraint: versionConstraint,
		Fresh:             installFresh,
		RecipeHash:        recipeHash,
		DownloadCacheDir:  cfg.DownloadCacheDir,
	}
	plan, err := getOrGeneratePlan(globalCtx, exec, mgr.GetState(), planCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate plan: %v\n", err)
		return err
	}

	// Execute the plan
	if err := exec.ExecutePlan(globalCtx, plan); err != nil {
		// Handle ChecksumMismatchError specially - it has a user-friendly message
		var checksumErr *executor.ChecksumMismatchError
		if errors.As(err, &checksumErr) {
			fmt.Fprintf(os.Stderr, "\n%s\n", checksumErr.Error())
			return err
		}
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		return err
	}

	// Get version from plan (plan always has resolved version)
	version := plan.Version
	if version == "" {
		// Fallback for recipes without dynamic versioning
		version = "dev"
	}

	// Check if this exact version is already installed (multi-version support)
	// Skip installation if exact version exists, but still update state
	if mgr.IsVersionInstalled(toolName, version) {
		printInfof("%s@%s is already installed\n", toolName, version)
		// Still update state flags (is_explicit, required_by)
		err = mgr.GetState().UpdateTool(toolName, func(ts *install.ToolState) {
			if isExplicit {
				ts.IsExplicit = true
			}
			if parent != "" {
				found := false
				for _, r := range ts.RequiredBy {
					if r == parent {
						found = true
						break
					}
				}
				if !found {
					ts.RequiredBy = append(ts.RequiredBy, parent)
				}
			}
		})
		if err != nil {
			printInfof("Warning: failed to update state: %v\n", err)
		}
		return nil
	}

	// Install to permanent location
	// cfg is already loaded
	// mgr is already initialized

	// Check if this is a system dependency recipe (only require_system steps)
	// System dependencies are validated but not managed by tsuku
	isSystemDep := isSystemDependencyPlan(plan)

	if !isSystemDep {
		// Extract binaries from recipe to store in state
		binaries := r.ExtractBinaries()
		installOpts := install.DefaultInstallOptions()
		installOpts.Binaries = binaries
		installOpts.RequestedVersion = versionConstraint // Record what user asked for ("17", "@lts", "")

		// Store the plan using canonical conversion
		installOpts.Plan = executor.ToStoragePlan(plan)

		// Resolve all dependencies using the central resolution algorithm
		resolvedDeps := actions.ResolveDependencies(r)

		// Resolve runtime dependencies for wrapper generation (with versions)
		runtimeDeps := resolveRuntimeDeps(r, mgr)
		if len(runtimeDeps) > 0 {
			installOpts.RuntimeDependencies = runtimeDeps
			printInfof("Runtime dependencies: %v\n", mapKeys(runtimeDeps))
		}

		if err := mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install to permanent location: %v\n", err)
			return err
		}

		// Update state with explicit flag, parent, and dependencies
		err = mgr.GetState().UpdateTool(toolName, func(ts *install.ToolState) {
			if isExplicit {
				ts.IsExplicit = true
			}
			if parent != "" {
				// Add parent to RequiredBy if not present
				found := false
				for _, r := range ts.RequiredBy {
					if r == parent {
						found = true
						break
					}
				}
				if !found {
					ts.RequiredBy = append(ts.RequiredBy, parent)
				}
			}
			// Record dependencies in state for dependency tree display and uninstall warnings
			ts.InstallDependencies = mapKeys(resolvedDeps.InstallTime)
			ts.RuntimeDependencies = mapKeys(resolvedDeps.Runtime)
		})
		if err != nil {
			printInfof("Warning: failed to update state: %v\n", err)
		}
	}

	// Update used_by for any library dependencies now that we know the tool version
	toolNameVersion := fmt.Sprintf("%s-%s", toolName, version)
	for _, dep := range r.Metadata.Dependencies {
		// Load dependency recipe to check if it's a library
		depRecipe, err := loader.Get(dep)
		if err != nil {
			continue // Skip if recipe not found
		}
		if depRecipe.IsLibrary() {
			// Get installed library version
			libVersion := mgr.GetInstalledLibraryVersion(dep)
			if libVersion != "" {
				if err := mgr.AddLibraryUsedBy(dep, libVersion, toolNameVersion); err != nil {
					printInfof("Warning: failed to update library state for %s: %v\n", dep, err)
				}
			}
		}
	}

	// Send telemetry event on successful installation
	if telemetryClient != nil {
		// isDependency is true when isExplicit is false (installed as a dependency)
		event := telemetry.NewInstallEvent(toolName, versionConstraint, version, !isExplicit)
		telemetryClient.Send(event)
	}

	printInfo()
	if isSystemDep {
		printInfof("✓ %s is available on your system\n", toolName)
		printInfo()
		printInfo("Note: tsuku doesn't manage this dependency. It validated that it's installed.")
	} else {
		printInfo("Installation successful!")
		printInfo()
		printInfo("To use the installed tool, add this to your shell profile:")
		printInfof("  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)
	}

	return nil
}

// resolveRuntimeDeps uses the new dependency resolution to get runtime dependencies
// and looks up their installed versions from state.
// Returns a map of dep name -> version for use in wrapper scripts.
func resolveRuntimeDeps(r *recipe.Recipe, mgr *install.Manager) map[string]string {
	// Use the new dependency resolution algorithm
	deps := actions.ResolveDependencies(r)

	if len(deps.Runtime) == 0 {
		return nil
	}

	// Look up installed versions for each runtime dep
	result := make(map[string]string)
	for depName := range deps.Runtime {
		toolState, err := mgr.GetState().GetToolState(depName)
		if err != nil || toolState == nil {
			// Dependency not installed - skip (shouldn't happen if install order is correct)
			printInfof("Warning: runtime dependency %s not found in state\n", depName)
			continue
		}
		result[depName] = toolState.Version
	}

	return result
}

// mapKeys returns the keys of a map as a slice (for display)
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// isSystemDependencyPlan returns true if the plan only contains require_system steps.
// System dependency recipes validate that external tools are installed but don't
// actually install anything, so they shouldn't create state entries or directories.
func isSystemDependencyPlan(plan *executor.InstallationPlan) bool {
	if plan == nil || len(plan.Steps) == 0 {
		return false
	}
	for _, step := range plan.Steps {
		if step.Action != "require_system" {
			return false
		}
	}
	return true
}
