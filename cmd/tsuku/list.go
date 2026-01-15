package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed tools",
	Long:  `List all tools currently installed by tsuku.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)

		// Check flags
		showSystemDeps, _ := cmd.Flags().GetBool("show-system-dependencies")
		showAll, _ := cmd.Flags().GetBool("all")
		showApps, _ := cmd.Flags().GetBool("apps")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		var tools []install.InstalledTool
		if showSystemDeps {
			tools, err = mgr.ListAll()
		} else {
			tools, err = mgr.List()
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list tools: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		// Get libraries if --all flag is set
		var libs []install.InstalledLibrary
		if showAll {
			libs, err = mgr.ListLibraries()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to list libraries: %v\n", err)
				exitWithCode(ExitGeneral)
			}
		}

		// Get apps if --apps flag is set
		var apps []install.InstalledApp
		if showApps || showAll {
			apps, err = mgr.ListApps()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to list apps: %v\n", err)
				exitWithCode(ExitGeneral)
			}
		}

		// JSON output mode
		if jsonOutput {
			type itemJSON struct {
				Name     string `json:"name"`
				Version  string `json:"version"`
				Path     string `json:"path"`
				Type     string `json:"type,omitempty"`
				IsActive bool   `json:"is_active,omitempty"`
			}
			type appJSON struct {
				Name               string `json:"name"`
				Version            string `json:"version"`
				AppPath            string `json:"app_path"`
				ApplicationSymlink string `json:"application_symlink,omitempty"`
				IsActive           bool   `json:"is_active,omitempty"`
			}
			type listOutput struct {
				Tools     []itemJSON `json:"tools"`
				Libraries []itemJSON `json:"libraries,omitempty"`
				Apps      []appJSON  `json:"apps,omitempty"`
			}
			output := listOutput{
				Tools:     make([]itemJSON, 0, len(tools)),
				Libraries: make([]itemJSON, 0, len(libs)),
				Apps:      make([]appJSON, 0, len(apps)),
			}
			for _, t := range tools {
				output.Tools = append(output.Tools, itemJSON{
					Name:     t.Name,
					Version:  t.Version,
					Path:     t.Path,
					Type:     "tool",
					IsActive: t.IsActive,
				})
			}
			for _, l := range libs {
				output.Libraries = append(output.Libraries, itemJSON{
					Name:    l.Name,
					Version: l.Version,
					Path:    l.Path,
					Type:    "library",
				})
			}
			for _, a := range apps {
				output.Apps = append(output.Apps, appJSON{
					Name:               a.Name,
					Version:            a.Version,
					AppPath:            a.AppPath,
					ApplicationSymlink: a.ApplicationSymlink,
					IsActive:           a.IsActive,
				})
			}
			printJSON(output)
			return
		}

		if len(tools) == 0 && len(libs) == 0 && len(apps) == 0 {
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
			exitWithCode(ExitGeneral)
		}

		for _, tool := range tools {
			prefix := "  "
			if toolState, exists := state.Installed[tool.Name]; exists && toolState.IsExecutionDependency {
				prefix = "* "
			}
			activeIndicator := ""
			if tool.IsActive {
				activeIndicator = " (active)"
			}
			fmt.Printf("%s%-20s  %s%s\n", prefix, tool.Name, tool.Version, activeIndicator)
		}

		if showSystemDeps {
			printInfo("\n* System dependency (installed by tsuku for internal use)")
		}

		// Show libraries if --all flag is set
		if showAll && len(libs) > 0 {
			printInfof("\nInstalled libraries (%d total):\n\n", len(libs))
			for _, lib := range libs {
				fmt.Printf("  %-20s  %s  [lib]\n", lib.Name, lib.Version)
			}
		}

		// Show apps if --apps or --all flag is set
		if (showApps || showAll) && len(apps) > 0 {
			printInfof("\nInstalled applications (%d total):\n\n", len(apps))
			for _, app := range apps {
				activeIndicator := ""
				if app.IsActive {
					activeIndicator = " (active)"
				}
				fmt.Printf("  %-20s  %s%s\n", app.Name, app.Version, activeIndicator)
				fmt.Printf("    App: %s\n", app.AppPath)
				if app.ApplicationSymlink != "" {
					fmt.Printf("    Link: %s\n", app.ApplicationSymlink)
				}
			}
		}
	},
}

func init() {
	listCmd.Flags().Bool("show-system-dependencies", false, "Include hidden system dependencies in output")
	listCmd.Flags().Bool("all", false, "Include libraries and applications in output")
	listCmd.Flags().Bool("apps", false, "Show only macOS application bundles")
	listCmd.Flags().Bool("json", false, "Output in JSON format")
}
