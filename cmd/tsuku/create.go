package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/builders"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/toolchain"
)

var createCmd = &cobra.Command{
	Use:   "create <tool> --from <ecosystem>",
	Short: "Create a recipe from a package ecosystem",
	Long: `Create a recipe by querying a package ecosystem's metadata API.

The generated recipe is written to ~/.tsuku/recipes/<tool>.toml and can be
inspected or edited before running 'tsuku install <tool>'.

Supported ecosystems:
  crates.io    Rust crates from crates.io
  rubygems     Ruby gems from rubygems.org
  pypi         Python packages from pypi.org
  npm          Node.js packages from npmjs.com

Examples:
  tsuku create ripgrep --from crates.io
  tsuku create bat --from crates.io --force
  tsuku create jekyll --from rubygems
  tsuku create ruff --from pypi
  tsuku create prettier --from npm`,
	Args: cobra.ExactArgs(1),
	Run:  runCreate,
}

var (
	createFrom  string
	createForce bool
)

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "Package ecosystem to use (required)")
	createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
	_ = createCmd.MarkFlagRequired("from")
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

	// Normalize ecosystem name
	ecosystem := normalizeEcosystem(createFrom)

	// Check toolchain availability before making API calls
	if err := toolchain.CheckAvailable(ecosystem); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitWithCode(ExitDependencyFailed)
	}

	// Initialize builder registry
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))
	builderRegistry.Register(builders.NewPyPIBuilder(nil))
	builderRegistry.Register(builders.NewNpmBuilder(nil))

	// Get the builder
	builder, ok := builderRegistry.Get(ecosystem)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown ecosystem '%s'\n", createFrom)
		fmt.Fprintf(os.Stderr, "\nAvailable ecosystems:\n")
		for _, name := range builderRegistry.List() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		exitWithCode(ExitUsage)
	}

	ctx := context.Background()

	// Check if builder can handle this package
	canBuild, err := builder.CanBuild(ctx, toolName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking package: %v\n", err)
		exitWithCode(ExitNetwork)
	}
	if !canBuild {
		fmt.Fprintf(os.Stderr, "Error: package '%s' not found in %s\n", toolName, ecosystem)
		exitWithCode(ExitRecipeNotFound)
	}

	// Build the recipe
	printInfof("Creating recipe for %s from %s...\n", toolName, ecosystem)
	result, err := builder.Build(ctx, toolName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building recipe: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Show warnings
	for _, warning := range result.Warnings {
		printInfof("Warning: %s\n", warning)
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
