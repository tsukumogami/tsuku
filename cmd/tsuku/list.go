package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed tools",
	Long:  `List all tools currently installed by tsuku.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)

		// Check if --show-system-dependencies flag is set
		showSystemDeps, _ := cmd.Flags().GetBool("show-system-dependencies")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		var tools []install.InstalledTool
		if showSystemDeps {
			tools, err = mgr.ListAll()
		} else {
			tools, err = mgr.List()
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			os.Exit(1)
		}

		// JSON output mode
		if jsonOutput {
			type toolJSON struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Path    string `json:"path"`
			}
			type listOutput struct {
				Tools []toolJSON `json:"tools"`
			}
			output := listOutput{Tools: make([]toolJSON, 0, len(tools))}
			for _, t := range tools {
				output.Tools = append(output.Tools, toolJSON{
					Name:    t.Name,
					Version: t.Version,
					Path:    t.Path,
				})
			}
			printJSON(output)
			return
		}

		if len(tools) == 0 {
			printInfo("No tools installed.")
			return
		}

		if showSystemDeps {
			printInfof("Installed tools (%d total, including system dependencies):\n\n", len(tools))
		} else {
			printInfof("Installed tools (%d total):\n\n", len(tools))
		}

		// Load state to show system dependency indicator
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
			os.Exit(1)
		}

		for _, tool := range tools {
			prefix := "  "
			if toolState, exists := state.Installed[tool.Name]; exists && toolState.IsExecutionDependency {
				prefix = "* "
			}
			fmt.Printf("%s%-20s  %s\n", prefix, tool.Name, tool.Version)
		}

		if showSystemDeps {
			printInfo("\n* System dependency (installed by tsuku for internal use)")
		}
	},
}

func init() {
	listCmd.Flags().Bool("show-system-dependencies", false, "Include hidden system dependencies in output")
	listCmd.Flags().Bool("json", false, "Output in JSON format")
}
