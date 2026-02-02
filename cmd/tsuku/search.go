package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for tools",
	Long:  `Search for tools by name or description across all available recipes.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := ""
		if len(args) > 0 {
			query = strings.ToLower(strings.TrimSpace(args[0]))
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")

		// Get all recipes from local, embedded, and registry sources
		allRecipes, err := loader.ListAllWithSource()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list recipes: %v\n", err)
		}

		// Filter and collect results
		type result struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Installed   string `json:"installed,omitempty"`
		}
		var results []result

		// Initialize install manager to check status
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		}
		var installedTools []install.InstalledTool
		if cfg != nil {
			mgr := install.New(cfg)
			installedTools, _ = mgr.List()
		}

		for _, ri := range allRecipes {
			match := query == "" ||
				strings.Contains(strings.ToLower(ri.Name), query) ||
				strings.Contains(strings.ToLower(ri.Description), query)

			if match {
				installedVer := ""
				for _, t := range installedTools {
					if t.Name == ri.Name {
						installedVer = t.Version
						break
					}
				}

				results = append(results, result{
					Name:        ri.Name,
					Description: ri.Description,
					Installed:   installedVer,
				})
			}
		}

		// JSON output mode
		if jsonOutput {
			type searchOutput struct {
				Results []result `json:"results"`
			}
			output := searchOutput{Results: results}
			if output.Results == nil {
				output.Results = []result{}
			}
			printJSON(output)
			return
		}

		if len(results) == 0 {
			if query == "" {
				printInfo("No recipes found.")
			} else {
				printInfof("No recipes found for '%s'.\n\n", query)
				printInfo("Tip: You can still try installing it!")
				printInfof("   Run: tsuku install %s\n", query)
				printInfo("   (Tsuku will attempt to find and install it using AI)")
			}
			return
		}

		// Print table
		// Calculate column widths
		maxName := 4  // "NAME"
		maxDesc := 11 // "DESCRIPTION"
		for _, r := range results {
			if len(r.Name) > maxName {
				maxName = len(r.Name)
			}
			if len(r.Description) > maxDesc {
				maxDesc = len(r.Description)
			}
		}
		// Cap description width to avoid wrapping mess
		if maxDesc > 60 {
			maxDesc = 60
		}

		fmt.Printf("%-*s  %-*s  %s\n", maxName, "NAME", maxDesc, "DESCRIPTION", "INSTALLED")
		for _, r := range results {
			desc := r.Description
			if len(desc) > maxDesc {
				desc = desc[:maxDesc-3] + "..."
			}
			installed := r.Installed
			if installed == "" {
				installed = "-"
			}
			fmt.Printf("%-*s  %-*s  %s\n", maxName, r.Name, maxDesc, desc, installed)
		}
	},
}

func init() {
	searchCmd.Flags().Bool("json", false, "Output in JSON format")
}
