package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/registry"
)

var (
	registryDryRun     bool
	registryRecipeName string
	registryRefreshAll bool
)

var updateRegistryCmd = &cobra.Command{
	Use:   "update-registry",
	Short: "Refresh the recipe cache",
	Long: `Refresh cached recipes from the registry.

By default, refreshes all expired cached recipes. Use --all to force refresh
of all cached recipes regardless of freshness.

Use --recipe to refresh a specific recipe only.
Use --dry-run to see what would be refreshed without making network requests.`,
	Run: func(cmd *cobra.Command, args []string) {
		reg := loader.Registry()
		if reg == nil {
			printInfo("Registry not configured.")
			return
		}

		// Create CachedRegistry with configured TTL
		ttl := config.GetRecipeCacheTTL()
		cachedReg := registry.NewCachedRegistry(reg, ttl)

		ctx := context.Background()

		if registryDryRun {
			runRegistryDryRun(cachedReg)
			return
		}

		if registryRecipeName != "" {
			runSingleRecipeRefresh(ctx, cachedReg, registryRecipeName)
			return
		}

		runRegistryRefreshAll(ctx, cachedReg)
	},
}

func init() {
	updateRegistryCmd.Flags().BoolVar(&registryDryRun, "dry-run", false, "Show what would be refreshed without fetching")
	updateRegistryCmd.Flags().StringVar(&registryRecipeName, "recipe", "", "Refresh a specific recipe only")
	updateRegistryCmd.Flags().BoolVar(&registryRefreshAll, "all", false, "Refresh all cached recipes regardless of freshness")
}

// runRegistryDryRun shows what would be refreshed without making network requests.
func runRegistryDryRun(cachedReg *registry.CachedRegistry) {
	cached, err := cachedReg.Registry().ListCached()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list cached recipes: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if len(cached) == 0 {
		printInfo("No cached recipes.")
		return
	}

	printInfo("Checking recipe cache...")

	// Sort for consistent output
	sort.Strings(cached)

	var expiredCount int
	for _, name := range cached {
		status, err := cachedReg.GetCacheStatus(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: error (%v)\n", name, err)
			continue
		}
		if status == nil {
			continue
		}

		if status.Status == "expired" {
			fmt.Printf("  %s: would refresh (cached %s ago)\n", name, formatAgeDuration(status.Age))
			expiredCount++
		} else {
			fmt.Printf("  %s: already fresh\n", name)
		}
	}

	fmt.Println()
	if expiredCount > 0 {
		printInfo(fmt.Sprintf("%d of %d cached recipes would be refreshed.", expiredCount, len(cached)))
	} else {
		printInfo("All cached recipes are fresh.")
	}
}

// runSingleRecipeRefresh refreshes a single recipe.
func runSingleRecipeRefresh(ctx context.Context, cachedReg *registry.CachedRegistry, name string) {
	printInfo(fmt.Sprintf("Refreshing recipe '%s'...", name))

	detail, err := cachedReg.Refresh(ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to refresh '%s': %v\n", name, err)
		exitWithCode(ExitGeneral)
	}

	if detail.Age > 0 {
		printInfo(fmt.Sprintf("  %s: refreshed (was %s old)", name, formatAgeDuration(detail.Age)))
	} else {
		printInfo(fmt.Sprintf("  %s: refreshed", name))
	}
}

// runRegistryRefreshAll refreshes all cached recipes.
func runRegistryRefreshAll(ctx context.Context, cachedReg *registry.CachedRegistry) {
	cached, err := cachedReg.Registry().ListCached()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list cached recipes: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if len(cached) == 0 {
		printInfo("No cached recipes to refresh.")
		return
	}

	printInfo("Refreshing recipe cache...")

	var stats *registry.RefreshStats

	if registryRefreshAll {
		// Force refresh all, including fresh ones
		stats = forceRegistryRefreshAll(ctx, cachedReg, cached)
	} else {
		// Normal refresh (only expired)
		stats, err = cachedReg.RefreshAll(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to refresh cache: %v\n", err)
			exitWithCode(ExitGeneral)
		}
	}

	// Sort details by name for consistent output
	sort.Slice(stats.Details, func(i, j int) bool {
		return stats.Details[i].Name < stats.Details[j].Name
	})

	// Print individual results
	for _, detail := range stats.Details {
		switch detail.Status {
		case "refreshed":
			if detail.Age > 0 {
				fmt.Printf("  %s: refreshed (was %s old)\n", detail.Name, formatAgeDuration(detail.Age))
			} else {
				fmt.Printf("  %s: refreshed\n", detail.Name)
			}
		case "already fresh":
			fmt.Printf("  %s: already fresh\n", detail.Name)
		case "error":
			fmt.Fprintf(os.Stderr, "  %s: error (%v)\n", detail.Name, detail.Error)
		}
	}

	// Print summary
	fmt.Println()
	if stats.Errors > 0 {
		printInfo(fmt.Sprintf("Refreshed %d of %d cached recipes (%d errors).",
			stats.Refreshed, stats.Total, stats.Errors))
	} else if stats.Refreshed > 0 {
		printInfo(fmt.Sprintf("Refreshed %d of %d cached recipes.", stats.Refreshed, stats.Total))
	} else {
		printInfo("All cached recipes are already fresh.")
	}

	// Clear in-memory cache
	loader.ClearCache()
}

// forceRegistryRefreshAll refreshes all recipes regardless of freshness.
func forceRegistryRefreshAll(ctx context.Context, cachedReg *registry.CachedRegistry, cached []string) *registry.RefreshStats {
	stats := &registry.RefreshStats{
		Total:   len(cached),
		Details: make([]registry.RefreshDetail, 0, len(cached)),
	}

	for _, name := range cached {
		detail, err := cachedReg.Refresh(ctx, name)
		if err != nil {
			stats.Errors++
			if detail != nil {
				stats.Details = append(stats.Details, *detail)
			} else {
				stats.Details = append(stats.Details, registry.RefreshDetail{
					Name:   name,
					Status: "error",
					Error:  err,
				})
			}
			continue
		}

		stats.Refreshed++
		stats.Details = append(stats.Details, *detail)
	}

	return stats
}

// formatAgeDuration formats a duration for human-readable display.
func formatAgeDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
