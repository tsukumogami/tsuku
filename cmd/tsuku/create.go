package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/toolchain"
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
	createFrom  string
	createForce bool
)

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "Source: ecosystem name or github:owner/repo (required)")
	createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
	_ = createCmd.MarkFlagRequired("from")
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

	// Parse the --from flag
	builderName, sourceArg, isGitHub := parseFromFlag(createFrom)

	// Initialize builder registry
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))
	builderRegistry.Register(builders.NewPyPIBuilder(nil))
	builderRegistry.Register(builders.NewNpmBuilder(nil))

	// Register GitHub builder (may fail if ANTHROPIC_API_KEY not set)
	if isGitHub {
		ghBuilder, err := builders.NewGitHubReleaseBuilder(nil, nil)
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

	// Show warnings (printed to stderr)
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
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
	printInfo()
	printInfo("To install, run:")
	printInfof("  tsuku install %s\n", toolName)
}
