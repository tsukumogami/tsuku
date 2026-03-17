package main

import (
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

		// Ensure distributed provider is registered if the tool came from
		// a distributed source (owner/repo). This must happen before the
		// install flow so the loader can resolve the recipe.
		if err := ensureSourceProvider(toolName, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not set up source provider for %s: %v\n", toolName, err)
			// Continue with the default chain -- the central registry may
			// still have a recipe with the same name.
		}

		if updateDryRun {
			printInfof("Checking updates for %s...\n", toolName)
			if err := runDryRun(toolName, ""); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

		printInfof("Updating %s...\n", toolName)
		if err := runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient); err != nil {
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

		// Send telemetry event for update
		if telemetryClient != nil && newVersion != "" {
			event := telemetry.NewUpdateEvent(toolName, previousVersion, newVersion)
			telemetryClient.Send(event)
		}
	},
}

// ensureSourceProvider checks the tool's recorded source and, if it's a
// distributed source (owner/repo), ensures the corresponding provider is
// registered in the loader. For central/embedded/local/empty sources this
// is a no-op because those providers are always available.
func ensureSourceProvider(toolName string, cfg *config.Config) error {
	toolState, err := install.New(cfg).GetState().GetToolState(toolName)
	if err != nil || toolState == nil {
		return nil // Not installed or no state -- nothing to do
	}

	source := toolState.Source
	if source == "" {
		return nil // Empty defaults to central -- already available
	}

	if !isDistributedSource(source) {
		return nil // central, local, embedded -- already in the chain
	}

	// Distributed source: dynamically register the provider
	return addDistributedProvider(source, cfg)
}

// isDistributedSource returns true if the source string is a distributed
// "owner/repo" source (as opposed to "central", "local", or "embedded").
func isDistributedSource(source string) bool {
	return strings.Contains(source, "/")
}

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without making changes")
}
