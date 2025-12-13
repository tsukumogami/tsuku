package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var installDryRun bool
var installForce bool
var installFresh bool
var installPlanPath string

var installCmd = &cobra.Command{
	Use:   "install [tool]...",
	Short: "Install a development tool",
	Long: `Install a development tool from the recipe registry.
You can specify a version using the @ syntax.

Examples:
  tsuku install kubectl
  tsuku install kubectl@v1.29.0
  tsuku install terraform@latest

Install from a pre-computed plan:
  tsuku install --plan plan.json
  tsuku eval rg | tsuku install --plan -`,
	Args: cobra.ArbitraryArgs, // Allow zero args when --plan is used
	Run: func(cmd *cobra.Command, args []string) {
		// Plan-based installation takes a different path
		if installPlanPath != "" {
			// Validate: cannot specify multiple tools with --plan
			if len(args) > 1 {
				printError(fmt.Errorf("cannot specify multiple tools with --plan flag"))
				exitWithCode(ExitUsage)
			}

			// Dry-run is not supported with --plan (plan already exists)
			if installDryRun {
				printError(fmt.Errorf("--dry-run is not supported with --plan (plan already exists)"))
				exitWithCode(ExitUsage)
			}

			// Tool name is optional - defaults to plan's tool name
			var toolName string
			if len(args) == 1 {
				toolName = args[0]
			}

			if err := runPlanBasedInstall(installPlanPath, toolName); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

		// Normal installation: require at least one tool
		if len(args) == 0 {
			printError(fmt.Errorf("requires at least 1 arg(s), only received 0"))
			exitWithCode(ExitUsage)
		}

		// Initialize telemetry
		telemetryClient := telemetry.NewClient()
		telemetry.ShowNoticeIfNeeded()

		for _, arg := range args {
			toolName := arg
			versionConstraint := ""

			if strings.Contains(arg, "@") {
				parts := strings.SplitN(arg, "@", 2)
				toolName = parts[0]
				versionConstraint = parts[1]
			}

			// Convert "latest" to empty for resolution, but keep original constraint for telemetry
			resolveVersion := versionConstraint
			if resolveVersion == "latest" {
				resolveVersion = ""
			}

			if installDryRun {
				if err := runDryRun(toolName, resolveVersion); err != nil {
					printError(err)
					exitWithCode(ExitInstallFailed)
				}
			} else {
				if err := runInstallWithTelemetry(toolName, resolveVersion, versionConstraint, true, "", telemetryClient); err != nil {
					// Continue installing other tools even if one fails?
					// For now, exit on first failure to be safe
					printError(err)
					exitWithCode(ExitInstallFailed)
				}
			}
		}
	},
}

func init() {
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Show what would be installed without making changes")
	installCmd.Flags().BoolVar(&installForce, "force", false, "Skip security warnings and proceed without prompts")
	installCmd.Flags().BoolVar(&installFresh, "fresh", false, "Force fresh plan generation, bypassing cached plans")
	installCmd.Flags().StringVar(&installPlanPath, "plan", "", "Install from a pre-computed plan file (use '-' for stdin)")
}

// isInteractive returns true if stdin is connected to a terminal
func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// confirmInstall prompts the user for confirmation and returns true if they agree
func confirmInstall() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "Continue installation? [y/N] ")
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// runDryRun shows what would be installed without making changes
func runDryRun(toolName, reqVersion string) error {
	// Load recipe
	r, err := loader.Get(toolName)
	if err != nil {
		return fmt.Errorf("recipe not found: %w", err)
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}
	defer exec.Cleanup()

	// Run dry-run with cancellable context
	return exec.DryRun(globalCtx)
}
