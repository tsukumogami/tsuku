package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
)

var shellenvCmd = &cobra.Command{
	Use:   "shellenv",
	Short: "Print shell commands to configure PATH for tsuku",
	Long: `Print shell commands that configure PATH to include tsuku's bin and
tools/current directories. Useful for users who install tsuku without
the install script, or for development builds that use a non-default
home directory.

Usage in shell profile:
  eval $(tsuku shellenv)

Usage for one-off sessions:
  eval $(./tsuku shellenv)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("failed to get config: %w", err)
		}

		homeDir, err := filepath.Abs(cfg.HomeDir)
		if err != nil {
			return fmt.Errorf("failed to resolve home directory: %w", err)
		}

		binDir := filepath.Join(homeDir, "bin")
		currentDir := filepath.Join(homeDir, "tools", "current")

		fmt.Fprintf(os.Stdout, "export PATH=\"%s:%s:$PATH\"\n", binDir, currentDir)
		return nil
	},
}
