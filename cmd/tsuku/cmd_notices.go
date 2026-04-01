package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
)

var noticesCmd = &cobra.Command{
	Use:   "notices",
	Short: "Show recent update failure notices",
	Long: `Display details of recent auto-update failures.

Shows tool name, attempted version, error message, and timestamp for
each failed update. Includes both previously shown and unshown notices.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		noticesDir := notices.NoticesDir(cfg.HomeDir)
		all, err := notices.ReadAllNotices(noticesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading notices: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		if len(all) == 0 {
			printInfo("No update failure notices.")
			return
		}

		fmt.Printf("%-15s  %-15s  %-30s  %s\n", "TOOL", "VERSION", "ERROR", "TIMESTAMP")
		for _, n := range all {
			errMsg := n.Error
			if len(errMsg) > 30 {
				errMsg = errMsg[:27] + "..."
			}
			fmt.Printf("%-15s  %-15s  %-30s  %s\n",
				n.Tool,
				n.AttemptedVersion,
				errMsg,
				n.Timestamp.Format("2006-01-02 15:04:05"),
			)
		}

		printInfof("\n%d notice(s) total. Failed updates were auto-rolled back.\n", len(all))
	},
}

func init() {
	rootCmd.AddCommand(noticesCmd)
}
