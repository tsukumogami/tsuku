package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <tool>",
	Short: "Revert a tool to its previous version",
	Long: `Revert a tool to the version that was active before the most recent update.

Rollback is one level deep: it switches to the immediately preceding version.
For deeper rollback, use 'tsuku install <tool>@<version>'.

Rollback does not change the version pin (Requested field), so auto-update
may re-apply the update on the next cycle. This is intentional: rollback is
a temporary fix for a broken release, not a permanent pin change.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading state: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		ts, ok := state.Installed[toolName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: %s is not installed\n", toolName)
			exitWithCode(ExitGeneral)
		}

		if ts.PreviousVersion == "" {
			fmt.Fprintf(os.Stderr, "Error: no previous version to roll back to for %s\n", toolName)
			exitWithCode(ExitGeneral)
		}

		// Verify previous version directory exists
		toolDir := cfg.ToolDir(toolName, ts.PreviousVersion)
		if _, err := os.Stat(toolDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: previous version directory not found: %s\n", toolDir)
			fmt.Fprintf(os.Stderr, "The previous version (%s) may have been garbage collected.\n", ts.PreviousVersion)
			fmt.Fprintf(os.Stderr, "Use 'tsuku install %s@%s' to install it explicitly.\n", toolName, ts.PreviousVersion)
			exitWithCode(ExitGeneral)
		}

		currentVersion := ts.ActiveVersion
		if err := mgr.Activate(toolName, ts.PreviousVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error rolling back %s: %v\n", toolName, err)
			exitWithCode(ExitGeneral)
		}

		tc := telemetry.NewClient()
		tc.SendUpdateOutcome(telemetry.NewUpdateOutcomeRollbackEvent(
			toolName, ts.PreviousVersion, currentVersion, "manual"))

		printInfof("Rolled back %s from %s to %s\n", toolName, currentVersion, ts.PreviousVersion)
		printInfo("Note: auto-update may re-apply this update on the next cycle.")
		printInfo("To permanently pin a version, use: tsuku install " + toolName + "@" + ts.PreviousVersion)
	},
}

func init() {
	rootCmd.AddCommand(rollbackCmd)
}
