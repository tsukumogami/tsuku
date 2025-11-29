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
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		mgr := install.New(cfg)
		tools, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing tools: %v\n", err)
			os.Exit(1)
		}

		if len(tools) == 0 {
			fmt.Println("No tools installed.")
			return
		}

		fmt.Println("Checking for updates...")
		res := version.New()
		ctx := context.Background()

		type updateInfo struct {
			Name    string
			Current string
			Latest  string
			Repo    string
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
			fmt.Printf("Checking %s...\n", tool.Name)
			latest, err := res.ResolveGitHub(ctx, repo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to check %s: %v\n", tool.Name, err)
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

		fmt.Println()
		if len(updates) == 0 {
			fmt.Println("All tools are up to date!")
			return
		}

		fmt.Printf("%-15s  %-15s  %-15s\n", "TOOL", "CURRENT", "LATEST")
		for _, u := range updates {
			fmt.Printf("%-15s  %-15s  %-15s\n", u.Name, u.Current, u.Latest)
		}
		fmt.Println("\nTo update, run: tsuku update <tool>")
	},
}
