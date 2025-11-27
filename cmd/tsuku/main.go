package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsuku-dev/tsuku/bundled"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/executor"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/version"
	"github.com/spf13/cobra"
)

var (
	// Version is the current version of tsuku
	Version = "0.3.0"

	// loader holds the recipe loader
	loader *recipe.Loader
)

var rootCmd = &cobra.Command{
	Use:   "tsuku",
	Short: "A modern, universal package manager for development tools",
	Long: `tsuku is a package manager that makes it easy to install and manage
development tools across different platforms.

It uses action-based recipes to download, extract, and install tools
to version-specific directories, with automatic PATH management.`,
	Version: Version,
}

var installCmd = &cobra.Command{
	Use:   "install <tool>...",
	Short: "Install a development tool",
	Long: `Install a development tool from the bundled recipe collection.
You can specify a version using the @ syntax.

Examples:
  tsuku install kubectl
  tsuku install kubectl@v1.29.0
  tsuku install terraform@latest`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, arg := range args {
			toolName := arg
			version := ""

			if strings.Contains(arg, "@") {
				parts := strings.SplitN(arg, "@", 2)
				toolName = parts[0]
				version = parts[1]

				if version == "latest" {
					version = ""
				}
			}

			if err := runInstall(toolName, version, true, ""); err != nil {
				// Continue installing other tools even if one fails?
				// For now, exit on first failure to be safe
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed tools",
	Long:  `List all tools currently installed by tsuku.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)

		// Check if --show-system-dependencies flag is set
		showSystemDeps, _ := cmd.Flags().GetBool("show-system-dependencies")

		var tools []install.InstalledTool
		if showSystemDeps {
			tools, err = mgr.ListAll()
		} else {
			tools, err = mgr.List()
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			os.Exit(1)
		}

		if len(tools) == 0 {
			fmt.Println("No tools installed.")
			return
		}

		if showSystemDeps {
			fmt.Printf("Installed tools (%d total, including system dependencies):\n\n", len(tools))
		} else {
			fmt.Printf("Installed tools (%d total):\n\n", len(tools))
		}

		// Load state to show system dependency indicator
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
			os.Exit(1)
		}

		for _, tool := range tools {
			prefix := "  "
			if toolState, exists := state.Installed[tool.Name]; exists && toolState.IsExecutionDependency {
				prefix = "* "
			}
			fmt.Printf("%s%-20s  %s\n", prefix, tool.Name, tool.Version)
		}

		if showSystemDeps {
			fmt.Println("\n* System dependency (installed by tsuku for internal use)")
		}
	},
}

func init() {
	listCmd.Flags().Bool("show-system-dependencies", false, "Include hidden system dependencies in output")
}

var updateCmd = &cobra.Command{
	Use:   "update <tool>",
	Short: "Update a tool to the latest version",
	Long: `Update an installed tool to its latest version.

Examples:
  tsuku update kubectl
  tsuku update terraform`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		// Check if installed
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			os.Exit(1)
		}

		installed := false
		for _, tool := range tools {
			if tool.Name == toolName {
				installed = true
				break
			}
		}

		if !installed {
			fmt.Fprintf(os.Stderr, "Error: %s is not installed. Use 'tsuku install %s' to install it.\n", toolName, toolName)
			os.Exit(1)
		}

		fmt.Printf("Updating %s...\n", toolName)
		if err := runInstall(toolName, "", true, ""); err != nil {
			os.Exit(1)
		}
	},
}

var versionsCmd = &cobra.Command{
	Use:   "versions <tool>",
	Short: "List available versions for a tool",
	Long:  `List all available versions (tags) for a tool.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Create version provider using factory
		res := version.New()
		factory := version.NewProviderFactory()
		provider, err := factory.ProviderFromRecipe(res, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Check if provider can list versions (Interface Segregation Principle)
		lister, canList := provider.(version.VersionLister)
		if !canList {
			fmt.Fprintf(os.Stderr, "Version listing not supported for %s (%s)\n",
				toolName, provider.SourceDescription())
			fmt.Fprintln(os.Stderr, "This source can resolve specific versions but cannot enumerate all versions.")
			os.Exit(1)
		}

		// List versions
		ctx := context.Background()
		fmt.Printf("Fetching versions for %s (%s)...\n", toolName, provider.SourceDescription())

		versions, err := lister.ListVersions(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list versions: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Available versions (%d total):\n\n", len(versions))
		for _, v := range versions {
			fmt.Printf("  %s\n", v)
		}
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for tools",
	Long:  `Search for tools in the bundled recipes by name or description.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := ""
		if len(args) > 0 {
			query = strings.ToLower(args[0])
		}

		// Get all recipes
		names := loader.List()

		// Filter and collect results
		type result struct {
			Name        string
			Description string
			Installed   string
		}
		var results []result

		// Initialize install manager to check status
		cfg, err := config.DefaultConfig()
		if err != nil {
			// If config fails, just assume nothing is installed
			// This shouldn't really happen in practice
			fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		}
		var installedTools []install.InstalledTool
		if cfg != nil {
			mgr := install.New(cfg)
			installedTools, _ = mgr.List() // Ignore error, just treat as empty
		}

		for _, name := range names {
			r, err := loader.Get(name)
			if err != nil {
				continue
			}

			// Check match
			match := query == "" ||
				strings.Contains(strings.ToLower(r.Metadata.Name), query) ||
				strings.Contains(strings.ToLower(r.Metadata.Description), query)

			if match {
				// Check installed status
				installedVer := "-"
				for _, t := range installedTools {
					if t.Name == name {
						installedVer = t.Version
						break
					}
				}

				results = append(results, result{
					Name:        r.Metadata.Name,
					Description: r.Metadata.Description,
					Installed:   installedVer,
				})
			}
		}

		if len(results) == 0 {
			fmt.Printf("No bundled recipes found for '%s'.\n\n", query)
			fmt.Println("💡 Tip: You can still try installing it!")
			fmt.Printf("   Run: tsuku install %s\n", query)
			fmt.Println("   (Tsuku will attempt to find and install it using AI)")
			return
		}

		// Print table
		// Calculate column widths
		maxName := 4  // "NAME"
		maxDesc := 11 // "DESCRIPTION"
		for _, r := range results {
			if len(r.Name) > maxName {
				maxName = len(r.Name)
			}
			if len(r.Description) > maxDesc {
				maxDesc = len(r.Description)
			}
		}
		// Cap description width to avoid wrapping mess
		if maxDesc > 60 {
			maxDesc = 60
		}

		fmt.Printf("%-*s  %-*s  %s\n", maxName, "NAME", maxDesc, "DESCRIPTION", "INSTALLED")
		for _, r := range results {
			desc := r.Description
			if len(desc) > maxDesc {
				desc = desc[:maxDesc-3] + "..."
			}
			fmt.Printf("%-*s  %-*s  %s\n", maxName, r.Name, maxDesc, desc, r.Installed)
		}
	},
}

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Show detailed information about a tool",
	Long:  `Show detailed information about a tool, including description, homepage, and installation status.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Printf("Tool '%s' not found in bundled recipes.\n", toolName)
			return
		}

		fmt.Printf("Name:           %s\n", r.Metadata.Name)
		fmt.Printf("Description:    %s\n", r.Metadata.Description)
		if r.Metadata.Homepage != "" {
			fmt.Printf("Homepage:       %s\n", r.Metadata.Homepage)
		}
		fmt.Printf("Version Format: %s\n", r.Metadata.VersionFormat)

		// Check installation status
		cfg, err := config.DefaultConfig()
		if err == nil {
			mgr := install.New(cfg)
			tools, _ := mgr.List()

			installed := false
			for _, t := range tools {
				if t.Name == toolName {
					fmt.Printf("Status:         Installed (v%s)\n", t.Version)
					fmt.Printf("Location:       %s\n", cfg.ToolDir(toolName, t.Version))
					installed = true
					break
				}
			}
			if !installed {
				fmt.Printf("Status:         Not installed\n")
			}
		}

		// Show verification method
		if r.Verify.Command != "" {
			fmt.Printf("Verify Command: %s\n", r.Verify.Command)
		}
	},
}

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Check for outdated tools",
	Long:  `Check for newer versions of installed tools.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing tools: %v\n", err)
			os.Exit(1)
		}

		if len(tools) == 0 {
			fmt.Println("No tools installed.")
			return
		}

		fmt.Println("Checking for updates...")
		res := version.New()
		ctx := context.Background()

		type updateInfo struct {
			Name    string
			Current string
			Latest  string
			Repo    string
		}
		var updates []updateInfo

		for _, tool := range tools {
			// Load recipe to find repo
			r, err := loader.Get(tool.Name)
			if err != nil {
				continue
			}

			// Find repo
			var repo string
			for _, step := range r.Steps {
				if step.Action == "github_archive" || step.Action == "github_file" {
					if r, ok := step.Params["repo"].(string); ok {
						repo = r
						break
					}
				}
			}

			if repo == "" {
				continue
			}

			// Check latest version
			fmt.Printf("Checking %s...\n", tool.Name)
			latest, err := res.ResolveGitHub(ctx, repo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to check %s: %v\n", tool.Name, err)
				continue
			}

			if latest.Version != tool.Version {
				// Simple string comparison for now.
				// Ideally should use semver, but this is a good start.
				// We assume if strings differ, and we just fetched latest, it's likely an update.
				// But to be safe, we only show if they are different.
				updates = append(updates, updateInfo{
					Name:    tool.Name,
					Current: tool.Version,
					Latest:  latest.Version,
					Repo:    repo,
				})
			}
		}

		fmt.Println()
		if len(updates) == 0 {
			fmt.Println("All tools are up to date! 🎉")
			return
		}

		fmt.Printf("%-15s  %-15s  %-15s\n", "TOOL", "CURRENT", "LATEST")
		for _, u := range updates {
			fmt.Printf("%-15s  %-15s  %-15s\n", u.Name, u.Current, u.Latest)
		}
		fmt.Println("\nTo update, run: tsuku update <tool>")
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove <tool>",
	Short: "Remove an installed tool",
	Long: `Remove a tool that was installed by tsuku.

Examples:
  tsuku remove kubectl
  tsuku remove terraform`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)
		// Check if tool is required by others
		state, err := mgr.GetState().Load()
		if err == nil {
			if ts, ok := state.Installed[toolName]; ok {
				if len(ts.RequiredBy) > 0 {
					fmt.Fprintf(os.Stderr, "Error: %s is required by: %s\n", toolName, strings.Join(ts.RequiredBy, ", "))
					fmt.Fprintf(os.Stderr, "Please remove them first.\n")
					os.Exit(1)
				}
			}
		}

		if err := mgr.Remove(toolName); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", toolName, err)
			os.Exit(1)
		}

		// Remove this tool from dependencies' RequiredBy list
		if state != nil {
			if err := mgr.GetState().RemoveTool(toolName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove tool from state: %v\n", err)
			}

			// We need to find which tools this tool depended on to clean up references
			// But we just removed it, so we might have lost that info if we didn't load the recipe.
			// Ideally, we should load the recipe before removing.
			// For now, let's try to load the recipe.
			if r, err := loader.Get(toolName); err == nil {
				for _, dep := range r.Metadata.Dependencies {
					if err := mgr.GetState().RemoveRequiredBy(dep, toolName); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to update dependency state for %s: %v\n", dep, err)
					}
					// Try to cleanup orphan
					cleanupOrphans(mgr, dep)
				}
			}
		}

		fmt.Printf("✅ Removed %s\n", toolName)
	},
}

func cleanupOrphans(mgr *install.Manager, toolName string) {
	state, err := mgr.GetState().Load()
	if err != nil {
		return
	}

	ts, ok := state.Installed[toolName]
	if !ok {
		return
	}

	// If explicit, don't remove
	if ts.IsExplicit {
		return
	}

	// If still required by others, don't remove
	if len(ts.RequiredBy) > 0 {
		return
	}

	fmt.Printf("Auto-removing orphaned dependency: %s\n", toolName)
	if err := mgr.Remove(toolName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-remove %s: %v\n", toolName, err)
		return
	}

	// Remove from state
	if err := mgr.GetState().RemoveTool(toolName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove tool from state: %v\n", err)
	}

	// Recursively clean up its dependencies
	if r, err := loader.Get(toolName); err == nil {
		for _, dep := range r.Metadata.Dependencies {
			if err := mgr.GetState().RemoveRequiredBy(dep, toolName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to update dependency state for %s: %v\n", dep, err)
			}
			cleanupOrphans(mgr, dep)
		}
	}
}

func runInstall(toolName, reqVersion string, isExplicit bool, parent string) error {
	return installWithDependencies(toolName, reqVersion, isExplicit, parent, make(map[string]bool))
}

// ensurePackageManagersForRecipe checks if a recipe uses package managers
// and auto-bootstraps them as hidden execution dependencies if needed
// It also injects the package manager paths into the step params
// Returns a list of bin paths that should be added to PATH for execution
func ensurePackageManagersForRecipe(mgr *install.Manager, r *recipe.Recipe) ([]string, error) {
	var execPaths []string
	for i := range r.Steps {
		step := &r.Steps[i]
		switch step.Action {
		case "npm_install":
			npmPath, err := install.EnsureNpm(mgr)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure npm: %w", err)
			}
			// Inject npm path into step params
			step.Params["npm_path"] = npmPath
			// Add nodejs bin directory to exec paths
			execPaths = append(execPaths, filepath.Dir(npmPath))
		case "pip_install":
			pythonPath, err := install.EnsurePython(mgr)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure python: %w", err)
			}
			// Inject python path into step params
			step.Params["python_path"] = pythonPath
			// Add python bin directory to exec paths
			execPaths = append(execPaths, filepath.Dir(pythonPath))
		case "cargo_install":
			cargoPath, err := install.EnsureCargo(mgr)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure cargo: %w", err)
			}
			// Inject cargo path into step params
			step.Params["cargo_path"] = cargoPath
			// Add cargo bin directory to exec paths
			execPaths = append(execPaths, filepath.Dir(cargoPath))
		case "pipx_install":
			pipxPath, err := install.EnsurePipx(mgr)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure pipx: %w", err)
			}
			// Inject pipx path into step params
			step.Params["pipx_path"] = pipxPath
			// Add pipx bin directory to exec paths
			execPaths = append(execPaths, filepath.Dir(pipxPath))
		}
	}
	return execPaths, nil
}

func installWithDependencies(toolName, reqVersion string, isExplicit bool, parent string, visited map[string]bool) error {
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
			fmt.Printf("Warning: failed to check hidden status: %v\n", err)
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
			fmt.Printf("Warning: failed to update state for %s: %v\n", toolName, err)
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'tsuku recipes' to see available recipes\n")
		return err
	}

	// Check and install dependencies
	if len(r.Metadata.Dependencies) > 0 {
		fmt.Printf("Checking dependencies for %s...\n", toolName)

		for _, dep := range r.Metadata.Dependencies {
			fmt.Printf("  Resolving dependency '%s'...\n", dep)
			// Install dependency (not explicit, parent is current tool)
			if err := installWithDependencies(dep, "", false, toolName, visited); err != nil {
				return fmt.Errorf("failed to install dependency '%s': %w", dep, err)
			}
		}
	}

	// Auto-bootstrap package managers if recipe uses them
	// This must happen BEFORE checking runtime dependencies so that if a package manager
	// (like npm/nodejs) is also a runtime dependency, we can expose it
	execPaths, err := ensurePackageManagersForRecipe(mgr, r)
	if err != nil {
		return fmt.Errorf("failed to ensure package managers: %w", err)
	}

	// Check and install runtime dependencies (these must be exposed, not hidden)
	// This happens AFTER package manager bootstrap so CheckAndExposeHidden can work
	if len(r.Metadata.RuntimeDependencies) > 0 {
		fmt.Printf("Checking runtime dependencies for %s...\n", toolName)

		for _, dep := range r.Metadata.RuntimeDependencies {
			fmt.Printf("  Resolving runtime dependency '%s'...\n", dep)
			// Install runtime dependency as explicit (exposed, not hidden)
			// No parent - these are top-level explicit installs
			if err := installWithDependencies(dep, "", true, "", visited); err != nil {
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

	// Execute recipe
	ctx := context.Background()
	if err := exec.Execute(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		return err
	}

	// Check if version was resolved (structure-only validation doesn't resolve versions)
	version := exec.Version()
	if version == "" {
		// For recipes without dynamic versioning (e.g. local test recipes), use "dev"
		version = "dev"
	}

	// Install to permanent location
	// cfg is already loaded
	// mgr is already initialized

	// Extract binaries from recipe to store in state
	binaries := r.ExtractBinaries()
	installOpts := install.DefaultInstallOptions()
	installOpts.Binaries = binaries

	if err := mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to install to permanent location: %v\n", err)
		return err
	}

	// Update state with explicit flag and parent
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
	})
	if err != nil {
		fmt.Printf("Warning: failed to update state: %v\n", err)
	}

	fmt.Println()
	fmt.Println("✅ Installation successful!")
	fmt.Println()
	fmt.Println("To use the installed tool, add this to your shell profile:")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)

	return nil
}

var recipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "List available recipes",
	Long:  `List all available recipes in the bundled collection.`,
	Run: func(cmd *cobra.Command, args []string) {
		names := loader.List()
		sort.Strings(names)

		fmt.Printf("Available recipes (%d total):\n\n", loader.Count())

		for _, name := range names {
			r, _ := loader.Get(name)
			fmt.Printf("  %-20s  %s\n", name, r.Metadata.Description)
		}
	},
}

// verifyWithAbsolutePath verifies a hidden tool using absolute paths
func verifyWithAbsolutePath(r *recipe.Recipe, toolName, version, installDir string) {
	command := r.Verify.Command
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)

	pattern := r.Verify.Pattern
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	fmt.Printf("  Running: %s\n", command)

	cmdExec := exec.Command("sh", "-c", command)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Verification failed: %v\nOutput: %s\n", err, string(output))
		os.Exit(1)
	}

	outputStr := strings.TrimSpace(string(output))
	fmt.Printf("  Output: %s\n", outputStr)

	if pattern != "" {
		if !strings.Contains(outputStr, pattern) {
			fmt.Fprintf(os.Stderr, "✗ Output does not match expected pattern\n  Expected: %s\n  Got: %s\n", pattern, outputStr)
			os.Exit(1)
		}
		fmt.Printf("  ✓ Pattern matched: %s\n", pattern)
	}
}

// verifyVisibleTool performs comprehensive verification for visible tools
func verifyVisibleTool(r *recipe.Recipe, toolName string, toolState *install.ToolState, installDir string, cfg *config.Config) {
	// Step 1: Verify installation via current/ symlink
	fmt.Printf("  Step 1: Verifying installation via symlink...\n")

	command := r.Verify.Command
	pattern := r.Verify.Pattern

	// For visible tools, use the binary name directly (will resolve via current/)
	// But first verify the symlink works by using absolute path
	version := toolState.Version
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	fmt.Printf("    Running: %s\n", command)
	cmdExec := exec.Command("sh", "-c", command)

	// For Step 1, add install directory bin/ to PATH so binaries can be found
	// This is needed for binary-only installs where verify command doesn't use {install_dir}
	env := os.Environ()
	binDir := filepath.Join(installDir, "bin")
	env = append(env, "PATH="+binDir+":"+os.Getenv("PATH"))
	cmdExec.Env = env

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "    ✗ Installation verification failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "    Output: %s\n", string(output))
		fmt.Fprintf(os.Stderr, "\nThe tool is installed but not working correctly.\n")
		os.Exit(1)
	}
	outputStr := strings.TrimSpace(string(output))
	fmt.Printf("    Output: %s\n", outputStr)

	if pattern != "" && !strings.Contains(outputStr, pattern) {
		fmt.Fprintf(os.Stderr, "    ✗ Pattern mismatch\n")
		fmt.Fprintf(os.Stderr, "    Expected: %s\n", pattern)
		fmt.Fprintf(os.Stderr, "    Got: %s\n", outputStr)
		os.Exit(1)
	}
	fmt.Printf("    ✓ Installation verified\n\n")

	// Step 2: Check if current/ is in PATH
	fmt.Printf("  Step 2: Checking if %s is in PATH...\n", cfg.CurrentDir)
	pathEnv := os.Getenv("PATH")
	pathInPATH := false
	for _, dir := range strings.Split(pathEnv, ":") {
		if dir == cfg.CurrentDir {
			pathInPATH = true
			break
		}
	}

	if !pathInPATH {
		fmt.Fprintf(os.Stderr, "    ✗ %s is not in your PATH\n", cfg.CurrentDir)
		fmt.Fprintf(os.Stderr, "\nThe tool is installed correctly, but you need to add this to your shell profile:\n")
		fmt.Fprintf(os.Stderr, "  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)
		os.Exit(1)
	}
	fmt.Printf("    ✓ %s is in PATH\n\n", cfg.CurrentDir)

	// Step 3: Verify tool binaries are accessible from PATH and check for conflicts
	fmt.Printf("  Step 3: Checking PATH resolution for binaries...\n")

	// Check each binary provided by this tool
	binariesToCheck := toolState.Binaries
	if len(binariesToCheck) == 0 {
		// Fallback: assume tool name is the binary
		binariesToCheck = []string{toolName}
	}

	for _, binaryPath := range binariesToCheck {
		// Extract just the binary name (e.g., "cargo/bin/cargo" -> "cargo")
		binaryName := filepath.Base(binaryPath)

		whichPath, err := exec.Command("which", binaryName).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "    ✗ Binary '%s' not found in PATH\n", binaryName)
			fmt.Fprintf(os.Stderr, "\nThe tool is installed and current/ is in PATH, but '%s' cannot be found.\n", binaryName)
			fmt.Fprintf(os.Stderr, "This may indicate a broken symlink in %s\n", cfg.CurrentDir)
			os.Exit(1)
		}

		resolvedPath := strings.TrimSpace(string(whichPath))
		expectedPath := cfg.CurrentSymlink(binaryName)

		fmt.Printf("    Binary '%s':\n", binaryName)
		fmt.Printf("      Found: %s\n", resolvedPath)
		fmt.Printf("      Expected: %s\n", expectedPath)

		if resolvedPath != expectedPath {
			fmt.Fprintf(os.Stderr, "      ✗ PATH conflict detected!\n")
			fmt.Fprintf(os.Stderr, "\nThe tool is installed, but another '%s' is earlier in your PATH:\n", binaryName)
			fmt.Fprintf(os.Stderr, "  Using: %s\n", resolvedPath)
			fmt.Fprintf(os.Stderr, "  Expected: %s\n", expectedPath)

			// Try to get version info from the conflicting tool
			versionCmd := exec.Command(binaryName, "--version")
			if versionOut, err := versionCmd.CombinedOutput(); err == nil {
				fmt.Fprintf(os.Stderr, "  Conflicting version output: %s\n", strings.TrimSpace(string(versionOut)))
			}
			os.Exit(1)
		}
		fmt.Printf("      ✓ Correct binary is being used from PATH\n")
	}
}

var verifyCmd = &cobra.Command{
	Use:   "verify <tool>",
	Short: "Verify an installed tool",
	Long:  `Verify that an installed tool is working correctly using the recipe's verification command.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		// Get config and manager
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)

		// Check if tool is installed
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
			os.Exit(1)
		}

		toolState, ok := state.Installed[toolName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Tool '%s' is not installed\n", toolName)
			os.Exit(1)
		}

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load recipe: %v\n", err)
			os.Exit(1)
		}

		// Check if recipe has verification
		if r.Verify.Command == "" {
			fmt.Fprintf(os.Stderr, "Recipe for '%s' does not define verification\n", toolName)
			os.Exit(1)
		}

		installDir := filepath.Join(cfg.ToolsDir, fmt.Sprintf("%s-%s", toolName, toolState.Version))
		fmt.Printf("Verifying %s (version %s)...\n", toolName, toolState.Version)

		// Determine verification strategy based on tool visibility
		if toolState.IsHidden {
			// Hidden tools: verify with absolute path
			fmt.Printf("  Tool is hidden (not in PATH)\n")
			verifyWithAbsolutePath(r, toolName, toolState.Version, installDir)
		} else {
			// Visible tools: comprehensive verification
			verifyVisibleTool(r, toolName, &toolState, installDir, cfg)
		}

		fmt.Printf("✓ %s is working correctly\n", toolName)
	},
}

func init() {
	// Initialize recipe loader
	var err error
	loader, err = recipe.NewLoader(bundled.Recipes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize recipe loader: %v\n", err)
		os.Exit(1)
	}

	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(recipesCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(outdatedCmd)
	rootCmd.AddCommand(verifyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
