package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
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

		// Load state to access each tool's recorded source
		state, stateErr := mgr.GetState().Load()
		if stateErr != nil {
			fmt.Fprintf(os.Stderr, "Error loading state: %v\n", stateErr)
			exitWithCode(ExitGeneral)
		}

		if !jsonOutput {
			printInfo("Checking for updates...")
		}
		res := version.New()
		factory := version.NewProviderFactory()
		ctx := context.Background()

		type updateInfo struct {
			Name    string `json:"name"`
			Current string `json:"current"`
			Latest  string `json:"latest"`
		}
		var updates []updateInfo

		for _, tool := range tools {
			// Skip exact-pinned tools (they can't update by definition)
			var requested string
			if ts, ok := state.Installed[tool.Name]; ok {
				if vs, vok := ts.Versions[ts.ActiveVersion]; vok {
					requested = vs.Requested
				}
			}
			if install.PinLevelFromRequested(requested) == install.PinExact {
				continue
			}

			// Load recipe using source-directed loading
			r, err := loadRecipeForTool(ctx, tool.Name, state, cfg)
			if err != nil {
				continue
			}

			// Create provider via ProviderFactory (covers all provider types)
			provider, err := factory.ProviderFromRecipe(res, r)
			if err != nil {
				continue
			}

			// Check latest version within pin boundary
			if !jsonOutput {
				printInfof("Checking %s...\n", tool.Name)
			}
			latest, err := version.ResolveWithinBoundary(ctx, provider, requested)
			if err != nil {
				continue
			}

			if latest.Version != tool.Version {
				updates = append(updates, updateInfo{
					Name:    tool.Name,
					Current: tool.Version,
					Latest:  latest.Version,
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

// loadRecipeForTool loads a recipe using source-directed loading when the tool
// has a distributed source. For central/local/embedded/empty sources, it falls
// back to the normal loader chain.
//
// If a distributed source is unreachable, the function falls back to the normal
// loader chain and logs a warning rather than failing fatally.
func loadRecipeForTool(ctx context.Context, toolName string, state *install.State, cfg *config.Config) (*recipe.Recipe, error) {
	source := ""
	if state != nil {
		if ts, ok := state.Installed[toolName]; ok {
			source = ts.Source
		}
	}

	// Empty source defaults to central -- use normal chain
	if source == "" || !isDistributedSource(source) {
		return loader.Get(toolName, recipe.LoaderOptions{})
	}

	// Ensure the distributed provider is registered
	if err := addDistributedProvider(source, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not reach source %q for %s, falling back to default chain: %v\n", source, toolName, err)
		return loader.Get(toolName, recipe.LoaderOptions{})
	}

	// Fetch from the recorded source
	data, err := loader.GetFromSource(ctx, toolName, source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load %s from %s, falling back to default chain: %v\n", toolName, source, err)
		return loader.Get(toolName, recipe.LoaderOptions{})
	}

	// Parse the raw bytes into a Recipe
	return loader.ParseAndCache(ctx, toolName, data)
}

func init() {
	outdatedCmd.Flags().Bool("json", false, "Output in JSON format")
}
