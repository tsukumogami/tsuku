package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/version"
)

var versionsCmd = &cobra.Command{
	Use:   "versions <tool>",
	Short: "List available versions for a tool",
	Long:  `List all available versions (tags) for a tool.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitWithCode(ExitRecipeNotFound)
		}

		// Create version provider using factory
		res := version.New()
		factory := version.NewProviderFactory()
		provider, err := factory.ProviderFromRecipe(res, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

		// List versions
		ctx := context.Background()
		if !jsonOutput {
			fmt.Printf("Fetching versions for %s (%s)...\n", toolName, provider.SourceDescription())
		}

		versions, err := lister.ListVersions(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list versions: %v\n", err)
			exitWithCode(ExitNetwork)
		}

		// JSON output mode
		if jsonOutput {
			type versionsOutput struct {
				Versions []string `json:"versions"`
			}
			printJSON(versionsOutput{Versions: versions})
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
}
