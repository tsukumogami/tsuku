package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/shellenv"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// planRetrievalConfig configures the plan retrieval flow.
type planRetrievalConfig struct {
	Tool              string               // Tool name
	VersionConstraint string               // User's version constraint (e.g., "14.1.0", "", "@lts")
	Fresh             bool                 // If true, skip cache and regenerate plan
	OS                string               // Target OS (defaults to runtime.GOOS)
	Arch              string               // Target arch (defaults to runtime.GOARCH)
	DownloadCacheDir  string               // Directory for download cache (enables caching during Decompose)
	RecipeLoader      actions.RecipeLoader // Recipe loader for dependency resolution (enables self-contained plans)
	RequireEmbedded   bool                 // Require action dependencies to resolve from embedded registry
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
	reporter progress.Reporter,
) (*executor.InstallationPlan, error) {
	return getOrGeneratePlanWith(ctx, exec, exec, stateMgr, cfg, reporter)
}

// getOrGeneratePlanWith is the testable implementation that accepts interfaces.
func getOrGeneratePlanWith(
	ctx context.Context,
	resolver versionResolver,
	generator planGenerator,
	cacheReader planCacheReader,
	cfg planRetrievalConfig,
	reporter progress.Reporter,
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
		// If user requested a specific version, fail with error instead of falling back
		if cfg.VersionConstraint != "" {
			return nil, fmt.Errorf("version resolution failed: %w", err)
		}
		// Fall back to "dev" version for recipes without proper version sources
		// This matches the behavior in executor.Execute() for backward compatibility
		reporter.Warn("version resolution failed: %v, using 'dev'", err)
		resolvedVersion = "dev"
	}

	// Generate cache key from resolution output
	cacheKey := executor.CacheKeyFor(cfg.Tool, resolvedVersion, targetOS, targetArch)

	// Check cache (unless --fresh)
	if !cfg.Fresh {
		cachedPlan, err := cacheReader.GetCachedPlan(cfg.Tool, resolvedVersion)
		if err == nil && cachedPlan != nil {
			// Convert storage plan to executor plan for validation
			execPlan := executor.FromStoragePlan(cachedPlan)
			if execPlan != nil {
				if err := executor.ValidateCachedPlan(execPlan, cacheKey); err == nil {
					reporter.Status(fmt.Sprintf("Using cached plan for %s@%s", cfg.Tool, resolvedVersion))
					return execPlan, nil
				}
				reporter.Status("Cached plan invalid, regenerating...")
			}
		}
	}

	// Generate fresh plan
	reporter.Status(fmt.Sprintf("Generating plan for %s@%s", cfg.Tool, resolvedVersion))

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
		OS:                 targetOS,
		Arch:               targetArch,
		RecipeSource:       "registry",
		Downloader:         downloader,
		DownloadCache:      downloadCache,
		RecipeLoader:       cfg.RecipeLoader,
		RequireEmbedded:    cfg.RequireEmbedded,
		AutoAcceptEvalDeps: true, // Auto-install eval-time dependencies during install
		OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
			return installEvalDeps(deps, autoAccept)
		},
	})
}

// installEvalDepsCallback returns a callback suitable for
// Executor.SetEvalDepsCallback that auto-installs missing eval-time
// dependencies during the install path.
func installEvalDepsCallback(deps []string, autoAccept bool) error {
	return installEvalDeps(deps, autoAccept)
}

func runInstallWithTelemetry(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, client *telemetry.Client) error {
	reporter := progress.NewTTYReporter(os.Stderr)
	defer func() {
		reporter.Stop()
		reporter.FlushDeferred()
	}()
	return installWithDependencies(toolName, reqVersion, versionConstraint, isExplicit, parent, make(map[string]bool), client, reporter)
}

// runInstallWithExternalReporter runs the install flow using a caller-provided
// reporter. The caller owns the reporter lifecycle (Stop/FlushDeferred). Use
// this when the caller needs to emit a permanent outcome line via the same
// reporter after the install completes, so TTY spinner replacement works
// correctly without mixing output streams.
func runInstallWithExternalReporter(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, client *telemetry.Client, reporter progress.Reporter) error {
	return installWithDependencies(toolName, reqVersion, versionConstraint, isExplicit, parent, make(map[string]bool), client, reporter)
}

