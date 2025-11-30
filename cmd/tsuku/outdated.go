package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/version"
)

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Check for outdated tools",
	Long:  `Check for newer versions of installed tools.`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonOutput, _ := cmd.Flags().GetBool("json")

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing tools: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		if len(tools) == 0 {
			if jsonOutput {
				type outdatedOutput struct {
					Updates []struct{} `json:"updates"`
				}
				printJSON(outdatedOutput{Updates: []struct{}{}})
				return
			}
			printInfo("No tools installed.")
			return
		}

		if !jsonOutput {
			printInfo("Checking for updates...")
		}
		res := version.New()
		ctx := context.Background()

		type updateInfo struct {
			Name    string `json:"name"`
			Current string `json:"current"`
			Latest  string `json:"latest"`
			Repo    string `json:"-"`
		}
		var updates []updateInfo

		for _, tool := range tools {
			// Load recipe to find repo
			r, err := loader.Get(tool.Name)
			if err != nil {
				continue
			}

			// Find repo
			var repo string
			for _, step := range r.Steps {
				if step.Action == "github_archive" || step.Action == "github_file" {
					if r, ok := step.Params["repo"].(string); ok {
						repo = r
						break
					}
				}
			}

			if repo == "" {
				continue
			}

			// Check latest version
			if !jsonOutput {
				printInfof("Checking %s...\n", tool.Name)
			}
			latest, err := res.ResolveGitHub(ctx, repo)
			if err != nil {
				printError(err)
				continue
			}

			if latest.Version != tool.Version {
				// Simple string comparison for now.
				// Ideally should use semver, but this is a good start.
				// We assume if strings differ, and we just fetched latest, it's likely an update.
				// But to be safe, we only show if they are different.
				updates = append(updates, updateInfo{
					Name:    tool.Name,
					Current: tool.Version,
					Latest:  latest.Version,
					Repo:    repo,
				})
			}
		}

		// JSON output mode
		if jsonOutput {
			type outdatedOutput struct {
				Updates []updateInfo `json:"updates"`
			}
			output := outdatedOutput{Updates: updates}
			if output.Updates == nil {
				output.Updates = []updateInfo{}
			}
			printJSON(output)
			return
		}

		printInfo()
		if len(updates) == 0 {
			printInfo("All tools are up to date!")
			return
		}

		fmt.Printf("%-15s  %-15s  %-15s\n", "TOOL", "CURRENT", "LATEST")
		for _, u := range updates {
			fmt.Printf("%-15s  %-15s  %-15s\n", u.Name, u.Current, u.Latest)
		}
		printInfo("\nTo update, run: tsuku update <tool>")
	},
}

func init() {
	outdatedCmd.Flags().Bool("json", false, "Output in JSON format")
}
