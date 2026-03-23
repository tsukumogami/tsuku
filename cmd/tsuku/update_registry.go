package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
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
		p := loader.ProviderBySource(recipe.SourceRegistry)
		if p == nil {
			printInfo("Registry not configured.")
			return
		}
		ra, ok := p.(recipe.RegistryAccessor)
		if !ok || ra.Registry() == nil {
			printInfo("Registry not configured.")
			return
		}
		reg := ra.Registry()

		// Create CachedRegistry with configured TTL
		ttl := config.GetRecipeCacheTTL()
		cachedReg := registry.NewCachedRegistry(reg, ttl)

		ctx := context.Background()

		if registryDryRun {
			runRegistryDryRun(cachedReg)
			return
		}

		// Fetch the registry manifest (recipes.json) so the satisfies
		// index can resolve ecosystem names for registry-only recipes.
		refreshManifest(ctx, reg)

		if registryRecipeName != "" {
			runSingleRecipeRefresh(ctx, cachedReg, registryRecipeName)
		} else {
			runRegistryRefreshAll(ctx, cachedReg)

			// Refresh distributed sources
			refreshDistributedSources(ctx)
		}

		// Regenerate the binary index from the current cached registry and
		// installed state so that 'tsuku install <command>' can resolve
		// commands to recipes.
		rebuildBinaryIndex(ctx, reg)
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

// refreshManifest fetches the registry manifest and caches it locally.
// The manifest provides the satisfies index for ecosystem name resolution.
// Errors are non-fatal: the CLI continues working without the manifest,
// but ecosystem name resolution for registry-only recipes won't work.
// If the manifest contains a deprecation notice, a warning is printed.
func refreshManifest(ctx context.Context, reg *registry.Registry) {
	manifest, err := reg.FetchManifest(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch registry manifest: %v\n", err)
		return
	}

	checkDeprecationWarning(manifest, reg.ManifestURL())
}

// refreshDistributedSources iterates all providers in the loader and calls
// Refresh() on those implementing RefreshableProvider, skipping the central
// registry (already refreshed above). Errors are reported per-source but
// don't block refresh of other sources.
func refreshDistributedSources(ctx context.Context) {
	var refreshed int
	var errors int

	for _, p := range loader.Providers() {
		// Skip the central registry -- it's already refreshed above
		if p.Source() == recipe.SourceRegistry {
			continue
		}

		rp, ok := p.(recipe.RefreshableProvider)
		if !ok {
			continue
		}

		source := string(p.Source())
		printInfo(fmt.Sprintf("Refreshing %s...", source))

		if err := rp.Refresh(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: error (%v)\n", source, err)
			errors++
			continue
		}

		fmt.Printf("  %s: refreshed\n", source)
		refreshed++
	}

	if refreshed+errors == 0 {
		return
	}

	fmt.Println()
	if errors > 0 {
		printInfo(fmt.Sprintf("Refreshed %d distributed source(s) (%d error(s)).", refreshed, errors))
	} else {
		printInfo(fmt.Sprintf("Refreshed %d distributed source(s).", refreshed))
	}
}

// rebuildBinaryIndex opens the binary index and rebuilds it from the cached
// registry and current installed state. If the cache directory does not exist
// yet (first-run scenario), the rebuild is skipped silently. Any other failure
// is printed to stderr and causes the command to exit non-zero.
func rebuildBinaryIndex(ctx context.Context, reg index.Registry) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config for index rebuild: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Ensure the cache directory exists before opening the index.
	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cache directory for index rebuild: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	dbPath := filepath.Join(cfg.CacheDir, "binary-index.db")
	idx, err := index.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open binary index: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	defer func() { _ = idx.Close() }()

	stateMgr := install.NewStateManager(cfg)
	stateReader := &stateReaderAdapter{mgr: stateMgr}

	if err := idx.Rebuild(ctx, reg, stateReader); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rebuild binary index: %v\n", err)
		exitWithCode(ExitGeneral)
	}
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
