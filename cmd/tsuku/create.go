package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/toolchain"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/validate"
)

const (
	// defaultLLMCostEstimate is the estimated cost per LLM generation in USD.
	// This is a conservative estimate based on typical token usage.
	defaultLLMCostEstimate = 0.10
)

var createCmd = &cobra.Command{
	Use:   "create <tool> --from <source>",
	Short: "Create a recipe from a package ecosystem or GitHub",
	Long: `Create a recipe by querying a package ecosystem's metadata API or analyzing
GitHub release assets.

The generated recipe is written to $TSUKU_HOME/recipes/<tool>.toml and can be
inspected or edited before running 'tsuku install <tool>'.

Supported sources:
  crates.io          Rust crates from crates.io
  rubygems           Ruby gems from rubygems.org
  pypi               Python packages from pypi.org
  npm                Node.js packages from npmjs.com
  github:owner/repo  GitHub releases (uses LLM to analyze assets)

Examples:
  tsuku create ripgrep --from crates.io
  tsuku create bat --from crates.io --force
  tsuku create jekyll --from rubygems
  tsuku create ruff --from pypi
  tsuku create prettier --from npm
  tsuku create gh --from github:cli/cli
  tsuku create age --from github:FiloSottile/age`,
	Args: cobra.ExactArgs(1),
	Run:  runCreate,
}

var (
	createFrom           string
	createForce          bool
	createAutoApprove    bool
	createSkipValidation bool
)

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "Source: ecosystem name or github:owner/repo (required)")
	createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
	createCmd.Flags().BoolVar(&createAutoApprove, "yes", false, "Skip recipe preview confirmation")
	createCmd.Flags().BoolVar(&createSkipValidation, "skip-validation", false, "Skip container validation (use when Docker is unavailable)")
	_ = createCmd.MarkFlagRequired("from")
}

