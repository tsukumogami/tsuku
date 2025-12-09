package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
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
			}
			output := cacheInfoOutput{}
			output.Downloads.Entries = int64(downloadInfo.EntryCount)
			output.Downloads.Size = downloadInfo.TotalSize
			output.Versions.Entries = int64(versionInfo.EntryCount)
			output.Versions.Size = versionInfo.TotalSize
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
	},
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
	cacheCmd.AddCommand(cacheInfoCmd)

	// Flags for cache clear
	cacheClearCmd.Flags().Bool("downloads", false, "Clear only download cache")
	cacheClearCmd.Flags().Bool("versions", false, "Clear only version cache")

	// Flags for cache info
	cacheInfoCmd.Flags().Bool("json", false, "Output in JSON format")
}
