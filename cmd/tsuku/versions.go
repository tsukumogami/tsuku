package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

var versionsCmd = &cobra.Command{
	Use:   "versions <tool>",
	Short: "List available versions for a tool",
	Long:  `List all available versions (tags) for a tool.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		refresh, _ := cmd.Flags().GetBool("refresh")

		// Load recipe
		r, err := loader.Get(toolName, recipe.LoaderOptions{})
		if err != nil {
			printError(err)
			exitWithCode(ExitRecipeNotFound)
		}

		// Create version provider using factory
		res := version.New()
		factory := version.NewProviderFactory()
		provider, err := factory.ProviderFromRecipe(res, r)
		if err != nil {
			printError(err)
			exitWithCode(ExitGeneral)
		}

		// Check if provider can list versions (Interface Segregation Principle)
		lister, canList := provider.(version.VersionLister)
		if !canList {
			fmt.Fprintf(os.Stderr, "Version listing not supported for %s (%s)\n",
				toolName, provider.SourceDescription())
			fmt.Fprintln(os.Stderr, "This source can resolve specific versions but cannot enumerate all versions.")
			exitWithCode(ExitGeneral)
		}

		// Wrap with cache
		cfg, err := config.DefaultConfig()
		if err != nil {
			printError(err)
			exitWithCode(ExitGeneral)
		}
		cachedLister := version.NewCachedVersionLister(lister, cfg.VersionCacheDir, config.GetVersionCacheTTL())

		// List versions
		ctx := context.Background()

		var versions []string
		var fromCache bool

		if refresh {
			// Bypass cache with --refresh flag
			if !jsonOutput {
				fmt.Printf("Fetching fresh versions for %s (%s)...\n", toolName, provider.SourceDescription())
			}
			versions, err = cachedLister.Refresh(ctx)
		} else {
			// Use cache
			versions, fromCache, err = cachedLister.ListVersionsWithCacheInfo(ctx)
			if !jsonOutput {
				if fromCache {
					cacheInfo := cachedLister.GetCacheInfo()
					fmt.Printf("Using cached versions for %s (%s) [expires %s]\n",
						toolName, provider.SourceDescription(), cacheInfo.ExpiresAt.Format("2006-01-02 15:04"))
				} else {
					fmt.Printf("Fetching versions for %s (%s)...\n", toolName, provider.SourceDescription())
				}
			}
		}

		if err != nil {
			printError(err)
			exitWithCode(ExitNetwork)
		}

		// JSON output mode
		if jsonOutput {
			type versionsOutput struct {
				Versions  []string `json:"versions"`
				FromCache bool     `json:"from_cache"`
			}
			printJSON(versionsOutput{Versions: versions, FromCache: fromCache})
			return
		}

		fmt.Printf("Available versions (%d total):\n\n", len(versions))
		for _, v := range versions {
			fmt.Printf("  %s\n", v)
		}
	},
}

func init() {
	versionsCmd.Flags().Bool("json", false, "Output in JSON format")
	versionsCmd.Flags().Bool("refresh", false, "Bypass cache and fetch fresh version list")
}
