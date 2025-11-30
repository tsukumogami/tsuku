package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/telemetry"
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
			os.Exit(1)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			os.Exit(1)
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
			os.Exit(1)
		}

		if updateDryRun {
			fmt.Printf("Checking updates for %s...\n", toolName)
			if err := runDryRun(toolName, ""); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		fmt.Printf("Updating %s...\n", toolName)
		if err := runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient); err != nil {
			os.Exit(1)
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

func init() {
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show what would be updated without making changes")
}
