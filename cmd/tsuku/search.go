package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/distributed"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// searchResult holds a single search result for display or JSON output.
type searchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Installed   string `json:"installed,omitempty"`
	Source      string `json:"source,omitempty"`
}

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

		// Get all recipes from local, embedded, registry, and distributed sources
		allRecipes, listErrs := loader.ListAllWithSource()
		for _, e := range listErrs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
		}

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

		// Track which recipe names came from the loader (built-in + distributed
		// providers that were successfully initialized)
		loaderRecipeNames := make(map[string]bool)
		var results []searchResult

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

				results = append(results, searchResult{
					Name:        ri.Name,
					Description: ri.Description,
					Installed:   installedVer,
				})
			}
			loaderRecipeNames[ri.Name] = true
		}

		// Search distributed source caches for additional matches.
		// This catches recipes from sources that failed to initialize as
		// providers (e.g., network errors at startup) but have cached
		// metadata from a prior run.
		var distResults []searchResult
		if cfg != nil {
			distResults = searchDistributedCaches(cfg, query, loaderRecipeNames, installedTools)
		}

		// JSON output mode
		if jsonOutput {
			type searchOutput struct {
				Results     []searchResult `json:"results"`
				Distributed []searchResult `json:"distributed,omitempty"`
			}
			output := searchOutput{Results: results, Distributed: distResults}
			if output.Results == nil {
				output.Results = []searchResult{}
			}
			printJSON(output)
			return
		}

		totalResults := len(results) + len(distResults)
		if totalResults == 0 {
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

		// Print main results table
		if len(results) > 0 {
			printSearchResultsTable(results)
		}

		// Print distributed results in a separate section
		if len(distResults) > 0 {
			if len(results) > 0 {
				fmt.Println()
			}
			fmt.Println("Distributed sources:")
			printSearchResultsTable(distResults)
		}
	},
}

// searchDistributedCaches searches cached metadata from registered distributed
// sources for recipe names matching the query. It reads only local cache files
// and makes no network calls.
func searchDistributedCaches(
	cfg *config.Config,
	query string,
	alreadySeen map[string]bool,
	installedTools []install.InstalledTool,
) []searchResult {
	userCfg, err := userconfig.Load()
	if err != nil || len(userCfg.Registries) == 0 {
		return nil
	}

	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)

	var results []searchResult
	for source := range userCfg.Registries {
		parts := strings.SplitN(source, "/", 2)
		if len(parts) != 2 {
			continue
		}
		owner, repo := parts[0], parts[1]

		meta, metaErr := cache.GetSourceMeta(owner, repo)
		if metaErr != nil || meta == nil {
			continue
		}

		for name := range meta.Files {
			if alreadySeen[name] {
				continue
			}

			match := query == "" ||
				strings.Contains(strings.ToLower(name), query)

			if match {
				installedVer := ""
				for _, t := range installedTools {
					if t.Name == name {
						installedVer = t.Version
						break
					}
				}

				results = append(results, searchResult{
					Name:      name,
					Source:    source,
					Installed: installedVer,
				})
				alreadySeen[name] = true
			}
		}
	}

	return results
}

// printSearchResultsTable prints a formatted table of search results.
func printSearchResultsTable(results []searchResult) {
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
	if maxDesc > 60 {
		maxDesc = 60
	}

	// Determine if we need a source column
	hasSource := false
	for _, r := range results {
		if r.Source != "" {
			hasSource = true
			break
		}
	}

	if hasSource {
		maxSource := 6 // "SOURCE"
		for _, r := range results {
			if len(r.Source) > maxSource {
				maxSource = len(r.Source)
			}
		}
		fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxName, "NAME", maxDesc, "DESCRIPTION", maxSource, "SOURCE", "INSTALLED")
		for _, r := range results {
			desc := r.Description
			if len(desc) > maxDesc {
				desc = desc[:maxDesc-3] + "..."
			}
			installed := r.Installed
			if installed == "" {
				installed = "-"
			}
			fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxName, r.Name, maxDesc, desc, maxSource, r.Source, installed)
		}
	} else {
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
	}
}

func init() {
	searchCmd.Flags().Bool("json", false, "Output in JSON format")
}
