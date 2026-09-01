package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/recipe"
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
		// A record below the current storage version was written by a
		// conversion that dropped the plan's dependencies, verify block, and
		// recipe type. Nil dependencies and a tool with no dependencies are
		// indistinguishable on disk, so the record cannot be trusted and the
		// plan is regenerated instead.
		if cachedPlan != nil && cachedPlan.StorageVersion < install.PlanStorageVersion {
			reporter.Status("Cached plan predates the current storage format, regenerating...")
			cachedPlan = nil
		}
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
		Reporter:           reporter,
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

// shouldInstallRuntimeDep reports whether depRecipe applies to the current
// runtime platform. A recipe with platform constraints (supported_os,
// supported_arch, unsupported_platforms, supported_libc) that exclude the
// current GOOS/GOARCH/libc must be skipped during the runtime_dependencies
// walk: its source-build steps may lack platform when-clauses and would
// fail with an unrelated error if the installer fell through to them.
//
// Returns true if depRecipe is nil so a missing recipe defers the decision
// to the recursive installWithDependencies call (which will surface the
// lookup error in its standard form).
func shouldInstallRuntimeDep(depRecipe *recipe.Recipe) bool {
	if depRecipe == nil {
		return true
	}
	return depRecipe.SupportsPlatformRuntime()
}

// installArgs carries the per-invocation inputs threaded through the
// install pipeline. Bundling these into a struct lets the recursive
// dependency walk derive child invocations via a copy-and-override
// pattern (`sub := args; sub.Tool = dep; ...`) instead of forwarding
// 10 positional parameters by hand. Request-scoped metadata that is
// orthogonal to a single install -- the cancellation context, the
// installevents.Source tag carried on ctx, and the visited-set used
// to detect cyclic dep walks -- is passed alongside `args`, not on it.
type installArgs struct {
	Tool              string
	ReqVersion        string
	VersionConstraint string
	IsExplicit        bool
	Parent            string
	Reporter          progress.Reporter
	TelemetryClient   *telemetry.Client

	// Reinstall re-runs the installation for a version that is already
	// installed, instead of reporting it as present and returning. It lives on
	// the args rather than being read from the --reinstall flag directly
	// because the dependency walk derives child invocations from this struct:
	// a package-level read would silently cascade the reinstall into every
	// dependency, and reinstall is scoped to the tool the user named.
	Reinstall bool
}

// withInstallFlags applies the install command's package-level flags to a set
// of arguments. It exists so the flags reach the pipeline from one place: the
// install command builds installArgs at seven call sites, and a flag added to
// six of them is a flag that silently does nothing on the seventh.
//
// runInstall applies it; runInstallWithReporter does not. That is not an
// oversight -- only `tsuku update` and the auto-apply path call the borrowed-
// reporter entry point, and neither registers an install flag, so applying them
// there would read every value as false and claim a wiring that does not exist.
func withInstallFlags(args installArgs) installArgs {
	args.Reinstall = installReinstall
	return args
}

func runInstall(ctx context.Context, args installArgs) error {
	reporter := progress.NewTTYReporter(os.Stderr)
	defer func() {
		reporter.Stop()
		reporter.FlushDeferred()
	}()
	args.Reporter = reporter
	return installWithDependencies(ctx, withInstallFlags(args), make(map[string]bool))
}

// runInstallWithReporter runs the install flow using a caller-provided
// reporter. The caller owns the reporter lifecycle (Stop/FlushDeferred). Use
// this when the caller needs to emit a permanent outcome line via the same
// reporter after the install completes, so TTY spinner replacement works
// correctly without mixing output streams.
func runInstallWithReporter(ctx context.Context, args installArgs) error {
	return installWithDependencies(ctx, args, make(map[string]bool))
}

