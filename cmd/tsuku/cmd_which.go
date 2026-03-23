package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

var whichCmd = &cobra.Command{
	Use:   "which <command>",
	Short: "Show which recipe provides a command",
	Long: `Show which recipe provides a command.

Looks up the binary index to find which recipe(s) install the given command.
The index must be built first by running 'tsuku update-registry'.

Examples:
  tsuku which jq
  tsuku which kubectl`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		command := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		dbPath := filepath.Join(cfg.CacheDir, "binary-index.db")

		idx, err := index.Open(dbPath, cfg.RegistryDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open binary index: %v\n", err)
			exitWithCode(ExitGeneral)
		}
		defer func() { _ = idx.Close() }()

		matches, err := idx.Lookup(globalCtx, command)
		if err != nil {
			if errors.Is(err, index.ErrIndexNotBuilt) {
				fmt.Println("Binary index not built. Run 'tsuku update-registry' first.")
				exitWithCode(ExitGeneral)
			}
			// StaleIndexWarning: results are still valid; print the warning but continue.
			var stale index.StaleIndexWarning
			if !errors.As(err, &stale) {
				fmt.Fprintf(os.Stderr, "Failed to look up command: %v\n", err)
				exitWithCode(ExitGeneral)
			}
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if len(matches) == 0 {
			fmt.Printf("%s not found in binary index\n", command)
			exitWithCode(ExitGeneral)
		}

		if len(matches) == 1 {
			fmt.Printf("%s is provided by recipe '%s'\n", command, matches[0].Recipe)
			return
		}

		// Multiple matches: print a table.
		fmt.Fprintf(os.Stdout, "%-30s  %-40s  %s\n", "Recipe", "Binary Path", "Installed")
		fmt.Fprintf(os.Stdout, "%-30s  %-40s  %s\n", "------", "-----------", "---------")
		for _, m := range matches {
			installed := "no"
			if m.Installed {
				installed = "yes"
			}
			fmt.Fprintf(os.Stdout, "%-30s  %-40s  %s\n", m.Recipe, m.BinaryPath, installed)
		}
	},
}
