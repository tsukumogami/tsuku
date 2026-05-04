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
	Short: "Show recent update notices",
	Long: `Display recent auto-update results.

Shows tool name, attempted version, error message (if any), and timestamp for
each update result. Failure notices persist until the tool is successfully updated.
Success notices are shown once and cleared after viewing.`,
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
			printInfo("No update notices.")
			return
		}

		fmt.Printf("%-15s  %-15s  %-30s  %s\n", "TOOL", "VERSION", "ERROR", "TIMESTAMP")
		var failures, successes int
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
			if n.Error != "" {
				failures++
			} else {
				successes++
			}
		}

		switch {
		case failures > 0 && successes > 0:
			printInfof("\n%d failure(s), %d success(es). Failed updates were auto-rolled back. Success entries cleared after viewing.\n", failures, successes)
		case failures > 0:
			printInfof("\n%d failure(s). Failed updates were auto-rolled back.\n", failures)
		default:
			printInfof("\n%d success(es). Cleared after viewing.\n", successes)
		}

		// Success notices are shown once; delete them after display.
		for _, n := range all {
			if n.Error == "" {
				_ = notices.RemoveNotice(noticesDir, n.Tool)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(noticesCmd)
}
