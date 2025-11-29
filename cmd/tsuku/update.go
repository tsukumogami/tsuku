package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
)

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