// formatWaitTime returns a human-readable string for how long until the rate limit resets.
// It calculates the time until the oldest generation timestamp expires (1 hour window).
func formatWaitTime(sm *install.StateManager) string {
	state, err := sm.Load()
	if err != nil || state.LLMUsage == nil || len(state.LLMUsage.GenerationTimestamps) == 0 {
		return "unknown"
	}

	// Find the oldest timestamp in the rolling window
	now := time.Now().UTC()
	oneHourAgo := now.Add(-time.Hour)
	var oldest time.Time

	for _, ts := range state.LLMUsage.GenerationTimestamps {
		if ts.After(oneHourAgo) {
			if oldest.IsZero() || ts.Before(oldest) {
				oldest = ts
			}
		}
	}

	if oldest.IsZero() {
		return "unknown"
	}

	// Time until oldest expires = oldest + 1 hour - now
	expiresAt := oldest.Add(time.Hour)
	wait := expiresAt.Sub(now)

	if wait <= 0 {
		return "less than a minute"
	}

	minutes := int(wait.Minutes())
	if minutes < 1 {
		return "less than a minute"
	}
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// confirmSkipValidation prompts the user to confirm skipping validation.
// Returns true if the user consents, false otherwise.
func confirmSkipValidation() bool {
	// Check if running interactively
	if !isInteractive() {
		fmt.Fprintln(os.Stderr, "Error: --skip-validation requires interactive mode")
		fmt.Fprintln(os.Stderr, "Cannot prompt for consent when stdin is not a terminal")
		return false
	}

	fmt.Fprintln(os.Stderr, "WARNING: Skipping validation. The recipe has NOT been tested.")
	fmt.Fprintln(os.Stderr, "Risks: Binary path errors, missing extraction steps, failed verification")
	fmt.Fprint(os.Stderr, "Continue without validation? (y/N) ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// parseFromFlag parses the --from flag value.
// Returns (builder, sourceArg, isGitHub).
// For ecosystem builders: ("crates.io", "", false)
// For github builder: ("github", "cli/cli", true)
func parseFromFlag(from string) (builder string, sourceArg string, isGitHub bool) {
	// Check for github:owner/repo format
	if strings.HasPrefix(strings.ToLower(from), "github:") {
		return "github", from[7:], true
	}
	// Otherwise, it's an ecosystem name
	return normalizeEcosystem(from), "", false
}

// normalizeEcosystem converts user-friendly ecosystem names to internal identifiers
func normalizeEcosystem(name string) string {
	// Map common variations to internal names
	normalized := strings.ToLower(name)
	switch normalized {
	case "crates.io", "crates_io", "crates", "cargo":
		return "crates.io"
	case "rubygems", "rubygems.org", "gems", "gem":
		return "rubygems"
	case "pypi", "pypi.org", "pip", "python":
		return "pypi"
	case "npm", "npmjs", "npmjs.com", "node", "nodejs":
		return "npm"
	default:
		return normalized
	}
}

func runCreate(cmd *cobra.Command, args []string) {
	toolName := args[0]

	// Warn if skipping review
	if createAutoApprove {
		fmt.Fprintln(os.Stderr, "Warning: Skipping recipe review (--yes). The recipe will be installed without confirmation.")
	}

	// Parse the --from flag
	builderName, sourceArg, isGitHub := parseFromFlag(createFrom)

	// Handle --skip-validation flag (only applies to GitHub builder)
	skipValidation := false
	if createSkipValidation {
		if !isGitHub {
			fmt.Fprintln(os.Stderr, "Warning: --skip-validation has no effect for non-GitHub sources")
		} else {
			// Require explicit consent for skipping validation
			if !confirmSkipValidation() {
				fmt.Fprintln(os.Stderr, "Aborted.")
				exitWithCode(ExitGeneral)
			}
			skipValidation = true
		}
	}

	// Initialize builder registry
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))
	builderRegistry.Register(builders.NewPyPIBuilder(nil))
	builderRegistry.Register(builders.NewNpmBuilder(nil))

	// For GitHub builder, check LLM budget and rate limits before proceeding
	var stateManager *install.StateManager
	if isGitHub {
		// Load user config for settings
		userCfg, err := userconfig.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load user config: %v\n", err)
			// Continue with defaults
			userCfg = userconfig.DefaultConfig()
		}

		// Check if LLM is enabled
		if !userCfg.LLMEnabled() {
			fmt.Fprintln(os.Stderr, "Error: LLM features are disabled")
			fmt.Fprintln(os.Stderr, "To enable: tsuku config set llm.enabled true")
			exitWithCode(ExitGeneral)
		}

		// Initialize state manager
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config: %v\n", err)
			exitWithCode(ExitGeneral)
		}
		stateManager = install.NewStateManager(cfg)

		// Get limit settings
		hourlyLimit := userCfg.LLMHourlyRateLimit()
		dailyBudget := userCfg.LLMDailyBudget()

		// Check budget and rate limits
		allowed, reason := stateManager.CanGenerate(hourlyLimit, dailyBudget)
		if !allowed {
			// Format error message based on the reason
			if strings.Contains(reason, "budget") {
				spent := stateManager.DailySpent()
				fmt.Fprintf(os.Stderr, "Error: daily LLM budget exhausted ($%.2f spent today)\n", spent)
				fmt.Fprintln(os.Stderr, "Budget resets at midnight. To adjust: tsuku config set llm.daily_budget 10.0")
			} else {
				fmt.Fprintf(os.Stderr, "Error: %s\n", reason)
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, "To increase the limit:")
				fmt.Fprintln(os.Stderr, "  tsuku config set llm.hourly_rate_limit 20")
				fmt.Fprintln(os.Stderr)
				fmt.Fprintf(os.Stderr, "Wait time: %s\n", formatWaitTime(stateManager))
			}
			exitWithCode(ExitGeneral)
		}
	}

	// Register GitHub builder (may fail if ANTHROPIC_API_KEY not set)
	if isGitHub {
		var opts []builders.GitHubReleaseBuilderOption

		// Set up validation executor unless --skip-validation was confirmed
		if !skipValidation {
			detector := validate.NewRuntimeDetector()
			predownloader := validate.NewPreDownloader()
			executor := validate.NewExecutor(detector, predownloader)
			opts = append(opts, builders.WithExecutor(executor))
		}

		ghBuilder, err := builders.NewGitHubReleaseBuilder(context.Background(), opts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitWithCode(ExitDependencyFailed)
		}
		builderRegistry.Register(ghBuilder)
	}

	// Get the builder
	builder, ok := builderRegistry.Get(builderName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown source '%s'\n", createFrom)
		fmt.Fprintf(os.Stderr, "\nAvailable sources:\n")
		for _, name := range builderRegistry.List() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  github:owner/repo\n")
		exitWithCode(ExitUsage)
	}

	ctx := context.Background()

	// For ecosystem builders, check toolchain and package existence
	if !isGitHub {
		// Check toolchain availability before making API calls
		if err := toolchain.CheckAvailable(builderName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitWithCode(ExitDependencyFailed)
		}

		// Check if builder can handle this package
		canBuild, err := builder.CanBuild(ctx, toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking package: %v\n", err)
			exitWithCode(ExitNetwork)
		}
		if !canBuild {
			fmt.Fprintf(os.Stderr, "Error: package '%s' not found in %s\n", toolName, builderName)
			exitWithCode(ExitRecipeNotFound)
		}
	}

	// Build the recipe
	var sourceDisplay string
	if isGitHub {
		sourceDisplay = fmt.Sprintf("github:%s", sourceArg)
	} else {
		sourceDisplay = builderName
	}
	printInfof("Creating recipe for %s from %s...\n", toolName, sourceDisplay)

	result, err := builder.Build(ctx, builders.BuildRequest{
		Package:   toolName,
		SourceArg: sourceArg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building recipe: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Record LLM usage for successful GitHub builds (includes cost tracking)
	if isGitHub && stateManager != nil {
		if err := stateManager.RecordGeneration(defaultLLMCostEstimate); err != nil {
			// Non-fatal: log warning but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to record LLM usage: %v\n", err)
		}
	}

	// Show warnings (printed to stderr)
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}

	// Add llm_validation metadata if validation was skipped
	if result.ValidationSkipped {
		result.Recipe.Metadata.LLMValidation = "skipped"
	}

	// Get recipes directory
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	recipePath := filepath.Join(cfg.RecipesDir, toolName+".toml")

	// Check if recipe already exists
	if _, err := os.Stat(recipePath); err == nil && !createForce {
		fmt.Fprintf(os.Stderr, "Error: recipe already exists at %s\n", recipePath)
		fmt.Fprintf(os.Stderr, "Use --force to overwrite\n")
		exitWithCode(ExitGeneral)
	}

	// Write the recipe
	if err := recipe.WriteRecipe(result.Recipe, recipePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing recipe: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	printInfof("\nRecipe created: %s\n", recipePath)
	printInfof("Source: %s\n", result.Source)

	// Show validation skipped warning
	if result.ValidationSkipped {
		printInfo()
		fmt.Fprintln(os.Stderr, "WARNING: Recipe was NOT validated in a container.")
		fmt.Fprintln(os.Stderr, "The recipe may have errors. Review before installing.")
	}

	printInfo()
	printInfo("To install, run:")
	printInfof("  tsuku install %s\n", toolName)
}
