// Command bootstrap-queue performs the initial migration from per-ecosystem
// queues to the unified priority queue format. It scans three data sources
// in precedence order:
//
//  1. Existing recipes (status: success) - these already have working recipes
//  2. Curated overrides (status: pending) - expert-specified sources
//  3. Homebrew queue (status: pending) - remaining packages as homebrew:<name>
//
// Duplicate entries are resolved by precedence: recipe > curated > homebrew.
// The output is data/queues/priority-queue.json in the unified format.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func main() {
	recipesDir := flag.String("recipes-dir", "recipes", "path to the recipes directory")
	curatedPath := flag.String("curated-path", "data/disambiguations/curated.jsonl", "path to the curated overrides file")
	homebrewPath := flag.String("homebrew-path", "data/queues/priority-queue-homebrew.json", "path to the homebrew priority queue")
	outputPath := flag.String("output-path", "data/queues/priority-queue.json", "path to write the unified queue")
	flag.Parse()

	cfg := batch.BootstrapConfig{
		RecipesDir:   *recipesDir,
		CuratedPath:  *curatedPath,
		HomebrewPath: *homebrewPath,
		OutputPath:   *outputPath,
	}

	result, err := batch.Bootstrap(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Bootstrap complete:\n")
	fmt.Fprintf(os.Stderr, "  Recipes:  %d entries (status: success)\n", result.RecipeEntries)
	fmt.Fprintf(os.Stderr, "  Curated:  %d entries (status: pending)\n", result.CuratedEntries)
	fmt.Fprintf(os.Stderr, "  Homebrew: %d entries (status: pending)\n", result.HomebrewEntries)
	fmt.Fprintf(os.Stderr, "  Total:    %d entries\n", result.TotalEntries)
}
