package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/recipe"
)

var (
	recipesLocalOnly bool
)

var recipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "List available recipes",
	Long: `List all available recipes from local and registry sources.

By default, lists all recipes from both sources. Local recipes are shown
first and take precedence over registry recipes with the same name.

Use --local to show only recipes from your local recipes directory
($TSUKU_HOME/recipes/).`,
	Run: func(cmd *cobra.Command, args []string) {
		var recipes []recipe.RecipeInfo
		var err error

		if recipesLocalOnly {
			recipes, err = loader.ListLocal()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing local recipes: %v\n", err)
				os.Exit(1)
			}
		} else {
			recipes, err = loader.ListAllWithSource()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing recipes: %v\n", err)
				os.Exit(1)
			}
		}

		if len(recipes) == 0 {
			if recipesLocalOnly {
				fmt.Println("No local recipes found.")
				fmt.Println()
				fmt.Println("Create a recipe with:")
				fmt.Println("  tsuku create <tool> --from <ecosystem>")
				fmt.Println()
				fmt.Println("Available ecosystems: crates.io, rubygems, pypi, npm")
			} else {
				fmt.Println("No recipes found.")
				fmt.Println()
				fmt.Println("Update the registry cache with:")
				fmt.Println("  tsuku update-registry")
			}
			return
		}

		// Sort by name
		sort.Slice(recipes, func(i, j int) bool {
			return recipes[i].Name < recipes[j].Name
		})

		// Count by source
		localCount := 0
		registryCount := 0
		for _, r := range recipes {
			if r.Source == recipe.SourceLocal {
				localCount++
			} else {
				registryCount++
			}
		}

		if recipesLocalOnly {
			fmt.Printf("Local recipes (%d total):\n\n", localCount)
		} else {
			fmt.Printf("Available recipes (%d total: %d local, %d registry):\n\n", len(recipes), localCount, registryCount)
		}

		for _, r := range recipes {
			sourceIndicator := ""
			if !recipesLocalOnly {
				if r.Source == recipe.SourceLocal {
					sourceIndicator = "[local]    "
				} else {
					sourceIndicator = "[registry] "
				}
			}
			fmt.Printf("  %s%-20s  %s\n", sourceIndicator, r.Name, r.Description)
		}
	},
}

func init() {
	recipesCmd.Flags().BoolVar(&recipesLocalOnly, "local", false, "Show only local recipes")
}
