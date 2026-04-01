package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var updateDryRun bool

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

		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		// Check if installed
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		var previousVersion string
		installed := false
		for _, tool := range tools {
			if tool.Name == toolName {
				installed = true
				previousVersion = tool.Version
				break
			}
		}

		if !installed {
			fmt.Fprintf(os.Stderr, "Error: %s is not installed. Use 'tsuku install %s' to install it.\n", toolName, toolName)
			exitWithCode(ExitGeneral)
		}

		// For distributed sources, use GetFromSource to fetch the recipe
		// directly from the recorded provider. This avoids recipe shadowing
		// where a local or central recipe with the same name would take
		// priority in the chain. The recipe is cached in the loader so
		// runInstallWithTelemetry can find it.
		state, _ := mgr.GetState().Load()
		if r, err := loadRecipeForTool(context.Background(), toolName, state, cfg); err == nil && r != nil {
			loader.CacheRecipe(toolName, r)
		}

		if updateDryRun {
			printInfof("Checking updates for %s...\n", toolName)
			if err := runDryRun(toolName, ""); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

		// Read the Requested field to respect install-time version constraint.
		// This ensures "tsuku update node" after "tsuku install node@18" stays
		// within 18.x.y instead of jumping to the absolute latest.
		var reqVersion string
		if state != nil {
			if ts, ok := state.Installed[toolName]; ok {
				if vs, ok := ts.Versions[ts.ActiveVersion]; ok {
					reqVersion = vs.Requested
				}
			}
		}

		// Snapshot old version's cleanup actions before installing new version.
		// These are needed to compute stale cleanup (files the old version
		// created that the new version no longer needs).
		var oldCleanupActions []install.CleanupAction
		if state != nil {
			if ts, ok := state.Installed[toolName]; ok {
				if vs, ok := ts.Versions[previousVersion]; ok {
					oldCleanupActions = vs.CleanupActions
				}
			}
		}

		printInfof("Updating %s...\n", toolName)
		if err := runInstallWithTelemetry(toolName, reqVersion, "", true, "", telemetryClient); err != nil {
			exitWithCode(ExitInstallFailed)
		}

		// Get the new version after update
		tools, _ = mgr.List()
		var newVersion string
		for _, tool := range tools {
			if tool.Name == toolName {
				newVersion = tool.Version
				break
			}
		}

		// Lifecycle-aware stale cleanup: delete files the old version created
		// that the new version no longer needs (e.g., shell.d scripts for a
		// shell the new version dropped). Only runs when the version changed
		// and the old version had cleanup actions recorded.
		if newVersion != "" && newVersion != previousVersion && len(oldCleanupActions) > 0 {
			// Reload state to get the new version's cleanup actions
			newState, _ := mgr.GetState().Load()
			if newState != nil {
				if ts, ok := newState.Installed[toolName]; ok {
					if vs, ok := ts.Versions[newVersion]; ok {
						stale := install.StaleCleanupActions(oldCleanupActions, vs.CleanupActions)
						mgr.ExecuteStaleCleanup(stale)
					}
				}
			}
		}

		// Send telemetry event for update
		if telemetryClient != nil && newVersion != "" {
			event := telemetry.NewUpdateEvent(toolName, previousVersion, newVersion)
			telemetryClient.Send(event)
		}
	},
}

// isDistributedSource returns true if the source string is a distributed
// "owner/repo" source (as opposed to "central", "local", or "embedded").
func isDistributedSource(source string) bool {
	return strings.Contains(source, "/")
}

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without making changes")
}
