package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/buildinfo"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/registry"
)

var quietFlag bool

// globalCtx is the application-level context that is canceled on SIGINT/SIGTERM.
// Commands should use this context for cancellable operations.
var globalCtx context.Context
var globalCancel context.CancelFunc

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
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(recipesCmd)
	rootCmd.AddCommand(versionsCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(outdatedCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(verifyCmd)
	rootCmd.AddCommand(updateRegistryCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(evalCmd)
}

func main() {
	// Set up cancellable context with signal handling
	globalCtx, globalCancel = context.WithCancel(context.Background())
	defer globalCancel()

	// Set up signal handling for graceful cancellation
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle signals in a goroutine
	go func() {
		sig := <-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived %s, canceling operation...\n", sig)
		globalCancel()

		// Wait for second signal to force exit
		<-sigChan
		fmt.Fprintln(os.Stderr, "Forced exit")
		exitWithCode(ExitCancelled)
	}()

	if err := rootCmd.Execute(); err != nil {
		// Check if the error was due to context cancellation
		if globalCtx.Err() == context.Canceled {
			exitWithCode(ExitCancelled)
		}
		fmt.Fprintln(os.Stderr, err)
		exitWithCode(ExitGeneral)
	}
}
