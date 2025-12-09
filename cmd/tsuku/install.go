package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var installDryRun bool
var installForce bool

var installCmd = &cobra.Command{
	Use:   "install <tool>...",
	Short: "Install a development tool",
	Long: `Install a development tool from the recipe registry.
You can specify a version using the @ syntax.

Examples:
  tsuku install kubectl
  tsuku install kubectl@v1.29.0
  tsuku install terraform@latest`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		for _, arg := range args {
			toolName := arg
			versionConstraint := ""

			if strings.Contains(arg, "@") {
				parts := strings.SplitN(arg, "@", 2)
				toolName = parts[0]
				versionConstraint = parts[1]
			}

			// Convert "latest" to empty for resolution, but keep original constraint for telemetry
			resolveVersion := versionConstraint
			if resolveVersion == "latest" {
				resolveVersion = ""
			}

			if installDryRun {
				if err := runDryRun(toolName, resolveVersion); err != nil {
					printError(err)
					exitWithCode(ExitInstallFailed)
				}
			} else {
				if err := runInstallWithTelemetry(toolName, resolveVersion, versionConstraint, true, "", telemetryClient); err != nil {
					// Continue installing other tools even if one fails?
					// For now, exit on first failure to be safe
					printError(err)
					exitWithCode(ExitInstallFailed)
				}
			}
		}
	},
}

func init() {
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Show what would be installed without making changes")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Skip security warnings and proceed without prompts")
}

// isInteractive returns true if stdin is connected to a terminal
func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// confirmInstall prompts the user for confirmation and returns true if they agree
func confirmInstall() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "Continue installation? [y/N] ")
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
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
	// Check for circular dependencies
	if visited[toolName] {
		return fmt.Errorf("circular dependency detected: %s", toolName)
	}
	visited[toolName] = true

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

	// Check if already installed
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
		// But for dependency check, we just return
		if !isExplicit && reqVersion == "" {
			return nil
		}
		// If it's an explicit install/update, we proceed
	}

	// Load recipe
	r, err := loader.Get(toolName)
	if err != nil {
		printError(err)
		fmt.Fprintf(os.Stderr, "\nTo create a recipe from a package ecosystem:\n")
		fmt.Fprintf(os.Stderr, "  tsuku create %s --from <ecosystem>\n", toolName)
		fmt.Fprintf(os.Stderr, "\nAvailable ecosystems: crates.io, rubygems, pypi, npm\n")
		return err
	}

	// Check if this is a library recipe
	if r.IsLibrary() {
		// Prevent direct installation of libraries
		if isExplicit && parent == "" {
			return fmt.Errorf("'%s' is a library and cannot be installed directly.\n"+
				"Libraries are installed automatically when you install a tool that depends on them.\n"+
				"For example, 'tsuku install ruby' will automatically install libyaml.", toolName)
		}
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

	// Execute recipe with cancellable context
	if err := exec.Execute(globalCtx); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		return err
	}

	// Check if version was resolved (structure-only validation doesn't resolve versions)
	version := exec.Version()
	if version == "" {
		// For recipes without dynamic versioning (e.g. local test recipes), use "dev"
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

	// Extract binaries from recipe to store in state
	binaries := r.ExtractBinaries()
	installOpts := install.DefaultInstallOptions()
	installOpts.Binaries = binaries
	installOpts.RequestedVersion = versionConstraint // Record what user asked for ("17", "@lts", "")

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
	printInfo("Installation successful!")
	printInfo()
	printInfo("To use the installed tool, add this to your shell profile:")
	printInfof("  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)

	return nil
}

// installLibrary handles installation of library recipes
// Libraries are installed to $TSUKU_HOME/libs/{name}-{version}/ and track used_by
// Note: used_by tracking is handled by the caller after tool installation completes
func installLibrary(libName, reqVersion, parent string, mgr *install.Manager, telemetryClient *telemetry.Client) error {
	// Load recipe
	r, err := loader.Get(libName)
	if err != nil {
		return fmt.Errorf("library recipe not found: %w", err)
	}

	// Check if we can skip installation (reuse existing version)
	// For now, just check if any version is installed
	existingVersion := mgr.GetInstalledLibraryVersion(libName)
	if existingVersion != "" && reqVersion == "" {
		printInfof("Library %s@%s already installed, reusing\n", libName, existingVersion)
		return nil
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		return fmt.Errorf("failed to create executor for library: %w", err)
	}
	defer exec.Cleanup()

	// Set tools directory for finding other installed tools
	cfg, _ := config.DefaultConfig()
	exec.SetToolsDir(cfg.ToolsDir)

	// Execute recipe to download and prepare library
	if err := exec.Execute(globalCtx); err != nil {
		return fmt.Errorf("library installation failed: %w", err)
	}

	version := exec.Version()
	if version == "" {
		version = "dev"
	}

	// Check if this specific version is already installed
	if mgr.IsLibraryInstalled(libName, version) {
		printInfof("Library %s@%s already installed\n", libName, version)
		return nil
	}

	// Install to libs directory
	// Note: used_by tracking is handled by the caller (installWithDependencies) after
	// tool installation completes, since we need the tool's version for proper tracking
	opts := install.LibraryInstallOptions{}

	if err := mgr.InstallLibrary(libName, version, exec.WorkDir(), opts); err != nil {
		return fmt.Errorf("failed to install library to permanent location: %w", err)
	}

	// Send telemetry event
	if telemetryClient != nil {
		event := telemetry.NewInstallEvent(libName, "", version, true) // Libraries are always dependencies
		telemetryClient.Send(event)
	}

	printInfof("Library %s@%s installed successfully\n", libName, version)
	return nil
}

// runDryRun shows what would be installed without making changes
func runDryRun(toolName, reqVersion string) error {
	// Load recipe
	r, err := loader.Get(toolName)
	if err != nil {
		return fmt.Errorf("recipe not found: %w", err)
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}
	defer exec.Cleanup()

	// Run dry-run with cancellable context
	return exec.DryRun(globalCtx)
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
