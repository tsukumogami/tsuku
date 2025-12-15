package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/toolchain"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

const (
	// defaultLLMCostEstimate is the estimated cost per LLM generation in USD.
	// This is a conservative estimate based on typical token usage.
	defaultLLMCostEstimate = 0.10
)

// cliProgressReporter implements builders.ProgressReporter for CLI output.
type cliProgressReporter struct {
	out io.Writer
}

func (r *cliProgressReporter) OnStageStart(stage string) {
	_, _ = fmt.Fprintf(r.out, "%s... ", stage)
}

func (r *cliProgressReporter) OnStageDone(detail string) {
	if detail != "" {
		_, _ = fmt.Fprintf(r.out, "done (%s)\n", detail)
	} else {
		_, _ = fmt.Fprintln(r.out, "done")
	}
}

func (r *cliProgressReporter) OnStageFailed() {
	_, _ = fmt.Fprintln(r.out, "failed")
}

var createCmd = &cobra.Command{
	Use:   "create <tool> --from <source>",
	Short: "Create a recipe from a package ecosystem, GitHub, or Homebrew",
	Long: `Create a recipe by querying a package ecosystem's metadata API, analyzing
GitHub release assets, or inspecting Homebrew bottles.

The generated recipe is written to $TSUKU_HOME/recipes/<tool>.toml and can be
inspected or edited before running 'tsuku install <tool>'.

Supported sources:
  crates.io           Rust crates from crates.io
  rubygems            Ruby gems from rubygems.org
  pypi                Python packages from pypi.org
  npm                 Node.js packages from npmjs.com
  github:owner/repo      GitHub releases (uses LLM to analyze assets)
  homebrew:formula       Homebrew formulas (uses LLM to generate recipes)
  homebrew:formula:source  Force source build even if bottles available

Examples:
  tsuku create ripgrep --from crates.io
  tsuku create bat --from crates.io --force
  tsuku create jekyll --from rubygems
  tsuku create ruff --from pypi
  tsuku create prettier --from npm
  tsuku create gh --from github:cli/cli
  tsuku create age --from github:FiloSottile/age
  tsuku create jq --from homebrew:jq
  tsuku create ripgrep --from homebrew:ripgrep`,
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

// confirmWithUser prompts the user with a message and waits for y/N response.
func confirmWithUser(prompt string) bool {
	if !isInteractive() {
		return false
	}

	fmt.Fprintf(os.Stderr, "%s (y/N) ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

// parseFromFlag parses the --from flag value.
// Returns (builder, remainder).
// Splits on the first ":" - builder name before, remainder after.
// Examples:
//   - "homebrew:jq:source" → ("homebrew", "jq:source")
//   - "github:cli/cli" → ("github", "cli/cli")
//   - "crates.io" → ("crates.io", "")
func parseFromFlag(from string) (builder string, remainder string) {
	if idx := strings.Index(from, ":"); idx != -1 {
		return strings.ToLower(from[:idx]), from[idx+1:]
	}
	return strings.ToLower(from), ""
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
	builderName, sourceArg := parseFromFlag(createFrom)

	// Normalize ecosystem names (e.g., "cargo" -> "crates.io", "pip" -> "pypi")
	builderName = normalizeEcosystem(builderName)

	// Handle --skip-validation flag
	skipValidation := false
	if createSkipValidation {
		// Require explicit consent for skipping validation
		if !confirmSkipValidation() {
			fmt.Fprintln(os.Stderr, "Aborted.")
			exitWithCode(ExitGeneral)
		}
		skipValidation = true
	}

	// Initialize builder registry with all builders
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))
	builderRegistry.Register(builders.NewPyPIBuilder(nil))
	builderRegistry.Register(builders.NewNpmBuilder(nil))
	builderRegistry.Register(builders.NewGitHubReleaseBuilder())
	builderRegistry.Register(builders.NewHomebrewBuilder())

	// Get the builder
	builder, ok := builderRegistry.Get(builderName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown source '%s'\n", createFrom)
		fmt.Fprintf(os.Stderr, "\nAvailable sources:\n")
		for _, name := range builderRegistry.List() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  github:owner/repo\n")
		fmt.Fprintf(os.Stderr, "  homebrew:formula\n")
		exitWithCode(ExitUsage)
	}

	ctx := context.Background()

	// For LLM builders, load config and state tracker for session options
	var stateManager *install.StateManager
	var userCfg *userconfig.Config
	if builder.RequiresLLM() {
		// Load user config for LLM settings
		var err error
		userCfg, err = userconfig.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load user config: %v\n", err)
			userCfg = userconfig.DefaultConfig()
		}

		// Initialize state manager for rate limit tracking
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting config: %v\n", err)
			exitWithCode(ExitGeneral)
		}
		stateManager = install.NewStateManager(cfg)
	}

	// Build session options
	var progressReporter builders.ProgressReporter
	if !quietFlag {
		progressReporter = &cliProgressReporter{out: os.Stdout}
	}

	sessionOpts := &builders.SessionOptions{
		ProgressReporter: progressReporter,
		LLMConfig:        userCfg,
		LLMStateTracker:  stateManager,
	}

	// Set up force init flag for later use
	forceInit := false
	_ = forceInit      // will be used when creating session
	_ = skipValidation // TODO: pass to orchestrator when validation is implemented

	// Build request for use throughout
	buildReq := builders.BuildRequest{
		Package:   toolName,
		SourceArg: sourceArg,
	}

	// For ecosystem builders, check toolchain and package existence
	if !builder.RequiresLLM() {
		// Check toolchain availability before making API calls
		if err := toolchain.CheckAvailable(builderName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitWithCode(ExitDependencyFailed)
		}

		// Check if builder can handle this package
		canBuild, err := builder.CanBuild(ctx, buildReq)
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
	if sourceArg != "" {
		sourceDisplay = fmt.Sprintf("%s:%s", builderName, sourceArg)
	} else {
		sourceDisplay = builderName
	}
	printInfof("Creating recipe for %s from %s...\n", toolName, sourceDisplay)

	// Get recipes directory early (needed for both paths)
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Create a session and generate the recipe
	session, err := builder.NewSession(ctx, buildReq, sessionOpts)
	if err != nil {
		// Check if this is a confirmable error (rate limit, budget exceeded)
		if confirmErr, ok := err.(builders.ConfirmableError); ok {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			if confirmWithUser(confirmErr.ConfirmationPrompt()) {
				// Retry with ForceInit to bypass rate limit checks
				sessionOpts.ForceInit = true
				session, err = builder.NewSession(ctx, buildReq, sessionOpts)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					exitWithCode(ExitDependencyFailed)
				}
			} else {
				exitWithCode(ExitGeneral)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitWithCode(ExitDependencyFailed)
		}
	}
	defer session.Close()

	// Generate the recipe
	result, err := session.Generate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building recipe: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Add llm_validation metadata if validation was skipped
	// (cost recording is handled by builder internally)
	if result.ValidationSkipped {
		result.Recipe.Metadata.LLMValidation = "skipped"
	}

	recipePath := filepath.Join(cfg.RecipesDir, toolName+".toml")

	// Check if recipe already exists
	if _, err := os.Stat(recipePath); err == nil && !createForce {
		fmt.Fprintf(os.Stderr, "Error: recipe already exists at %s\n", recipePath)
		fmt.Fprintf(os.Stderr, "Use --force to overwrite\n")
		exitWithCode(ExitGeneral)
	}

	// For LLM builders, show preview and prompt for approval (unless --yes)
	if builder.RequiresLLM() && !createAutoApprove {
		fmt.Println() // Blank line after "Creating recipe..." message
		approved, err := previewRecipe(result.Recipe, result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			exitWithCode(ExitGeneral)
		}
		if !approved {
			printInfo("Canceled.")
			return
		}
	} else {
		// For ecosystem builders or when --yes is used, show warnings (printed to stderr)
		for _, warning := range result.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
		}
	}

	// Write the recipe
	if err := recipe.WriteRecipe(result.Recipe, recipePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing recipe: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	printInfof("\nRecipe created: %s\n", recipePath)
	printInfof("Source: %s\n", result.Source)

	// Display cost for LLM builds
	if builder.RequiresLLM() && stateManager != nil && userCfg != nil {
		dailySpent := stateManager.DailySpent()
		dailyBudget := userCfg.LLMDailyBudget()

		if dailyBudget > 0 {
			// Show cost with budget context
			printInfof("Cost: ~$%.2f (today: $%.2f of $%.2f budget)\n",
				defaultLLMCostEstimate, dailySpent, dailyBudget)
		} else {
			// Unlimited budget - show without budget portion
			printInfof("Cost: ~$%.2f (today: $%.2f)\n",
				defaultLLMCostEstimate, dailySpent)
		}
	}

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

// previewRecipe displays a recipe summary and prompts for user action.
// Returns true if the user approves installation, false if canceled.
func previewRecipe(r *recipe.Recipe, result *builders.BuildResult) (bool, error) {
	fmt.Printf("Generated recipe for %s:\n\n", r.Metadata.Name)

	// Show downloads
	fmt.Println("  Downloads:")
	urls := extractDownloadURLs(r)
	if len(urls) == 0 {
		fmt.Println("    (none)")
	} else {
		for _, url := range urls {
			fmt.Printf("    - %s\n", url)
		}
	}
	fmt.Println()

	// Show actions
	fmt.Println("  Actions:")
	for i, step := range r.Steps {
		fmt.Printf("    %d. %s\n", i+1, describeStep(step))
	}
	fmt.Println()

	// Show verification
	if r.Verify.Command != "" {
		fmt.Printf("  Verification: %s\n", r.Verify.Command)
		fmt.Println()
	}

	// Show LLM info if available
	if result.Provider != "" {
		fmt.Printf("  LLM: %s (cost: $%.4f)\n", result.Provider, result.Cost)
	}

	// Show repair attempts if any
	if result.RepairAttempts > 0 {
		fmt.Printf("  Note: Recipe required %d repair attempt(s)\n", result.RepairAttempts)
	}

	// Show validation status
	if result.ValidationSkipped {
		fmt.Println("  Warning: Validation was skipped")
	}

	// Show other warnings
	for _, warning := range result.Warnings {
		// Skip LLM usage warning as we show cost separately
		if !strings.HasPrefix(warning, "LLM usage:") {
			fmt.Printf("  Warning: %s\n", warning)
		}
	}
	fmt.Println()

	return promptForApproval(r)
}

// promptForApproval handles the interactive prompt loop.
// Returns true if user chooses to install, false if canceled.
func promptForApproval(r *recipe.Recipe) (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("[v]iew full recipe, [i]nstall, [c]ancel? ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "v", "view":
			// Show full TOML and re-prompt
			tomlStr, err := formatRecipeTOML(r)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error formatting recipe: %v\n", err)
			} else {
				fmt.Println()
				fmt.Println(tomlStr)
				fmt.Println()
			}
		case "i", "install":
			return true, nil
		case "c", "cancel", "":
			return false, nil
		default:
			fmt.Println("Invalid input. Please enter 'v', 'i', or 'c'.")
		}
	}
}

