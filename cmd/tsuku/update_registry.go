package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var updateRegistryCmd = &cobra.Command{
	Use:   "update-registry",
	Short: "Clear the recipe cache",
	Long: `Clear the local recipe cache to force fresh downloads from the registry.

This is useful when you want to get the latest recipes without waiting for
automatic cache expiration.`,
	Run: func(cmd *cobra.Command, args []string) {
		reg := loader.Registry()
		if reg == nil {
			printInfo("Registry not configured.")
			return
		}

		printInfo("Clearing recipe cache...")
		if err := reg.ClearCache(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clear cache: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		// Also clear in-memory cache
		loader.ClearCache()

		printInfo("Recipe cache cleared. Recipes will be fetched fresh on next use.")
	},
}
