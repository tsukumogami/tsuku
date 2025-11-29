package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/executor"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/recipe"
)

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

	// Set tools directory for finding other installed tools
	exec.SetToolsDir(cfg.ToolsDir)

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
	fmt.Println("Installation successful!")
	fmt.Println()
	fmt.Println("To use the installed tool, add this to your shell profile:")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)

	return nil
}
