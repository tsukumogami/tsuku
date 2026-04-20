package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var applyUpdatesCmd = &cobra.Command{
	Use:           "apply-updates",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Redirect stdout/stderr to devnull for truly silent background operation
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return nil
		}

		userCfg, err := userconfig.Load()
		if err != nil {
			return nil
		}

		installFn := func(toolName, version, constraint string) error {
			return runInstallWithTelemetry(toolName, version, constraint, false, "", nil)
		}

		updates.MaybeAutoApply(cfg, userCfg, nil, installFn, nil)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyUpdatesCmd)
}
