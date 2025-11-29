package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/telemetry"
)

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

		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)

		// Get version before removal for telemetry
		var previousVersion string
		tools, _ := mgr.List()
		for _, tool := range tools {
			if tool.Name == toolName {
				previousVersion = tool.Version
				break
			}
		}

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

		// Send telemetry event for successful removal
		if telemetryClient != nil && previousVersion != "" {
			event := telemetry.NewRemoveEvent(toolName, previousVersion)
			telemetryClient.Send(event)
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

		fmt.Printf("Removed %s\n", toolName)
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
