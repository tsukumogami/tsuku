package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

var hookEnvCmd = &cobra.Command{
	Use:    "hook-env [shell]",
	Short:  "Compute and print environment activation for prompt hooks",
	Long:   `Internal command used by shell prompt hooks to activate per-project tool versions. Not intended for direct use.`,
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := args[0]

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		prevPath := os.Getenv("_TSUKU_PREV_PATH")
		curDir := os.Getenv("_TSUKU_DIR")

		result, err := shellenv.ComputeActivation(cwd, prevPath, curDir, cfg)
		if err != nil {
			return err
		}

		// No-op: no output, exit 0.
		if result == nil {
			return nil
		}

		fmt.Print(shellenv.FormatExports(result, shell))
		return nil
	},
}
