package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// Platform flag validation whitelists per DESIGN-installation-plans-eval.md
// Uses tsuku's supported platforms as the source of truth
var validOSValues = makeSet(recipe.TsukuSupportedOS())
var validArchValues = makeSet(recipe.TsukuSupportedArch())

// makeSet converts a slice to a map[string]bool for O(1) lookups
func makeSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

var evalOS string
var evalArch string
var evalYes bool
var evalRecipePath string

var evalCmd = &cobra.Command{
	Use:   "eval <tool>[@version]",
	Short: "Generate an installation plan for a tool",
	Long: `Generate a deterministic installation plan for a tool and output it as JSON.

The plan captures fully-resolved URLs, computed checksums, and all steps
needed to reproduce the installation. This enables:
  - Recipe testing via JSON comparison
  - Audit trails of what would be downloaded
  - Cross-platform plan generation

By default, plans are generated for the current platform. Use --os and --arch
to generate plans for other platforms.

Some tools require dependencies at eval time (e.g., npm packages need nodejs
to generate package-lock.json). If these dependencies are missing, you will
be prompted to install them. Use --yes to auto-accept.

Use --recipe to evaluate a local recipe file:
  tsuku eval --recipe ./my-recipe.toml
  tsuku eval --recipe /path/to/recipe.toml --os darwin --arch arm64

Examples:
  tsuku eval kubectl
  tsuku eval kubectl@v1.29.0
  tsuku eval ripgrep --os linux --arch arm64
  tsuku eval netlify-cli --yes
  tsuku eval --recipe ./my-recipe.toml --os darwin --arch arm64`,
	Args: cobra.MaximumNArgs(1),
	Run:  runEval,
}

func init() {
	evalCmd.Flags().StringVar(&evalOS, "os", "", "Target operating system (linux, darwin)")
	evalCmd.Flags().StringVar(&evalArch, "arch", "", "Target architecture (amd64, arm64)")
	evalCmd.Flags().BoolVar(&evalYes, "yes", false, "Auto-accept installation of eval-time dependencies")
	evalCmd.Flags().StringVar(&evalRecipePath, "recipe", "", "Path to a local recipe file (for testing)")
}

// ValidateOS validates an OS value against the whitelist.
// Returns an error if the value is invalid.
func ValidateOS(os string) error {
	if os == "" {
		return nil // Empty is valid (uses runtime default)
	}
	if !validOSValues[os] {
		return fmt.Errorf("invalid OS value %q: must be one of linux, darwin", os)
	}
	return nil
}

// ValidateArch validates an architecture value against the whitelist.
// Returns an error if the value is invalid.
func ValidateArch(arch string) error {
	if arch == "" {
		return nil // Empty is valid (uses runtime default)
	}
	if !validArchValues[arch] {
		return fmt.Errorf("invalid arch value %q: must be one of amd64, arm64", arch)
	}
	return nil
}

