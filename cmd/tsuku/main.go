package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/buildinfo"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/registry"
)

var quietFlag bool

var rootCmd = &cobra.Command{
	Use:   "tsuku",
	Short: "A modern, universal package manager for development tools",
	Long: `tsuku is a package manager that makes it easy to install and manage
development tools across different platforms.

It uses action-based recipes to download, extract, and install tools
to version-specific directories, with automatic PATH management.`,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress informational output")

	// Set version from build info (handles tagged releases and dev builds)
	rootCmd.Version = buildinfo.Version()

	// Initialize registry client
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	reg := registry.New(cfg.RegistryDir)

	// Initialize recipe loader with registry and local recipes directory
	loader = recipe.NewWithLocalRecipes(reg, cfg.RecipesDir)

	// Register all commands
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(recipesCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(outdatedCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(updateRegistryCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(completionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitWithCode(ExitGeneral)
	}
}
