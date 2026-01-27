package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/registry"
)

var (
	cleanupDryRun     bool
	cleanupMaxAge     string
	cleanupForceLimit bool
)

var cacheCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove old recipe cache entries",
	Long: `Remove old recipe cache entries based on last access time.

By default, removes entries not accessed within 30 days.

Examples:
  tsuku cache cleanup                    # Remove entries older than 30 days
  tsuku cache cleanup --max-age 7d       # Remove entries older than 7 days
  tsuku cache cleanup --dry-run          # Show what would be removed
  tsuku cache cleanup --force-limit      # Evict entries to enforce size limit`,
	Run: runCacheCleanup,
}

func init() {
	cacheCleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be removed without deleting")
	cacheCleanupCmd.Flags().StringVar(&cleanupMaxAge, "max-age", "30d", "Maximum age for cache entries (e.g., 30d, 7d, 24h)")
	cacheCleanupCmd.Flags().BoolVar(&cleanupForceLimit, "force-limit", false, "Force LRU eviction to enforce size limit")
}

func runCacheCleanup(cmd *cobra.Command, args []string) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		printError(err)
		exitWithCode(ExitGeneral)
	}

	sizeLimit := config.GetRecipeCacheSizeLimit()
	cm := registry.NewCacheManager(cfg.RegistryDir, sizeLimit)

	// Get size before cleanup
	sizeBefore, err := cm.Size()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get cache size: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if cleanupForceLimit {
		runForceLimitCleanup(cm, sizeBefore, sizeLimit)
		return
	}

	// Parse max-age duration
	maxAge, err := parseDuration(cleanupMaxAge)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid --max-age value: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	runAgeBasedCleanup(cm, maxAge, sizeBefore, sizeLimit)
}

func runForceLimitCleanup(cm *registry.CacheManager, sizeBefore, sizeLimit int64) {
	if cleanupDryRun {
		// For dry-run, just show current status
		fmt.Println("Dry run: checking cache status...")
		printCacheStatus(sizeBefore, sizeLimit)

		highWater := int64(float64(sizeLimit) * 0.80)
		if sizeBefore > highWater {
			fmt.Printf("\nCache is above high water mark (80%%). Would evict entries to reach 60%%.\n")
		} else {
			fmt.Println("\nCache is below high water mark. No eviction needed.")
		}
		return
	}

	fmt.Println("Enforcing cache size limit...")

	evicted, err := cm.EnforceLimit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to enforce limit: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	sizeAfter, _ := cm.Size()

	if evicted == 0 {
		fmt.Println("Cache is within size limits. No entries removed.")
	} else {
		fmt.Printf("Removed %d entries, freed %s.\n", evicted, formatBytes(sizeBefore-sizeAfter))
	}

	printCacheStatus(sizeAfter, sizeLimit)
}

func runAgeBasedCleanup(cm *registry.CacheManager, maxAge time.Duration, sizeBefore, sizeLimit int64) {
	action := "Cleaning up"
	if cleanupDryRun {
		action = "Would remove"
		fmt.Println("Dry run: showing entries that would be removed...")
	} else {
		fmt.Println("Cleaning up recipe cache...")
	}

	details, freedBytes, err := cm.CleanupWithDetails(maxAge, cleanupDryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to cleanup cache: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if len(details) == 0 {
		fmt.Println("No entries to remove.")
		printCacheStatus(sizeBefore, sizeLimit)
		return
	}

	// Print each entry being removed
	for _, detail := range details {
		ageDays := int(detail.Age.Hours() / 24)
		fmt.Printf("  %s %s (not accessed in %d days)\n", action, detail.Name, ageDays)
	}

	fmt.Println()
	if cleanupDryRun {
		fmt.Printf("Would remove %d entries, freeing %s.\n", len(details), formatBytes(freedBytes))
		printCacheStatus(sizeBefore, sizeLimit)
	} else {
		sizeAfter, _ := cm.Size()
		fmt.Printf("Removed %d entries, freed %s.\n", len(details), formatBytes(freedBytes))
		printCacheStatus(sizeAfter, sizeLimit)
	}
}

func printCacheStatus(size, limit int64) {
	percent := float64(size) / float64(limit) * 100
	fmt.Printf("Cache: %s of %s (%.2f%%)\n", formatBytes(size), formatBytes(limit), percent)
}

// parseDuration parses a duration string with support for "d" suffix for days.
// Examples: "30d", "7d", "24h", "1h30m"
func parseDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Handle "Xd" format for days (Go's time.ParseDuration doesn't support days)
	if len(value) > 1 && (value[len(value)-1] == 'd' || value[len(value)-1] == 'D') {
		daysStr := value[:len(value)-1]
		days, err := strconv.ParseFloat(daysStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid day format: %s", value)
		}
		if days <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s", value)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}
