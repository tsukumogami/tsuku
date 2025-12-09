package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var activateCmd = &cobra.Command{
	Use:   "activate <tool> <version>",
	Short: "Switch to a different installed version of a tool",
	Long: `Switch to a different installed version of a tool.

This updates the symlinks in $TSUKU_HOME/bin to point to the specified version.
The requested version must already be installed.

Examples:
  tsuku activate liberica-jdk 17.0.12
  tsuku activate nodejs 20.10.0`,
	Args: cobra.ExactArgs(2),
	Run:  runActivate,
}

func init() {
	// No flags needed
}

func runActivate(cmd *cobra.Command, args []string) {
	toolName := args[0]
	version := args[1]

	// Load config
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Create manager
	mgr := install.New(cfg)

	// Activate the version
	if err := mgr.Activate(toolName, version); err != nil {
		printError(err)
		exitWithCode(ExitGeneral)
	}

	fmt.Printf("Activated %s version %s\n", toolName, version)
}