func installWithDependencies(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, visited map[string]bool, telemetryClient *telemetry.Client, reporter progress.Reporter) error {
	// Initialize manager for state updates
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	mgr := install.New(cfg)
	mgr.SetReporter(reporter)

	// If explicit install, check if tool is hidden and just expose it
	if isExplicit && parent == "" {
		wasHidden, err := install.CheckAndExposeHidden(mgr, toolName)
		if err != nil {
			reporter.Warn("failed to check hidden status: %v", err)
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
			reporter.Warn("failed to update state for %s: %v", toolName, err)
		}

		// If explicit update requested, we might want to proceed with re-installation
		// But for dependency check, we just return WITHOUT marking as visited
		// This allows shared dependencies to be recognized as already installed
		if !isExplicit && reqVersion == "" {
			setInstalledInIndex(toolName, true)
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
	r, err := loader.Get(toolName, recipe.LoaderOptions{})
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
	for _, w := range validationResult.Warnings {
		reporter.Warn("%s: %s", toolName, w)
	}

	// Check if this is a library recipe
	if r.IsLibrary() {
		return installLibrary(toolName, reqVersion, mgr, telemetryClient, reporter)
	}

	// Check and display system dependency instructions (for explicit installs only)
	// System deps are displayed before proceeding, allowing users to install them manually
	if isExplicit && !quietFlag && hasSystemDeps(r) {
		target, err := resolveTarget(installTargetFamily)
		if err != nil {
			return fmt.Errorf("failed to resolve target: %w", err)
		}

		if displaySystemDeps(r, target) {
			// System deps were displayed - exit without error
			// User should run the commands shown and try again
			return nil
		}
	}

	// Check for checksum verification (only warn for explicit installs)
	if isExplicit {
		switch r.GetChecksumVerification() {
		case recipe.ChecksumDynamic:
			// Downloads without static checksums — inform but don't block.
			// The plan generator computes checksums at install time.
			reporter.Log("Note: Checksums for '%s' will be computed during installation.", toolName)

		case recipe.ChecksumEcosystem, recipe.ChecksumStatic:
			// Ecosystem verification or static checksums — silent.
		}
	}

	// Check and install dependencies
	if len(r.Metadata.Dependencies) > 0 {
		reporter.Status(fmt.Sprintf("Checking dependencies for %s...", toolName))

		for _, dep := range r.Metadata.Dependencies {
			reporter.Status(fmt.Sprintf("Resolving dependency '%s'...", dep))
			// Install dependency (not explicit, parent is current tool)
			// Dependencies don't have version constraints and are tracked for telemetry
			if err := installWithDependencies(dep, "", "", false, toolName, visited, telemetryClient, reporter); err != nil {
				return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
			}
		}
	}

	// Check and install runtime dependencies (these must be exposed, not hidden)
	// This happens AFTER package manager bootstrap so CheckAndExposeHidden can work
	if len(r.Metadata.RuntimeDependencies) > 0 {
		reporter.Status(fmt.Sprintf("Checking runtime dependencies for %s...", toolName))

		for _, dep := range r.Metadata.RuntimeDependencies {
			reporter.Status(fmt.Sprintf("Resolving runtime dependency '%s'...", dep))
			// Install runtime dependency as explicit (exposed, not hidden)
			// No parent - these are top-level explicit installs
			if err := installWithDependencies(dep, "", "", true, "", visited, telemetryClient, reporter); err != nil {
				return fmt.Errorf("failed to install runtime dependency '%s': %w", dep, err)
			}
		}
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

	// Auto-install eval-time deps during the install path. This is what
	// makes Executor.ResolveVersion (the cache-key step below) and
	// resolveVersionWith (later, in plan generation) agree on the
	// version: both probe the same bundled python-standalone, which is
	// installed here if missing.
	exec.SetEvalDepsCallback(installEvalDepsCallback, true)

	// Set tools directory for finding other installed tools
	exec.SetToolsDir(cfg.ToolsDir)

	// Set libraries directory for finding installed libraries
	exec.SetLibsDir(cfg.LibsDir)

	// Set apps directory for macOS .app bundles
	exec.SetAppsDir(cfg.AppsDir)

	// Set current directory for binary symlinks
	exec.SetCurrentDir(cfg.CurrentDir)

	// Set download cache directory
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)
	exec.SetSkipCacheSecurityChecks(installSkipSecurity)

	// Set key cache directory for PGP signature verification
	exec.SetKeyCacheDir(cfg.KeyCacheDir)

	// Pass through --no-shell-init flag
	exec.SetNoShellInit(installNoShellInit)

	// Propagate the shared reporter to all execution contexts
	exec.SetReporter(reporter)

	// Look up resolved dependency versions for variable expansion
	// This mirrors the logic in installLibrary() for libraries
	if len(r.Metadata.Dependencies) > 0 {
		resolvedDeps := actions.ResolvedDeps{
			InstallTime: make(map[string]string),
		}
		for _, depName := range r.Metadata.Dependencies {
			// First, check if it's a library (installed in libs/)
			if libVersion := mgr.GetInstalledLibraryVersion(depName); libVersion != "" {
				resolvedDeps.InstallTime[depName] = libVersion
				continue
			}
			// Otherwise, check if it's a tool (installed in tools/)
			if toolState, err := mgr.GetState().GetToolState(depName); err == nil && toolState != nil {
				if toolState.ActiveVersion != "" {
					resolvedDeps.InstallTime[depName] = toolState.ActiveVersion
				} else if toolState.Version != "" {
					resolvedDeps.InstallTime[depName] = toolState.Version
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
		DownloadCacheDir:  cfg.DownloadCacheDir,
		RecipeLoader:      loader,
		RequireEmbedded:   installRequireEmbedded,
	}
	plan, err := getOrGeneratePlan(globalCtx, exec, mgr.GetState(), planCfg, reporter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate plan: %v\n", err)
		return err
	}

	// Short-circuit: if the resolved version is already installed, skip plan
	// execution entirely. This avoids re-downloading and re-extracting tools
	// that are already present (e.g., during idempotent `tsuku install -y`).
	planVersion := plan.Version
	if planVersion == "" {
		planVersion = "dev"
	}
	if mgr.IsVersionInstalled(toolName, planVersion) {
		reporter.Status(fmt.Sprintf("%s@%s is already installed", toolName, planVersion))
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
			reporter.Warn("failed to update state: %v", err)
		}
		setInstalledInIndex(toolName, true)
		return nil
	}

	// Emit the install-start line now that the version is resolved from the plan.
	reporter.Status(fmt.Sprintf("Installing %s@%s", toolName, planVersion))

	// Execute the plan
	if err := exec.ExecutePlan(globalCtx, plan); err != nil {
		reporter.Log("❌ %s@%s", toolName, planVersion)
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
		runtimeDeps := resolveRuntimeDeps(r, mgr, reporter)
		if len(runtimeDeps) > 0 {
			installOpts.RuntimeDependencies = runtimeDeps
			reporter.Status(fmt.Sprintf("Runtime dependencies: %v", mapKeys(runtimeDeps)))
		}

		if err := mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install to permanent location: %v\n", err)
			return err
		}
		setInstalledInIndex(toolName, true)

		// Execute post-install phase (e.g., install_shell_init).
		// The ToolInstallDir must point to the final installed location so
		// source_command can find the tool's binary.
		exec.SetToolInstallDir(cfg.ToolDir(toolName, version))
		if err := exec.ExecutePhase(globalCtx, plan, "post-install"); err != nil {
			// Post-install failures warn but don't block installation
			reporter.Warn("post-install phase failed: %v", err)
		}

		// Collect cleanup actions recorded by post-install actions and
		// rebuild shell caches for any shells that were written to.
		postInstallCleanup := exec.GetCleanupActions()
		if len(postInstallCleanup) > 0 {
			affectedShells := make(map[string]bool)
			for _, ca := range postInstallCleanup {
				if shell := install.ShellFromCleanupPath(ca.Path); shell != "" {
					affectedShells[shell] = true
				}
			}
			for shell := range affectedShells {
				if err := shellenv.RebuildShellCache(cfg.HomeDir, shell); err != nil {
					reporter.Warn("failed to rebuild shell cache for %s: %v", shell, err)
				}
			}
		}

		// Update state with explicit flag, parent, dependencies, and cleanup actions
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

			// Store cleanup actions from post-install phase in the version state
			if len(postInstallCleanup) > 0 && ts.Versions != nil {
				if vs, ok := ts.Versions[version]; ok {
					vs.CleanupActions = convertCleanupActions(postInstallCleanup)
					ts.Versions[version] = vs
				}
			}
		})
		if err != nil {
			reporter.Warn("failed to update state: %v", err)
		}
	}

	// Update used_by for any library dependencies now that we know the tool version
	toolNameVersion := fmt.Sprintf("%s-%s", toolName, version)
	for _, dep := range r.Metadata.Dependencies {
		// Load dependency recipe to check if it's a library
		depRecipe, err := loader.Get(dep, recipe.LoaderOptions{})
		if err != nil {
			continue // Skip if recipe not found
		}
		if depRecipe.IsLibrary() {
			// Get installed library version
			libVersion := mgr.GetInstalledLibraryVersion(dep)
			if libVersion != "" {
				if err := mgr.AddLibraryUsedBy(dep, libVersion, toolNameVersion); err != nil {
					reporter.Warn("failed to update library state for %s: %v", dep, err)
				}
			}
		}
	}

	// Verify installation before reporting success
	// Skip verification for system dependencies (require_system only recipes)
	if !isSystemDep {
		if r.Verify != nil && r.Verify.Command != "" {
			reporter.Status(fmt.Sprintf("Verifying %s@%s", toolName, version))

			// Get the tool state for verification
			toolState, err := mgr.GetState().GetToolState(toolName)
			if err != nil {
				return fmt.Errorf("failed to get tool state for verification: %w", err)
			}

			// Load state for dependency validation
			state, err := mgr.GetState().Load()
			if err != nil {
				return fmt.Errorf("failed to load state for verification: %w", err)
			}

			// Verbose: false — post-install; sub-step output is noise during install flow.
			// The tsuku verify command passes Verbose: true to show full detail.
			opts := ToolVerifyOptions{Verbose: false, SkipPATHChecks: true, SkipDependencyValidation: true}
			if err := RunToolVerification(r, toolName, toolState, cfg, state, opts); err != nil {
				return fmt.Errorf("installation verification failed: %w", err)
			}
		} else {
			reporter.Log("Note: Recipe has no verify command, skipping verification")
		}
	}

	// Send telemetry event on successful installation
	if telemetryClient != nil {
		// isDependency is true when isExplicit is false (installed as a dependency)
		event := telemetry.NewInstallEvent(toolName, versionConstraint, version, !isExplicit)
		telemetryClient.Send(event)
	}

	if isSystemDep {
		reporter.Log("%s is available on your system", toolName)
		reporter.Log("Note: tsuku doesn't manage this dependency. It validated that it's installed.")
	} else {
		reporter.Log("✅ %s@%s", toolName, version)
		if isExplicit && parent == "" {
			reporter.DeferWarn("To use the installed tool, add this to your shell profile:\n  export PATH=\"%s:$PATH\"", cfg.CurrentDir)
		}
	}

	return nil
}

// resolveRuntimeDeps uses the new dependency resolution to get runtime dependencies
// and looks up their installed versions from state.
// Returns a map of dep name -> version for use in wrapper scripts.
func resolveRuntimeDeps(r *recipe.Recipe, mgr *install.Manager, reporter progress.Reporter) map[string]string {
	// Use the new dependency resolution algorithm
	deps := actions.ResolveDependencies(r)

	if len(deps.Runtime) == 0 {
		return nil
	}

	// Look up installed versions for each runtime dep
	result := make(map[string]string)
	for depName := range deps.Runtime {
		// Check library state first (libraries are installed to $TSUKU_HOME/libs/)
		if libVersion := mgr.GetInstalledLibraryVersion(depName); libVersion != "" {
			result[depName] = libVersion
			continue
		}
		// Fall back to tool state (tools are installed to $TSUKU_HOME/tools/)
		toolState, err := mgr.GetState().GetToolState(depName)
		if err != nil || toolState == nil {
			// Dependency not installed - skip (shouldn't happen if install order is correct)
			reporter.Warn("runtime dependency %s not found in state", depName)
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

// convertCleanupActions converts executor-level CleanupActions to state-level
// CleanupActions. The two types mirror each other but live in different packages.
func convertCleanupActions(execActions []actions.CleanupAction) []install.CleanupAction {
	result := make([]install.CleanupAction, len(execActions))
	for i, a := range execActions {
		result[i] = install.CleanupAction{
			Action:      a.Action,
			Path:        a.Path,
			ContentHash: a.ContentHash,
		}
	}
	return result
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

// clearAndRecordInstallSuccess checks whether toolName had a failure notice and,
// if so, removes it and writes a success notice with the tool's current active version.
// Best-effort: silently ignores errors so notice management never fails an install.
func clearAndRecordInstallSuccess(toolName string) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return
	}
	noticesDir := notices.NoticesDir(cfg.HomeDir)
	existing, err := notices.ReadNotice(noticesDir, toolName)
	if err != nil || existing == nil || existing.Error == "" {
		return
	}
	var activeVersion string
	mgr := install.New(cfg)
	if tools, err := mgr.List(); err == nil {
		for _, t := range tools {
			if t.Name == toolName && t.IsActive {
				activeVersion = t.Version
				break
			}
		}
	}
	_ = notices.RemoveNotice(noticesDir, toolName)
	if activeVersion != "" {
		_ = notices.WriteNotice(noticesDir, &notices.Notice{
			Tool:             toolName,
			AttemptedVersion: activeVersion,
			Error:            "",
			Timestamp:        time.Now(),
			Shown:            false,
		})
	}
}
