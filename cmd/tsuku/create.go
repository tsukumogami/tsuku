package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
	"github.com/tsukumogami/tsuku/internal/toolchain"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/validate"
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
  go:module           Go modules from proxy.golang.org
  cpan                Perl distributions from metacpan.org
  github:owner/repo      GitHub releases (uses LLM to analyze assets)
  homebrew:formula       Homebrew formulas (uses LLM to generate recipes)
  homebrew:formula:source  Force source build even if bottles available
  cask:name              Homebrew Casks (macOS applications)

Examples:
  tsuku create ripgrep --from crates.io
  tsuku create bat --from crates.io --force
  tsuku create jekyll --from rubygems
  tsuku create ruff --from pypi
  tsuku create prettier --from npm
  tsuku create gh --from github:cli/cli
  tsuku create age --from github:FiloSottile/age
  tsuku create lazygit --from go:github.com/jesseduffield/lazygit
  tsuku create ack --from cpan
  tsuku create jq --from homebrew:jq
  tsuku create ripgrep --from homebrew:ripgrep
  tsuku create vscode --from cask:visual-studio-code
  tsuku create iterm2 --from cask:iterm2`,
	Args: cobra.ExactArgs(1),
	Run:  runCreate,
}

var (
	createFrom              string
	createForce             bool
	createAutoApprove       bool
	createSkipSandbox       bool
	createDeterministicOnly bool
	createOutput            string
)

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "Source: ecosystem name or github:owner/repo (required)")
	createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
	createCmd.Flags().BoolVar(&createAutoApprove, "yes", false, "Skip recipe preview confirmation")
	createCmd.Flags().BoolVar(&createSkipSandbox, "skip-sandbox", false, "Skip container sandbox testing (use when Docker is unavailable)")
	createCmd.Flags().BoolVar(&createDeterministicOnly, "deterministic-only", false, "Skip LLM fallback; exit with structured error if deterministic generation fails")
	createCmd.Flags().StringVar(&createOutput, "output", "", "Write recipe to this path instead of the default registry location")
}

// confirmSkipSandbox prompts the user to confirm skipping sandbox testing.
// Returns true if the user consents, false otherwise.
func confirmSkipSandbox() bool {
	// Check if running interactively
	if !isInteractive() {
		fmt.Fprintln(os.Stderr, "Error: --skip-sandbox requires interactive mode")
		fmt.Fprintln(os.Stderr, "Cannot prompt for consent when stdin is not a terminal")
		return false
	}

	fmt.Fprintln(os.Stderr, "WARNING: Skipping sandbox testing. The recipe has NOT been tested.")
	fmt.Fprintln(os.Stderr, "Risks: Binary path errors, missing extraction steps, failed verification")
	fmt.Fprint(os.Stderr, "Continue without sandbox testing? (y/N) ")

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

// offerToolchainInstall prompts the user to install a missing toolchain (or
// auto-installs with --yes). Returns true if the install succeeded.
func offerToolchainInstall(info *toolchain.Info, ecosystem string, autoApprove bool) bool {
	fmt.Fprintf(os.Stderr, "%s requires %s, which is not installed.\n", ecosystem, info.Name)

	if autoApprove {
		fmt.Fprintf(os.Stderr, "Installing %s (required toolchain)...\n", info.TsukuRecipe)
	} else {
		if !confirmWithUser(fmt.Sprintf("Install %s using tsuku?", info.TsukuRecipe)) {
			return false
		}
	}

	if err := runInstallWithTelemetry(info.TsukuRecipe, "", "", false, "create", nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to install required toolchain '%s': %v\n", info.TsukuRecipe, err)
		return false
	}
	fmt.Fprintf(os.Stderr, "%s installed successfully.\n", info.TsukuRecipe)
	return true
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
	case "go", "golang", "goproxy":
		return "go"
	case "cpan", "metacpan", "perl":
		return "cpan"
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

	// Initialize builder registry with all builders
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))
	builderRegistry.Register(builders.NewPyPIBuilder(nil))
	builderRegistry.Register(builders.NewNpmBuilder(nil))
	builderRegistry.Register(builders.NewGoBuilder(nil))
	builderRegistry.Register(builders.NewCPANBuilder(nil))
	builderRegistry.Register(builders.NewGitHubReleaseBuilder())
	builderRegistry.Register(builders.NewHomebrewBuilder())
	builderRegistry.Register(builders.NewCaskBuilder(nil))

	var builderName, sourceArg string
	if createFrom != "" {
		// Explicit --from flag: parse and normalize as before.
		builderName, sourceArg = parseFromFlag(createFrom)
		builderName = normalizeEcosystem(builderName)
	} else {
		// No --from flag: run discovery to find the builder and source.
		result, err := runDiscovery(toolName)
		if err != nil {
			printError(err)
			exitWithCode(ExitRecipeNotFound)
		}
		builderName = result.Builder
		sourceArg = result.Source
		fmt.Fprintf(os.Stderr, "Discovered: %s\n", result.Reason)
	}

	// Handle --skip-sandbox flag
	skipSandbox := false
	if createSkipSandbox {
		// --yes implies consent for skipping sandbox (needed for batch/CI)
		if !createAutoApprove {
			if !confirmSkipSandbox() {
				fmt.Fprintln(os.Stderr, "Aborted.")
				exitWithCode(ExitGeneral)
			}
		}
		skipSandbox = true
	}

	// Get the builder
	builder, ok := builderRegistry.Get(builderName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown source '%s'\n", builderName)
		fmt.Fprintf(os.Stderr, "\nAvailable sources:\n")
		for _, name := range builderRegistry.List() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintf(os.Stderr, "  github:owner/repo\n")
		fmt.Fprintf(os.Stderr, "  homebrew:formula\n")
		fmt.Fprintf(os.Stderr, "  cask:name\n")
		exitWithCode(ExitUsage)
	}

	// Guard: fail early if --deterministic-only is set and the builder needs LLM
	if createDeterministicOnly && builder.RequiresLLM() {
		source := sourceArg
		if source == "" {
			source = builderName
		}
		fmt.Fprintf(os.Stderr, "Error: '%s' resolved to %s (%s), which requires LLM for recipe generation.\n",
			toolName, builderName, source)
		fmt.Fprintln(os.Stderr, "Remove --deterministic-only or wait for a recipe to be contributed.")
		exitWithCode(ExitDeterministicFailed)
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
		ProgressReporter:  progressReporter,
		LLMConfig:         userCfg,
		LLMStateTracker:   stateManager,
		DeterministicOnly: createDeterministicOnly,
	}

	// Build request for use throughout
	buildReq := builders.BuildRequest{
		Package:   toolName,
		SourceArg: sourceArg,
	}

	// For ecosystem builders, check toolchain and package existence
	if !builder.RequiresLLM() {
		// Check toolchain availability before making API calls
		if err := toolchain.CheckAvailable(builderName); err != nil {
			info := toolchain.GetInfo(builderName)
			if info == nil || info.TsukuRecipe == "" {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				exitWithCode(ExitDependencyFailed)
			}
			if !offerToolchainInstall(info, builderName, createAutoApprove) {
				exitWithCode(ExitDependencyFailed)
			}
			// After install, add $TSUKU_HOME/tools/current to PATH so the
			// re-check and subsequent builder commands can find the new binary.
			cfg, cfgErr := config.DefaultConfig()
			if cfgErr == nil {
				os.Setenv("PATH", cfg.CurrentDir+string(filepath.ListSeparator)+os.Getenv("PATH"))
			}
			if err := toolchain.CheckAvailable(builderName); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s was installed but '%s' is still not on PATH.\n", info.TsukuRecipe, info.Binary)
				fmt.Fprintf(os.Stderr, "Try running: eval $(tsuku shellenv)\n")
				exitWithCode(ExitDependencyFailed)
			}
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

	effectiveSkipSandbox := skipSandbox

	// Create sandbox executor (if not skipping sandbox)
	var sandboxExec *sandbox.Executor
	if !effectiveSkipSandbox {
		// Ensure cache directories exist (needed for mounting into container)
		if err := cfg.EnsureDirectories(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
			exitWithCode(ExitGeneral)
		}
		detector := validate.NewRuntimeDetector()
		sandboxExec = sandbox.NewExecutor(detector,
			sandbox.WithDownloadCacheDir(cfg.DownloadCacheDir))
	}

	// Create orchestrator for generate → sandbox → repair cycle
	orchestrator := builders.NewOrchestrator(
		builders.WithSandboxExecutor(sandboxExec),
		builders.WithOrchestratorConfig(builders.OrchestratorConfig{
			SkipSandbox:      effectiveSkipSandbox,
			MaxRepairs:       builders.DefaultMaxRepairs,
			ToolsDir:         cfg.ToolsDir,
			LibsDir:          cfg.LibsDir,
			DownloadCacheDir: cfg.DownloadCacheDir,
		}),
	)

	// Generate recipe using orchestrator (handles sandbox validation and repair)
	orchResult, err := orchestrator.Create(ctx, builder, buildReq, sessionOpts)
	if err != nil {
		// Handle DeterministicFailedError (--deterministic-only mode)
		var detErr *builders.DeterministicFailedError
		if errors.As(err, &detErr) {
			fmt.Fprintf(os.Stderr, "deterministic generation failed: [%s] %s\n",
				detErr.Category, detErr.Message)
			exitWithCode(ExitDeterministicFailed)
		}

		// Handle ValidationFailedError with detailed output
		var valErr *builders.ValidationFailedError
		if errors.As(err, &valErr) {
			fmt.Fprintln(os.Stderr, "Error: recipe validation failed in sandbox")
			fmt.Fprintf(os.Stderr, "Exit code: %d\n", valErr.SandboxResult.ExitCode)
			if valErr.RepairAttempts > 0 {
				fmt.Fprintf(os.Stderr, "Repair attempts: %d\n", valErr.RepairAttempts)
			}
			if valErr.SandboxResult.Stderr != "" {
				fmt.Fprintln(os.Stderr, "\nError output:")
				fmt.Fprintln(os.Stderr, valErr.SandboxResult.Stderr)
			}
			if valErr.SandboxResult.Stdout != "" {
				fmt.Fprintln(os.Stderr, "\nContainer output:")
				fmt.Fprintln(os.Stderr, valErr.SandboxResult.Stdout)
			}
			exitWithCode(ExitGeneral)
		}

		// Check if this is a confirmable error (rate limit, budget exceeded)
		if confirmErr, ok := err.(builders.ConfirmableError); ok {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			if confirmWithUser(confirmErr.ConfirmationPrompt()) {
				// Retry with ForceInit to bypass rate limit checks
				sessionOpts.ForceInit = true
				orchResult, err = orchestrator.Create(ctx, builder, buildReq, sessionOpts)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					exitWithCode(ExitDependencyFailed)
				}
			} else {
				exitWithCode(ExitGeneral)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error building recipe: %v\n", err)
			exitWithCode(ExitGeneral)
		}
	}

	result := orchResult.BuildResult

	// Add llm_validation metadata if sandbox testing was skipped
	// (cost recording is handled by builder internally)
	if orchResult.SandboxSkipped {
		result.Recipe.Metadata.LLMValidation = "skipped"
	}

	recipePath := filepath.Join(cfg.RecipesDir, toolName+".toml")
	if createOutput != "" {
		recipePath = createOutput
	}

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

	// Show sandbox skipped warning
	if orchResult.SandboxSkipped {
		printInfo()
		fmt.Fprintln(os.Stderr, "WARNING: Recipe was NOT tested in a sandbox container.")
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

	// Show sandbox status
	if result.SandboxSkipped {
		fmt.Println("  Warning: Sandbox testing was skipped")
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
		case "homebrew":
			if formula, ok := step.Params["formula"].(string); ok {
				urls = append(urls, fmt.Sprintf("ghcr.io/homebrew/core/%s:...", formula))
			}
		case "download", "download_archive":
			if url, ok := step.Params["url"].(string); ok {
				urls = append(urls, url)
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
	case "homebrew":
		if formula, ok := step.Params["formula"].(string); ok {
			return fmt.Sprintf("Download Homebrew bottle for %s", formula)
		}
		return "Download Homebrew bottle"
	case "download":
		return "Download file"
	case "download_archive":
		return "Download and extract archive"
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

// runDiscovery resolves a tool name to a builder and source using the
// discovery resolver chain. Currently only the registry lookup stage is
// active; ecosystem probe and LLM stages are stubs.
func runDiscovery(toolName string) (*discover.DiscoveryResult, error) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	discoveryDir := filepath.Join(cfg.RegistryDir, "discovery")

	var stages []discover.Resolver

	// Stage 1: Registry lookup (if the directory exists).
	if info, err := os.Stat(discoveryDir); err == nil && info.IsDir() {
		lookup, err := discover.NewRegistryLookup(discoveryDir)
		if err == nil {
			stages = append(stages, lookup)
		}
	}

	// Stage 2: Ecosystem probe — queries package registries in parallel.
	var probers []builders.EcosystemProber
	for _, b := range []builders.SessionBuilder{
		builders.NewCargoBuilder(nil),
		builders.NewPyPIBuilder(nil),
		builders.NewNpmBuilder(nil),
		builders.NewGemBuilder(nil),
		builders.NewGoBuilder(nil),
		builders.NewCPANBuilder(nil),
		builders.NewCaskBuilder(nil),
		builders.NewHomebrewBuilder(),
	} {
		if p, ok := b.(builders.EcosystemProber); ok {
			probers = append(probers, p)
		}
	}
	stages = append(stages, discover.NewEcosystemProbe(probers, 3*time.Second))

	// Stage 3: LLM discovery (stub — always misses).
	stages = append(stages, &discover.LLMDiscovery{})

	chain := discover.NewChainResolver(stages...)
	return chain.Resolve(globalCtx, toolName)
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
