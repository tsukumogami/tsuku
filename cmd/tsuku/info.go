package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
)

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
			fmt.Printf("Tool '%s' not found in registry.\n", toolName)
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
