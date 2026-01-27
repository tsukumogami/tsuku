package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/registry"
	"github.com/tsukumogami/tsuku/internal/version"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage tsuku caches",
	Long:  `Manage tsuku caches including download and version caches.`,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all caches",
	Long: `Clear all tsuku caches (downloads and versions).

This removes cached downloads and version information,
forcing fresh downloads and version lookups on next use.`,
	Run: func(cmd *cobra.Command, args []string) {
		downloadsOnly, _ := cmd.Flags().GetBool("downloads")
		versionsOnly, _ := cmd.Flags().GetBool("versions")

		// If no specific flag, clear all
		clearAll := !downloadsOnly && !versionsOnly

		cfg, err := config.DefaultConfig()
		if err != nil {
			printError(err)
			exitWithCode(ExitGeneral)
		}

		if clearAll || downloadsOnly {
			downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)
			if err := downloadCache.Clear(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to clear download cache: %v\n", err)
				exitWithCode(ExitGeneral)
			}
			fmt.Println("Download cache cleared")
		}

		if clearAll || versionsOnly {
			versionCache := version.NewCache(cfg.VersionCacheDir)
			if err := versionCache.Clear(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to clear version cache: %v\n", err)
				exitWithCode(ExitGeneral)
			}
			fmt.Println("Version cache cleared")
		}
	},
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache information",
	Long:  `Show information about tsuku caches including size and entry counts.`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonOutput, _ := cmd.Flags().GetBool("json")

		cfg, err := config.DefaultConfig()
		if err != nil {
			printError(err)
			exitWithCode(ExitGeneral)
		}

		downloadCache := actions.NewDownloadCache(cfg.DownloadCacheDir)
		downloadInfo, err := downloadCache.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get download cache info: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		versionCache := version.NewCache(cfg.VersionCacheDir)
		versionInfo, err := versionCache.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get version cache info: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		// Get registry cache info
		sizeLimit := config.GetRecipeCacheSizeLimit()
		ttl := config.GetRecipeCacheTTL()
		registryManager := registry.NewCacheManager(cfg.RegistryDir, sizeLimit)
		registryInfo, err := registryManager.InfoWithTTL(ttl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get registry cache info: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		if jsonOutput {
			type cacheInfoOutput struct {
				Downloads struct {
					Entries int64 `json:"entries"`
					Size    int64 `json:"size_bytes"`
				} `json:"downloads"`
				Versions struct {
					Entries int64 `json:"entries"`
					Size    int64 `json:"size_bytes"`
				} `json:"versions"`
				Registry struct {
					Entries    int64  `json:"entries"`
					Size       int64  `json:"size_bytes"`
					OldestName string `json:"oldest_name,omitempty"`
					NewestName string `json:"newest_name,omitempty"`
					StaleCount int    `json:"stale_count"`
					SizeLimit  int64  `json:"size_limit_bytes"`
				} `json:"registry"`
			}
			output := cacheInfoOutput{}
			output.Downloads.Entries = int64(downloadInfo.EntryCount)
			output.Downloads.Size = downloadInfo.TotalSize
			output.Versions.Entries = int64(versionInfo.EntryCount)
			output.Versions.Size = versionInfo.TotalSize
			output.Registry.Entries = int64(registryInfo.EntryCount)
			output.Registry.Size = registryInfo.TotalSize
			output.Registry.OldestName = registryInfo.OldestName
			output.Registry.NewestName = registryInfo.NewestName
			output.Registry.StaleCount = registryInfo.StaleCount
			output.Registry.SizeLimit = sizeLimit
			printJSON(output)
			return
		}

		fmt.Println("Cache Information")
		fmt.Println()
		fmt.Println("Downloads:")
		fmt.Printf("  Entries: %d\n", downloadInfo.EntryCount)
		fmt.Printf("  Size:    %s\n", formatBytes(downloadInfo.TotalSize))
		fmt.Printf("  Path:    %s\n", cfg.DownloadCacheDir)
		fmt.Println()
		fmt.Println("Versions:")
		fmt.Printf("  Entries: %d\n", versionInfo.EntryCount)
		fmt.Printf("  Size:    %s\n", formatBytes(versionInfo.TotalSize))
		fmt.Printf("  Path:    %s\n", cfg.VersionCacheDir)
		fmt.Println()
		fmt.Println("Registry:")
		fmt.Printf("  Entries: %d\n", registryInfo.EntryCount)
		fmt.Printf("  Size:    %s\n", formatBytes(registryInfo.TotalSize))
		if registryInfo.OldestName != "" {
			fmt.Printf("  Oldest:  %s (cached %s)\n", registryInfo.OldestName, formatRelativeTime(registryInfo.OldestAccess))
		}
		if registryInfo.NewestName != "" {
			fmt.Printf("  Newest:  %s (cached %s)\n", registryInfo.NewestName, formatRelativeTime(registryInfo.NewestAccess))
		}
		if registryInfo.StaleCount > 0 {
			fmt.Printf("  Stale:   %d entries (require refresh)\n", registryInfo.StaleCount)
		}
		percentUsed := float64(registryInfo.TotalSize) / float64(sizeLimit) * 100
		fmt.Printf("  Limit:   %s (%.2f%% used)\n", formatBytes(sizeLimit), percentUsed)
		fmt.Printf("  Path:    %s\n", cfg.RegistryDir)
	},
}

// formatRelativeTime formats a time as a human-readable relative duration.
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	age := time.Since(t)
	switch {
	case age < time.Hour:
		mins := int(age.Minutes())
		if mins <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case age < 24*time.Hour:
		hours := int(age.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(age.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// formatBytes formats a byte count as a human-readable string
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

func init() {
	// Add subcommands
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheCleanupCmd)
	cacheCmd.AddCommand(cacheInfoCmd)

	// Flags for cache clear
	cacheClearCmd.Flags().Bool("downloads", false, "Clear only download cache")
	cacheClearCmd.Flags().Bool("versions", false, "Clear only version cache")

	// Flags for cache info
	cacheInfoCmd.Flags().Bool("json", false, "Output in JSON format")
}
