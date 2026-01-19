package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var removeCmd = &cobra.Command{
	Use:   "remove <tool>[@version]",
	Short: "Remove an installed tool",
	Long: `Remove a tool that was installed by tsuku.

Without a version, removes all installed versions of the tool.
With @version syntax, removes only the specified version.

Examples:
  tsuku remove kubectl           # Remove all versions
  tsuku remove kubectl@1.29.0    # Remove specific version
  tsuku remove terraform`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		arg := args[0]
		forceRemove, _ := cmd.Flags().GetBool("force")

		// Parse tool@version syntax
		toolName := arg
		targetVersion := ""
		if strings.Contains(arg, "@") {
			parts := strings.SplitN(arg, "@", 2)
			toolName = parts[0]
			targetVersion = parts[1]
		}

		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)

		// Check if tool is required by others (only when removing all versions)
		state, err := mgr.GetState().Load()
		if err == nil {
			if ts, ok := state.Installed[toolName]; ok {
				if len(ts.RequiredBy) > 0 && targetVersion == "" {
					// Only warn when removing all versions
					fmt.Fprintf(os.Stderr, "Warning: %s is required by: %s\n", toolName, strings.Join(ts.RequiredBy, ", "))
					if !forceRemove {
						fmt.Fprintf(os.Stderr, "Use --force to remove anyway.\n")
						exitWithCode(ExitDependencyFailed)
					}
					fmt.Fprintf(os.Stderr, "Proceeding with removal due to --force flag.\n")
				}
			}
		}

		// Get version for telemetry before removal
		var removedVersion string
		if targetVersion != "" {
			removedVersion = targetVersion
		} else if state != nil {
			if ts, ok := state.Installed[toolName]; ok {
				removedVersion = ts.ActiveVersion
			}
		}

		// Perform removal
		var removeErr error
		if targetVersion != "" {
			// Remove specific version
			removeErr = mgr.RemoveVersion(toolName, targetVersion)
		} else {
			// Remove all versions
			removeErr = mgr.RemoveAllVersions(toolName)
		}

		if removeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", toolName, removeErr)
			exitWithCode(ExitGeneral)
		}

		// Send telemetry event for successful removal
		if telemetryClient != nil && removedVersion != "" {
			event := telemetry.NewRemoveEvent(toolName, removedVersion)
			telemetryClient.Send(event)
		}

		// Clean up dependency references (only when removing all versions)
		if targetVersion == "" {
			// We need to find which tools this tool depended on to clean up references
			// Load the recipe to get dependencies
			if r, err := loader.Get(toolName, recipe.LoaderOptions{}); err == nil {
				for _, dep := range r.Metadata.Dependencies {
					if err := mgr.GetState().RemoveRequiredBy(dep, toolName); err != nil {
						printInfof("Warning: failed to update dependency state for %s: %v\n", dep, err)
					}
					// Try to cleanup orphan
					cleanupOrphans(mgr, dep)
				}
			}
		}

		if targetVersion != "" {
			printInfof("Removed %s@%s\n", toolName, targetVersion)
		} else {
			printInfof("Removed %s (all versions)\n", toolName)
		}
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

	printInfof("Auto-removing orphaned dependency: %s\n", toolName)
	if err := mgr.Remove(toolName); err != nil {
		printInfof("Warning: failed to auto-remove %s: %v\n", toolName, err)
		return
	}

	// Remove from state
	if err := mgr.GetState().RemoveTool(toolName); err != nil {
		printInfof("Warning: failed to remove tool from state: %v\n", err)
	}

	// Recursively clean up its dependencies
	if r, err := loader.Get(toolName, recipe.LoaderOptions{}); err == nil {
		for _, dep := range r.Metadata.Dependencies {
			if err := mgr.GetState().RemoveRequiredBy(dep, toolName); err != nil {
				printInfof("Warning: failed to update dependency state for %s: %v\n", dep, err)
			}
			cleanupOrphans(mgr, dep)
		}
	}
}

func init() {
	removeCmd.Flags().BoolP("force", "f", false, "Force removal even if other tools depend on this one")
}
