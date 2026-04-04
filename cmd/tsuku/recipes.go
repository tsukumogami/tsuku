package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

var (
	recipesLocalOnly bool
)

var recipesCmd = &cobra.Command{
	Use:   "recipes",
	Short: "List available recipes",
	Long: `List all available recipes from local, registry, and distributed sources.

By default, lists all recipes from all sources. Local recipes are shown
first and take precedence over registry recipes with the same name.

Use --local to show only recipes from your local recipes directory
($TSUKU_HOME/recipes/).`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonOutput, _ := cmd.Flags().GetBool("json")

		var recipes []recipe.RecipeInfo

		if recipesLocalOnly {
			var err error
			recipes, err = loader.ListLocal()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error listing local recipes: %v\n", err)
				exitWithCode(ExitGeneral)
			}
		} else {
			var listErrs []error
			recipes, listErrs = loader.ListAllWithSource()
			for _, e := range listErrs {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
			}
		}

		if len(recipes) == 0 {
			if recipesLocalOnly {
				printInfo("No local recipes found.")
				printInfo()
				printInfo("Create a recipe with:")
				printInfo("  tsuku create <tool> --from <ecosystem>")
				printInfo()
				printInfo("Available ecosystems: crates.io, rubygems, pypi, npm")
			} else {
				printInfo("No recipes found.")
				printInfo()
				printInfo("Update the registry cache with:")
				printInfo("  tsuku update-registry")
			}
			return
		}

		// Sort by name
		sort.Slice(recipes, func(i, j int) bool {
			return recipes[i].Name < recipes[j].Name
		})

		// JSON output mode
		if jsonOutput {
			type recipeJSON struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Source      string `json:"source"`
			}
			type recipesOutput struct {
				Recipes []recipeJSON `json:"recipes"`
			}
			output := recipesOutput{
				Recipes: make([]recipeJSON, 0, len(recipes)),
			}
			for _, r := range recipes {
				output.Recipes = append(output.Recipes, recipeJSON{
					Name:        r.Name,
					Description: r.Description,
					Source:      string(r.Source),
				})
			}
			printJSON(output)
			return
		}

		// Count by source
		localCount := 0
		embeddedCount := 0
		registryCount := 0
		distributedCount := 0
		for _, r := range recipes {
			switch r.Source {
			case recipe.SourceLocal:
				localCount++
			case recipe.SourceEmbedded:
				embeddedCount++
			case recipe.SourceRegistry:
				registryCount++
			default:
				// Distributed sources have "owner/repo" as source
				if strings.Contains(string(r.Source), "/") {
					distributedCount++
				}
			}
		}

		if recipesLocalOnly {
			printInfof("Local recipes (%d total):\n\n", localCount)
		} else if distributedCount > 0 {
			printInfof("Available recipes (%d total: %d local, %d embedded, %d registry, %d distributed):\n\n", len(recipes), localCount, embeddedCount, registryCount, distributedCount)
		} else {
			printInfof("Available recipes (%d total: %d local, %d embedded, %d registry):\n\n", len(recipes), localCount, embeddedCount, registryCount)
		}

		for _, r := range recipes {
			sourceIndicator := ""
			if !recipesLocalOnly {
				switch r.Source {
				case recipe.SourceLocal:
					sourceIndicator = "[local]    "
				case recipe.SourceEmbedded:
					sourceIndicator = "[embedded] "
				case recipe.SourceRegistry:
					sourceIndicator = "[registry] "
				default:
					if strings.Contains(string(r.Source), "/") {
						sourceIndicator = fmt.Sprintf("[%s] ", r.Source)
					}
				}
			}
			fmt.Printf("  %s%-20s  %s\n", sourceIndicator, r.Name, r.Description)
		}
	},
}

func init() {
	recipesCmd.Flags().BoolVar(&recipesLocalOnly, "local", false, "Show only local recipes")
	recipesCmd.Flags().Bool("json", false, "Output in JSON format")
}
