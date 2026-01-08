package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/buildinfo"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/registry"
)

var (
	quietFlag   bool
	verboseFlag bool
	debugFlag   bool
)

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
	// Global verbosity flags
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Show errors only")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show verbose output (INFO level)")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Show debug output (includes timestamps and source locations)")

	// Initialize logger before command execution
	rootCmd.PersistentPreRun = initLogger

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

	// Configure constraint lookup for step analysis (enables platform constraint validation)
	loader.SetConstraintLookup(defaultConstraintLookup())

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

// initLogger initializes the global logger based on flags and environment variables.
// Flags take precedence over environment variables.
func initLogger(cmd *cobra.Command, args []string) {
	level := determineLogLevel()
	handler := log.NewCLIHandler(level)
	logger := log.New(handler)
	log.SetDefault(logger)

	// Display warning banner when debug mode is enabled
	if level == slog.LevelDebug {
		fmt.Fprintln(os.Stderr, "[DEBUG MODE] Output may contain file paths and URLs. Do not share publicly.")
	}
}

// determineLogLevel returns the appropriate slog.Level based on flags and environment variables.
// Priority: flags > environment variables > default (WARN)
func determineLogLevel() slog.Level {
	// Flags take precedence
	if debugFlag {
		return slog.LevelDebug
	}
	if verboseFlag {
		return slog.LevelInfo
	}
	if quietFlag {
		return slog.LevelError
	}

	// Check environment variables
	if isTruthy(os.Getenv("TSUKU_DEBUG")) {
		return slog.LevelDebug
	}
	if isTruthy(os.Getenv("TSUKU_VERBOSE")) {
		return slog.LevelInfo
	}
	if isTruthy(os.Getenv("TSUKU_QUIET")) {
		return slog.LevelError
	}

	// Default to WARN level
	return slog.LevelWarn
}

// isTruthy returns true if the string represents a truthy value.
func isTruthy(s string) bool {
	s = strings.ToLower(s)
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

// defaultConstraintLookup returns a ConstraintLookup that uses the action registry.
// It returns (constraint, true) for known actions and (nil, false) for unknown actions.
// For SystemAction types, it converts the action's ImplicitConstraint to recipe.Constraint.
func defaultConstraintLookup() recipe.ConstraintLookup {
	return func(actionName string) (*recipe.Constraint, bool) {
		action := actions.Get(actionName)
		if action == nil {
			return nil, false // unknown action
		}

		// Check if action implements SystemAction with ImplicitConstraint
		if sysAction, ok := action.(actions.SystemAction); ok {
			if actConstraint := sysAction.ImplicitConstraint(); actConstraint != nil {
				// Convert actions.Constraint to recipe.Constraint
				return &recipe.Constraint{
					OS:          actConstraint.OS,
					LinuxFamily: actConstraint.LinuxFamily,
					// Arch is not set by action constraints (left empty)
				}, true
			}
		}

		// Known action with no implicit constraint
		return nil, true
	}
}