func runEval(cmd *cobra.Command, args []string) {
	// Validate platform flags early
	if err := ValidateOS(evalOS); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitWithCode(ExitUsage)
	}
	if err := ValidateArch(evalArch); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitWithCode(ExitUsage)
	}

	// Validate mutually exclusive options
	if evalRecipePath != "" && len(args) > 0 {
		printError(fmt.Errorf("cannot specify both --recipe and a tool name"))
		exitWithCode(ExitUsage)
	}
	if evalRecipePath == "" && len(args) == 0 {
		printError(fmt.Errorf("requires either a tool name or --recipe flag"))
		exitWithCode(ExitUsage)
	}

	var r *recipe.Recipe
	var recipeSource string
	var reqVersion string

	if evalRecipePath != "" {
		// Recipe file mode: load from local file using shared function
		var err error
		r, err = loadLocalRecipe(evalRecipePath)
		if err != nil {
			printError(fmt.Errorf("failed to load recipe: %w", err))
			exitWithCode(ExitGeneral)
		}
		recipeSource = evalRecipePath // Use file path as source identifier
		// Note: version specification (@version) not supported with file paths
	} else {
		// Registry mode: existing behavior
		toolName := args[0]
		if strings.Contains(toolName, "@") {
			parts := strings.SplitN(toolName, "@", 2)
			toolName = parts[0]
			reqVersion = parts[1]
		}

		// Convert "latest" to empty for resolution
		if reqVersion == "latest" {
			reqVersion = ""
		}

		var err error
		r, err = loader.Get(toolName)
		if err != nil {
			printError(err)
			fmt.Fprintf(os.Stderr, "\nTo create a recipe from a package ecosystem:\n")
			fmt.Fprintf(os.Stderr, "  tsuku create %s --from <ecosystem>\n", toolName)
			fmt.Fprintf(os.Stderr, "\nAvailable ecosystems: crates.io, rubygems, pypi, npm\n")
			exitWithCode(ExitRecipeNotFound)
		}
		recipeSource = "registry"
	}

	// Resolve target platform (use flags or fall back to runtime)
	targetOS := evalOS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := evalArch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}

	// Check platform support for target platform
	if !r.SupportsPlatform(targetOS, targetArch) {
		printError(&recipe.UnsupportedPlatformError{
			RecipeName:           r.Metadata.Name,
			CurrentOS:            targetOS,
			CurrentArch:          targetArch,
			SupportedOS:          r.Metadata.SupportedOS,
			SupportedArch:        r.Metadata.SupportedArch,
			UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
		})
		exitWithCode(ExitGeneral)
	}

	// Load config to get cache directory
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create executor: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	defer exec.Cleanup()

	// Create downloader for checksum computation
	// This ensures `tsuku eval <tool> | tsuku install --plan -` is equivalent to `tsuku install <tool>`
	predownloader := validate.NewPreDownloader()
	downloader := validate.NewPreDownloaderAdapter(predownloader)

	// Create download cache for persisting downloads
	// Uses the standard cache directory so downloads can be reused by install --plan
	downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)

	// Configure plan generation
	planCfg := executor.PlanConfig{
		OS:                 evalOS,
		Arch:               evalArch,
		RecipeSource:       recipeSource,
		Downloader:         downloader,
		DownloadCache:      downloadCache,
		AutoAcceptEvalDeps: evalYes,
		RecipeLoader:       loader,
		OnWarning: func(action, message string) {
			// Output warnings to stderr so they don't mix with JSON
			fmt.Fprintf(os.Stderr, "Warning: %s\n", message)
		},
		OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
			return installEvalDeps(deps, autoAccept)
		},
	}

	// Generate plan (downloads files to compute checksums and caches them)
	plan, err := exec.GeneratePlan(globalCtx, planCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate plan: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Output JSON to stdout
	printJSON(plan)
}

// installEvalDeps prompts the user to install eval-time dependencies.
// If autoAccept is true, dependencies are installed without prompting.
func installEvalDeps(deps []string, autoAccept bool) error {
	if !autoAccept {
		fmt.Fprintf(os.Stderr, "The following tools are required for evaluation:\n")
		for _, dep := range deps {
			fmt.Fprintf(os.Stderr, "  - %s\n", dep)
		}
		fmt.Fprintf(os.Stderr, "\nInstall now? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("user declined to install dependencies")
		}
	} else {
		fmt.Fprintf(os.Stderr, "Installing eval-time dependencies: %v\n", deps)
	}

	// Install each dependency
	for _, dep := range deps {
		fmt.Fprintf(os.Stderr, "Installing %s...\n", dep)
		if err := runInstallTool(dep); err != nil {
			return fmt.Errorf("failed to install %s: %w", dep, err)
		}
		fmt.Fprintf(os.Stderr, "Installed %s\n", dep)
	}
	return nil
}

// runInstallTool installs a tool using the existing install infrastructure.
// It redirects stdout to stderr to avoid corrupting plan JSON output.
func runInstallTool(toolName string) error {
	// Redirect stdout to stderr during installation to prevent
	// install progress from corrupting the plan JSON on stdout
	origStdout := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = origStdout }()

	// Use the same install mechanism as the install command
	// Pass nil for telemetry client since this is an internal operation
	return runInstallWithTelemetry(toolName, "", "", false, "", nil)
}
