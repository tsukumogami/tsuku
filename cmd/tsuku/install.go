package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

var installDryRun bool
var installForce bool
var installFresh bool
var installPlanPath string
var installSandbox bool
var installRecipePath string
var installTargetFamily string
var installRequireEmbedded bool

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
  tsuku eval rg | tsuku install --plan -

Test installation in a sandbox container:
  tsuku install kubectl --sandbox
  tsuku install --recipe ./my-recipe.toml --sandbox
  tsuku eval rg | tsuku install --plan - --sandbox`,
	Args: cobra.ArbitraryArgs, // Allow zero args when --plan or --recipe is used
	Run: func(cmd *cobra.Command, args []string) {
		// Sandbox installation mode
		if installSandbox {
			// Validate: cannot specify multiple tools with --sandbox
			if len(args) > 1 {
				printError(fmt.Errorf("cannot specify multiple tools with --sandbox flag"))
				exitWithCode(ExitUsage)
			}

			// Dry-run is not supported with --sandbox
			if installDryRun {
				printError(fmt.Errorf("--dry-run is not supported with --sandbox"))
				exitWithCode(ExitUsage)
			}

			// Determine tool name
			var toolName string
			if len(args) == 1 {
				toolName = args[0]
			}

			// Require either tool name, --plan, or --recipe
			if toolName == "" && installPlanPath == "" && installRecipePath == "" {
				printError(fmt.Errorf("--sandbox requires a tool name, --plan, or --recipe"))
				exitWithCode(ExitUsage)
			}

			if err := runSandboxInstall(toolName, installPlanPath, installRecipePath); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

		// Recipe-based installation (without sandbox) - load recipe and convert to plan
		if installRecipePath != "" {
			// Validate: cannot specify multiple tools with --recipe
			if len(args) > 1 {
				printError(fmt.Errorf("cannot specify multiple tools with --recipe flag"))
				exitWithCode(ExitUsage)
			}

			// Dry-run is not supported with --recipe (plan will be generated)
			if installDryRun {
				printError(fmt.Errorf("--dry-run is not supported with --recipe"))
				exitWithCode(ExitUsage)
			}

			// Tool name is optional - defaults to recipe's tool name
			var toolName string
			if len(args) == 1 {
				toolName = args[0]
			}

			if err := runRecipeBasedInstall(installRecipePath, toolName); err != nil {
				printError(err)
				exitWithCode(ExitInstallFailed)
			}
			return
		}

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
					// TODO(#1273): Distinguish dependency failures (ExitDependencyFailed)
					// from other install failures. Currently all errors exit with
					// ExitInstallFailed, which prevents programmatic consumers from
					// identifying missing dependencies via exit code alone.
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
	installCmd.Flags().BoolVar(&installSandbox, "sandbox", false, "Run installation in an isolated container for testing")
	installCmd.Flags().StringVar(&installRecipePath, "recipe", "", "Path to a local recipe file (for testing)")
	installCmd.Flags().StringVar(&installTargetFamily, "target-family", "", "Override detected linux_family (debian, rhel, arch, alpine, suse)")
	installCmd.Flags().BoolVar(&installRequireEmbedded, "require-embedded", false, "Require action dependencies to resolve from embedded registry")
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
	r, err := loader.Get(toolName, recipe.LoaderOptions{})
	if err != nil {
		return fmt.Errorf("recipe not found: %w", err)
	}

	// Check platform support before installation
	if !r.SupportsPlatformRuntime() {
		return r.NewUnsupportedPlatformError()
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

// runRecipeBasedInstall loads a recipe from a file and installs it
func runRecipeBasedInstall(recipePath, toolName string) error {
	// Load recipe from file
	r, err := recipe.ParseFile(recipePath)
	if err != nil {
		return fmt.Errorf("failed to load recipe from %s: %w", recipePath, err)
	}

	// Use recipe's name if tool name not provided
	if toolName == "" {
		toolName = r.Metadata.Name
	}

	// Initialize telemetry
	telemetryClient := telemetry.NewClient()
	telemetry.ShowNoticeIfNeeded()

	// Cache the recipe in the loader so normal installation flow can find it
	loader.CacheRecipe(toolName, r)

	// Use the normal installation flow with the cached recipe
	if err := runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient); err != nil {
		return err
	}

	return nil
}