// confirmDependencyTree displays the dependency tree and asks for confirmation.
// Returns true if the user approves, false if canceled.
// If autoApprove is true, shows the tree but skips the prompt.
func confirmDependencyTree(req *builders.ConfirmationRequest, autoApprove bool) bool {
	fmt.Println()
	fmt.Println("Dependency tree:")
	fmt.Println(req.FormattedTree)
	fmt.Println()

	if len(req.AlreadyHave) > 0 {
		fmt.Printf("Already have recipes for: %s\n", strings.Join(req.AlreadyHave, ", "))
	}

	if len(req.ToGenerate) == 0 {
		fmt.Println("All recipes already exist. Nothing to generate.")
		return true
	}

	fmt.Printf("Will generate %d recipe(s): %s\n", len(req.ToGenerate), strings.Join(req.ToGenerate, ", "))
	fmt.Printf("Estimated cost: $%.2f\n", req.EstimatedCost)
	fmt.Println()

	if autoApprove {
		return true
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Proceed? [y/n] ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// extractDownloadURLs returns download URLs from the recipe.
func extractDownloadURLs(r *recipe.Recipe) []string {
	var urls []string
	for _, step := range r.Steps {
		switch step.Action {
		case "github_archive", "github_file":
			if repo, ok := step.Params["repo"].(string); ok {
				if pattern, ok := step.Params["asset_pattern"].(string); ok {
					urls = append(urls, fmt.Sprintf("github.com/%s/releases/.../%s", repo, pattern))
				}
			}
		case "homebrew_bottle":
			if formula, ok := step.Params["formula"].(string); ok {
				urls = append(urls, fmt.Sprintf("ghcr.io/homebrew/core/%s:...", formula))
			}
		case "download", "download_archive":
			if url, ok := step.Params["url"].(string); ok {
				urls = append(urls, url)
			}
		case "hashicorp_release":
			if product, ok := step.Params["product"].(string); ok {
				urls = append(urls, fmt.Sprintf("releases.hashicorp.com/%s/...", product))
			}
		}
	}
	return urls
}

// describeStep returns a human-readable description of a recipe step.
func describeStep(step recipe.Step) string {
	switch step.Action {
	case "github_archive":
		format := "tar.gz"
		if f, ok := step.Params["archive_format"].(string); ok {
			format = f
		}
		return fmt.Sprintf("Download and extract %s archive from GitHub", format)
	case "github_file":
		return "Download binary from GitHub releases"
	case "homebrew_bottle":
		if formula, ok := step.Params["formula"].(string); ok {
			return fmt.Sprintf("Download Homebrew bottle for %s", formula)
		}
		return "Download Homebrew bottle"
	case "download":
		return "Download file"
	case "download_archive":
		return "Download and extract archive"
	case "hashicorp_release":
		return "Download from HashiCorp releases"
	case "chmod":
		return "Set file permissions"
	case "npm_install":
		return "Install via npm"
	case "pip_install":
		return "Install via pip"
	case "gem_install":
		return "Install via gem"
	case "cargo_install":
		return "Install via cargo"
	case "run":
		if cmd, ok := step.Params["command"].(string); ok {
			// Truncate long commands
			if len(cmd) > 40 {
				cmd = cmd[:37] + "..."
			}
			return fmt.Sprintf("Run: %s", cmd)
		}
		return "Run command"
	default:
		if step.Description != "" {
			return step.Description
		}
		return step.Action
	}
}

// formatRecipeTOML returns the recipe as a TOML string.
func formatRecipeTOML(r *recipe.Recipe) (string, error) {
	// Use the same encoding structure as recipe.WriteRecipe
	type recipeForEncoding struct {
		Metadata recipe.MetadataSection   `toml:"metadata"`
		Version  recipe.VersionSection    `toml:"version"`
		Steps    []map[string]interface{} `toml:"steps"`
		Verify   recipe.VerifySection     `toml:"verify"`
	}

	steps := make([]map[string]interface{}, len(r.Steps))
	for i, step := range r.Steps {
		steps[i] = step.ToMap()
	}

	encodable := &recipeForEncoding{
		Metadata: r.Metadata,
		Version:  r.Version,
		Steps:    steps,
		Verify:   r.Verify,
	}

	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(encodable); err != nil {
		return "", err
	}
	return buf.String(), nil
}
