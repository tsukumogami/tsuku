package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var recipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "List available recipes",
	Long:  `List all cached recipes from the registry.`,
	Run: func(cmd *cobra.Command, args []string) {
		names := loader.List()
		sort.Strings(names)

		fmt.Printf("Available recipes (%d total):\n\n", loader.Count())

		for _, name := range names {
			r, _ := loader.Get(name)
			fmt.Printf("  %-20s  %s\n", name, r.Metadata.Description)
		}
	},
}
