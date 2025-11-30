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

Examples:
  tsuku create ripgrep --from crates.io
  tsuku create bat --from crates.io --force
  tsuku create jekyll --from rubygems`,
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
	default:
		return normalized
	}
}

func runCreate(cmd *cobra.Command, args []string) {
	toolName := args[0]

	// Normalize ecosystem name
	ecosystem := normalizeEcosystem(createFrom)

	// Initialize builder registry
	builderRegistry := builders.NewRegistry()
	builderRegistry.Register(builders.NewCargoBuilder(nil))
	builderRegistry.Register(builders.NewGemBuilder(nil))

	// Get the builder
	builder, ok := builderRegistry.Get(ecosystem)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown ecosystem '%s'\n", createFrom)
		fmt.Fprintf(os.Stderr, "\nAvailable ecosystems:\n")
		for _, name := range builderRegistry.List() {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		os.Exit(1)
	}

	ctx := context.Background()

	// Check if builder can handle this package
	canBuild, err := builder.CanBuild(ctx, toolName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking package: %v\n", err)
		os.Exit(1)
	}
	if !canBuild {
		fmt.Fprintf(os.Stderr, "Error: package '%s' not found in %s\n", toolName, ecosystem)
		os.Exit(1)
	}

	// Build the recipe
	fmt.Printf("Creating recipe for %s from %s...\n", toolName, ecosystem)
	result, err := builder.Build(ctx, toolName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building recipe: %v\n", err)
		os.Exit(1)
	}

	// Show warnings
	for _, warning := range result.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}

	// Get recipes directory
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting config: %v\n", err)
		os.Exit(1)
	}

	recipePath := filepath.Join(cfg.RecipesDir, toolName+".toml")

	// Check if recipe already exists
	if _, err := os.Stat(recipePath); err == nil && !createForce {
		fmt.Fprintf(os.Stderr, "Error: recipe already exists at %s\n", recipePath)
		fmt.Fprintf(os.Stderr, "Use --force to overwrite\n")
		os.Exit(1)
	}

	// Write the recipe
	if err := recipe.WriteRecipe(result.Recipe, recipePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing recipe: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nRecipe created: %s\n", recipePath)
	fmt.Printf("Source: %s\n", result.Source)
	fmt.Println()
	fmt.Printf("To install, run:\n")
	fmt.Printf("  tsuku install %s\n", toolName)
}
