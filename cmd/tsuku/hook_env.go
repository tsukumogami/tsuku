package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/activation"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/userconfig"
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
		stamp := os.Getenv("_TSUKU_STATE_STAMP")

		result, err := activation.ComputeActivation(cwd, prevPath, curDir, stamp, cfg, install.NewStateManager(cfg))
		if err != nil {
			return err
		}

		// Trigger background update check (best-effort, <1ms).
		if userCfg, loadErr := userconfig.Load(); loadErr == nil {
			updates.CheckAndSpawnUpdateCheck(cfg, userCfg)
		}

		// No-op: no output, exit 0.
		if result == nil {
			return nil
		}

		// Reasons to stderr, shell code to stdout, and always exit 0: a prompt
		// hook that exits non-zero gets wrapped in "|| true" by users, which
		// would discard this reporting entirely.
		reportActivation(result)

		fmt.Print(activation.FormatExports(result, shell))
		return nil
	},
}
