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

var (
	updateDryRun bool
	updateAll    bool
)

var updateCmd = &cobra.Command{
	Use:   "update [tool]",
	Short: "Update a tool to the latest version",
	Long: `Update an installed tool to its latest version within pin boundaries.

Use --all to update all installed tools at once.

Examples:
  tsuku update kubectl
  tsuku update terraform
  tsuku update --all
  tsuku update --all --dry-run`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if updateAll && len(args) > 0 {
			fmt.Fprintf(os.Stderr, "Error: --all and a tool name are mutually exclusive\n")
			exitWithCode(ExitUsage)
		}
		if !updateAll && len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: provide a tool name or use --all\n")
			cmd.Usage()
			exitWithCode(ExitUsage)
		}

		if updateAll {
			runUpdateAll(cmd)
			return
		}

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
			if telemetryClient != nil {
				telemetryClient.SendUpdateOutcome(telemetry.NewUpdateOutcomeFailureEvent(
					toolName, reqVersion, telemetry.ClassifyError(err), "manual"))
			}
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

						// Update diff visibility: warn when shell init content
						// changed between versions. This surfaces silent supply-chain
						// changes where an upstream binary alters its init output.
						warnShellInitChanges(toolName, oldCleanupActions, vs.CleanupActions)
					}
				}
			}
		}

		// Send telemetry event for update
		if telemetryClient != nil && newVersion != "" {
			event := telemetry.NewUpdateEvent(toolName, previousVersion, newVersion)
			telemetryClient.Send(event)
			// Also emit outcome event for reliability tracking
			telemetryClient.SendUpdateOutcome(telemetry.NewUpdateOutcomeSuccessEvent(
				toolName, previousVersion, newVersion, "manual"))
		}
	},
}

// warnShellInitChanges compares content hashes between old and new cleanup
// actions for shell.d paths. When a matching path has different hashes, it
// means the tool's shell init output changed during the update -- a signal
// worth surfacing to the user.
func warnShellInitChanges(toolName string, old, new []install.CleanupAction) {
	// Build a map of path -> content hash from old actions
	oldHashes := make(map[string]string)
	for _, ca := range old {
		if ca.ContentHash != "" {
			oldHashes[ca.Path] = ca.ContentHash
		}
	}

	for _, ca := range new {
		if ca.ContentHash == "" {
			continue
		}
		oldHash, exists := oldHashes[ca.Path]
		if !exists {
			// New path not in old -- new shell init file, not a change
			continue
		}
		if oldHash != ca.ContentHash {
			shell := install.ShellFromCleanupPath(ca.Path)
			if shell == "" {
				shell = ca.Path
			}
			fmt.Fprintf(os.Stderr, "Warning: shell init changed for %s (%s)\n", toolName, shell)
		}
	}
}

// isDistributedSource returns true if the source string is a distributed
// "owner/repo" source (as opposed to "central", "local", or "embedded").
func isDistributedSource(source string) bool {
	return strings.Contains(source, "/")
}

// runUpdateAll updates all installed tools within their pin boundaries.
func runUpdateAll(cmd *cobra.Command) {
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

	if len(tools) == 0 {
		printInfo("No tools installed.")
		return
	}

	state, _ := mgr.GetState().Load()
	telemetryClient := telemetry.NewClient()
	telemetry.ShowNoticeIfNeeded()

	var updated, failed, skipped int

	for _, tool := range tools {
		// Read pin constraint
		var requested string
		if state != nil {
			if ts, ok := state.Installed[tool.Name]; ok {
				if vs, ok := ts.Versions[ts.ActiveVersion]; ok {
					requested = vs.Requested
				}
			}
		}

		// Skip exact-pinned tools
		if install.PinLevelFromRequested(requested) == install.PinExact {
			skipped++
			continue
		}

		if updateDryRun {
			printInfof("Checking %s...\n", tool.Name)
			if err := runDryRun(tool.Name, ""); err != nil {
				printError(err)
				failed++
			}
			continue
		}

		// Load recipe from correct source
		if r, loadErr := loadRecipeForTool(context.Background(), tool.Name, state, cfg); loadErr == nil && r != nil {
			loader.CacheRecipe(tool.Name, r)
		}

		printInfof("Updating %s...\n", tool.Name)
		if err := runInstallWithTelemetry(tool.Name, requested, "", true, "", telemetryClient); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to update %s: %v\n", tool.Name, err)
			if telemetryClient != nil {
				telemetryClient.SendUpdateOutcome(telemetry.NewUpdateOutcomeFailureEvent(
					tool.Name, requested, telemetry.ClassifyError(err), "manual-batch"))
			}
			failed++
			continue
		}
		updated++
	}

	if updateDryRun {
		return
	}

	total := len(tools) - skipped
	if total == 0 {
		printInfo("All tools are exact-pinned, nothing to update.")
		return
	}

	if updated == 0 && failed == 0 {
		printInfo("All tools are up to date.")
	} else if failed == 0 {
		printInfof("Updated %d/%d tools.\n", updated, total)
	} else {
		printInfof("Updated %d/%d tools (%d failed).\n", updated, total, failed)
	}
}

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without making changes")
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all installed tools within pin boundaries")
}
