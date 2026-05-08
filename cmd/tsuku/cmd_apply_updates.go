package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/progress"
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

		noticesDir := notices.NoticesDir(cfg.HomeDir)
		var reporters []*progress.InboxReporter

		installFn := func(toolName, version, constraint string) error {
			reporter := progress.NewInboxReporter(toolName, noticesDir)
			reporters = append(reporters, reporter)
			return runInstallWithExternalReporter(toolName, version, constraint, false, "", nil, reporter)
		}

		updates.MaybeAutoApply(cfg, userCfg, nil, installFn, nil)

		// Stop reporters after MaybeAutoApply has written success notices.
		// Reporters with no accumulated messages return early, leaving the
		// success notice intact. Reporters with warnings (e.g., version_fallback)
		// overwrite the success notice with a richer notice.
		for _, r := range reporters {
			r.Stop()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyUpdatesCmd)
}