func installWithDependencies(ctx context.Context, args installArgs, visited map[string]bool) error {
	toolName := args.Tool
	reqVersion := args.ReqVersion
	versionConstraint := args.VersionConstraint
	isExplicit := args.IsExplicit
	parent := args.Parent
	reporter := args.Reporter
	telemetryClient := args.TelemetryClient
	reinstall := args.Reinstall

	// Initialize manager for state updates. The event bus dispatches
	// lifecycle events to the notices and telemetry subscribers; src
	// tags every event so subscribers can distinguish manual / auto /
	// project-auto triggers.
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	bus := newEventBus(cfg, telemetryClient)
	mgr := install.New(cfg, install.WithEventBus(bus))
	mgr.SetReporter(reporter)

	// If explicit install, check if tool is hidden and just expose it.
	//
	// Not when a version was asked for: exposing links whatever version the
	// entry already records, so taking this shortcut for `tsuku install foo@2`
	// while foo@1 sits there hidden would hand the user version 1 and report
	// success. Falling through installs the version they named.
	//
	// Not under --reinstall either. Exposing only creates symlinks; it never
	// touches the payload. Someone repairing a hidden tool's files would get
	// links to the same broken files and a success message.
	if isExplicit && parent == "" && reqVersion == "" && versionConstraint == "" && !reinstall {
		wasHidden, err := install.CheckAndExposeHidden(ctx, mgr, toolName)
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
		// Update state via semantic Manager methods.
		if err := recordInstallRelationship(mgr, toolName, parent, isExplicit); err != nil {
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
		return installLibrary(ctx, toolName, reqVersion, reinstall, mgr, telemetryClient, reporter)
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

	// Check and install dependencies
	if len(r.Metadata.Dependencies) > 0 {
		reporter.Status(fmt.Sprintf("Checking dependencies for %s...", toolName))

		for _, dep := range r.Metadata.Dependencies {
			reporter.Status(fmt.Sprintf("Resolving dependency '%s'...", dep))
			// Install dependency (not explicit, parent is current tool).
			// Dependencies don't have version constraints and are tracked
			// for telemetry. Construct the sub-call by copy-and-override on
			// the parent args rather than threading positional parameters.
			sub := args
			sub.Tool = dep
			sub.IsExplicit = false
			sub.Parent = toolName
			sub.ReqVersion = ""
			sub.VersionConstraint = ""
			// Reinstall is scoped to the tool the user named. A dependency that
			// is already installed stays as it is; one that is missing is
			// installed normally. Carrying the flag down would rewrite every
			// tree in the dependency graph for a repair aimed at one tool.
			sub.Reinstall = false
			if err := installWithDependencies(ctx, sub, visited); err != nil {
				return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
			}
			// Cooperative cancellation: when ctx is canceled mid-walk, stop
			// before processing the next dep so we don't begin a fresh
			// install that will itself observe the cancellation later. The
			// recursive call above may have completed cleanly even with a
			// canceled ctx (e.g. the short-circuit path for an already-
			// installed tool); this check ensures we don't continue past it.
			if cerr := ctx.Err(); cerr != nil {
				return fmt.Errorf("install canceled: %w", cerr)
			}
		}
	}

	// Check and install runtime dependencies (these must be exposed, not hidden)
	// This happens AFTER package manager bootstrap so CheckAndExposeHidden can work
	if len(r.Metadata.RuntimeDependencies) > 0 {
		reporter.Status(fmt.Sprintf("Checking runtime dependencies for %s...", toolName))

		for _, dep := range r.Metadata.RuntimeDependencies {
			reporter.Status(fmt.Sprintf("Resolving runtime dependency '%s'...", dep))
			// Skip deps that don't apply to the current platform (e.g. a
			// darwin-only dep declared by a cross-platform recipe). The skip
			// is silent: the SONAME completeness scanner is the right surface
			// for warning when the binary's NEEDED list references a SONAME
			// that no platform-supported recipe ships.
			depRecipe, depErr := loader.Get(dep, recipe.LoaderOptions{})
			if depErr == nil && !shouldInstallRuntimeDep(depRecipe) {
				continue
			}
			// Install runtime dependency as explicit (exposed, not hidden).
			// No parent -- these are top-level explicit installs.
			sub := args
			sub.Tool = dep
			sub.IsExplicit = true
			sub.Parent = ""
			sub.ReqVersion = ""
			sub.VersionConstraint = ""
			// Same scoping as the install-time dependency loop above.
			sub.Reinstall = false
			if err := installWithDependencies(ctx, sub, visited); err != nil {
				return fmt.Errorf("failed to install runtime dependency '%s': %w", dep, err)
			}
			// Cooperative cancellation between iterations -- see the matching
			// check in the install-dependencies loop above.
			if cerr := ctx.Err(); cerr != nil {
				return fmt.Errorf("install canceled: %w", cerr)
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
	if len(r.Metadata.Dependencies) > 0 || len(r.Metadata.RuntimeDependencies) > 0 {
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
		// Carry the recipe's metadata-level runtime_dependencies through to
		// the execution context so the homebrew action's relocate phase
		// (RPATH chain) and wrapper-script PATH consumer can read the
		// author-declared list verbatim. The executor populates this from
		// the recipe metadata if we leave it empty here, but setting it at
		// the call site keeps the wiring explicit in the install flow.
		if len(r.Metadata.RuntimeDependencies) > 0 {
			resolvedDeps.RuntimeDependencies = append([]string{}, r.Metadata.RuntimeDependencies...)
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
	// A hidden entry is deliberately not short-circuited on. Its payload is on
	// disk but it has no symlinks, so reporting "already installed" would leave
	// the user without the command they just asked for. CheckAndExposeHidden
	// above already took the cheap path when the entry knew its own binaries;
	// reaching here means it did not, and a real install is the fix.
	//
	// --reinstall is the other way past this. It is what makes an existing
	// install able to pick up a fix: the plan runs again and the manager
	// replaces the tool directory, so files written by older code are rewritten
	// by the current code. Nothing else reaches that path -- `--force` only
	// suppresses security prompts, and `--fresh` only regenerates the plan.
	if mgr.IsVersionInstalled(toolName, planVersion) && !isHiddenTool(mgr, toolName) && !reinstall {
		reporter.Status(fmt.Sprintf("%s@%s is already installed", toolName, planVersion))
		if err := recordInstallRelationship(mgr, toolName, parent, isExplicit); err != nil {
			reporter.Warn("failed to update state: %v", err)
		}
		setInstalledInIndex(toolName, true)
		return nil
	}

	// Disclose a recipe that pins no upstream checksum, for explicit installs
	// only. This sits after the already-installed short-circuit on purpose:
	// the note describes an integrity property of the download that is about
	// to happen, and on the no-op path there is no download -- the cached plan
	// is reused and nothing is hashed. Printed earlier it was a permanent line
	// narrating work that never ran, while the actual outcome ("already
	// installed") was a transient spinner message the terminal then cleared.
	if isExplicit && !quietFlag {
		switch r.GetChecksumVerification() {
		case recipe.ChecksumDynamic:
			// No upstream checksum to compare against: the plan pins whatever
			// the server served at generation time, which catches later
			// corruption but not a substitution made before we first looked.
			reporter.Log("Note: '%s' publishes no checksums; integrity is pinned to the artifact fetched now.", toolName)

		case recipe.ChecksumEcosystem, recipe.ChecksumStatic:
			// Ecosystem verification or an upstream-declared checksum -- silent.
		}
	}

	// Emit the install-start line now that the version is resolved from the plan.
	reporter.Status(fmt.Sprintf("Installing %s@%s", toolName, planVersion))

	// Execute the plan
	if err := exec.ExecutePlan(globalCtx, plan); err != nil {
		reporter.Log("❌ %s@%s", toolName, planVersion)
		recordDependencyInstalls(cfg, mgr, toolName, exec.GetDependencyInstalls(), reporter.Warn)
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
		// Snapshot what the version being replaced wrote outside its own tool
		// directory, before InstallWithOptions resets its VersionState. Only a
		// reinstall can reach this with a prior record under the same version
		// key -- every other path either short-circuits or writes a new key --
		// and without the snapshot a fragment the recipe has since stopped
		// writing would be orphaned: gone from state, still on disk, still
		// concatenated into the user's shell by the init cache.
		var priorCleanup []install.CleanupAction
		if reinstall {
			priorCleanup = recordedCleanupFor(mgr, toolName, version)
		}

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

		if err := mgr.InstallWithOptions(ctx, toolName, version, exec.WorkDir(), installOpts); err != nil {
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

		// Record the cleanup actions post-install produced and refresh the
		// shell caches they touched.
		finishPostInstall(cfg, mgr, toolName, version, exec.GetCleanupActions(), reporter.Warn)

		// Delete what the replaced install wrote outside the tool directory and
		// this one no longer writes. Recording comes first, so ExecuteStaleCleanup
		// reads the new record when it checks whether some other installed
		// version still claims the path.
		if reinstall {
			newCleanup := convertCleanupActions(exec.GetCleanupActions())
			mgr.ExecuteStaleCleanup(install.StaleCleanupActions(priorCleanup, newCleanup))
		}

		// Update state with explicit flag, parent, and dependencies
		// via semantic Manager methods.
		if err := recordInstallRelationship(mgr, toolName, parent, isExplicit); err != nil {
			reporter.Warn("failed to update state: %v", err)
		}
		if err := mgr.SetInstallDependencies(toolName, mapKeys(resolvedDeps.InstallTime)); err != nil {
			reporter.Warn("failed to record install dependencies: %v", err)
		}
		if err := mgr.SetRuntimeDependencies(toolName, mapKeys(resolvedDeps.Runtime)); err != nil {
			reporter.Warn("failed to record runtime dependencies: %v", err)
		}
	}

	// Record the dependencies the executor installed itself. Outside the
	// system-dependency branch on purpose: whether the parent turned out to be
	// a require_system stub says nothing about the dependencies that were
	// installed on the way to finding out.
	recordDependencyInstalls(cfg, mgr, toolName, exec.GetDependencyInstalls(), reporter.Warn)

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
		if isExplicit && parent == "" && !isToolPathConfigured(cfg) {
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

// recordInstallRelationship records the explicit-install and required-by
// relationship for a tool via Manager's semantic methods. When isExplicit
// is true, MarkExplicit sets IsExplicit and, if parent is non-empty,
// appends parent to RequiredBy. When isExplicit is false but parent is
// non-empty, only the required-by edge is recorded via AddRequiredBy.
// Both branches are no-ops when there is nothing to record.
// isHiddenTool reports whether the tool is recorded as somebody's hidden
// dependency rather than something the user installed.
func isHiddenTool(mgr *install.Manager, toolName string) bool {
	ts, err := mgr.GetToolState(toolName)
	return err == nil && ts != nil && ts.IsHidden
}

// recordedCleanupFor returns the cleanup actions state currently records for
// one version of a tool -- the files that version wrote outside its own install
// directory. Returns nil when the tool or the version has no entry, which is
// the ordinary case for a first install.
func recordedCleanupFor(mgr *install.Manager, toolName, version string) []install.CleanupAction {
	ts, err := mgr.GetToolState(toolName)
	if err != nil || ts == nil {
		return nil
	}
	vs, ok := ts.Versions[version]
	if !ok {
		return nil
	}
	return vs.CleanupActions
}

func recordInstallRelationship(mgr *install.Manager, toolName, parent string, isExplicit bool) error {
	if isExplicit {
		return mgr.MarkExplicit(toolName, parent)
	}
	if parent != "" {
		return mgr.GetState().AddRequiredBy(toolName, parent)
	}
	return nil
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

// clearAndRecordInstallSuccess is a no-op kept for source compatibility
// with existing callers in install.go. With the install lifecycle event
// bus in place, Manager.Install publishes Installed/Updated on success
// and the notices subscriber writes a fresh notice that atomically
// overwrites any prior failure record — no separate clear step needed.
func clearAndRecordInstallSuccess(toolName string) {
	_ = toolName
}
