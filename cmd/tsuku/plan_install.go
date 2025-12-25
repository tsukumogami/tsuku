package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// runPlanBasedInstall installs a tool from an external plan file or stdin.
// It bypasses normal recipe evaluation and directly executes the provided plan.
func runPlanBasedInstall(planPath, toolName string) error {
	// Load plan from file or stdin
	plan, err := loadPlanFromSource(planPath)
	if err != nil {
		return err
	}

	// Validate plan (includes tool name check if provided)
	if err := validateExternalPlan(plan, toolName); err != nil {
		return err
	}

	// Use tool name from plan if not specified on command line
	effectiveToolName := toolName
	if effectiveToolName == "" {
		effectiveToolName = plan.Tool
	}

	// Initialize config and manager
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	mgr := install.New(cfg)

	// Create minimal recipe for executor context
	// The executor needs a recipe to set up paths, but the plan contains all actual steps
	minimalRecipe := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: effectiveToolName,
		},
	}

	// Create executor with minimal recipe and plan's version
	exec, err := executor.NewWithVersion(minimalRecipe, plan.Version)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}
	defer exec.Cleanup()

	// Set download cache directory for checksum verification
	exec.SetDownloadCacheDir(cfg.DownloadCacheDir)

	// Set tools directory for finding other installed tools
	exec.SetToolsDir(cfg.ToolsDir)

	printInfof("Installing %s@%s from plan...\n", effectiveToolName, plan.Version)

	// Execute the plan
	if err := exec.ExecutePlan(globalCtx, plan); err != nil {
		// Handle ChecksumMismatchError specially - it has a user-friendly message
		var checksumErr *executor.ChecksumMismatchError
		if errors.As(err, &checksumErr) {
			fmt.Fprintf(os.Stderr, "\n%s\n", checksumErr.Error())
			return err
		}
		return fmt.Errorf("plan execution failed: %w", err)
	}

	// Check if this is a system dependency recipe (only require_system steps)
	// System dependencies are validated but not tracked in state or installed
	isSystemDep := isSystemDependencyRecipe(plan)

	if !isSystemDep {
		// Prepare install options
		installOpts := install.DefaultInstallOptions()
		installOpts.Plan = executor.ToStoragePlan(plan)

		// Install to permanent location
		if err := mgr.InstallWithOptions(effectiveToolName, plan.Version, exec.WorkDir(), installOpts); err != nil {
			return fmt.Errorf("failed to install to permanent location: %w", err)
		}

		// Update state to mark as explicit installation
		err = mgr.GetState().UpdateTool(effectiveToolName, func(ts *install.ToolState) {
			ts.IsExplicit = true
		})
		if err != nil {
			printInfof("Warning: failed to update state: %v\n", err)
		}

		printInfo()
		printInfo("Installation successful!")
		printInfo()
		printInfof("To use the installed tool, add this to your shell profile:\n")
		printInfof("  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)
	} else {
		// System dependency validated successfully - no installation needed
		printInfo()
		printInfof("✓ %s is available on your system\n", effectiveToolName)
		printInfo()
		printInfo("Note: tsuku doesn't manage this dependency. It validated that it's installed.")
	}

	return nil
}

// isSystemDependencyRecipe returns true if the plan only contains require_system steps.
// System dependency recipes validate that external tools are installed but don't
// actually install anything, so they shouldn't create state entries or directories.
func isSystemDependencyRecipe(plan *executor.InstallationPlan) bool {
	if len(plan.Steps) == 0 {
		return false
	}
	for _, step := range plan.Steps {
		if step.Action != "require_system" {
			return false
		}
	}
	return true
}
