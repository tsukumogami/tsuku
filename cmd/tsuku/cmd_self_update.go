package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/buildinfo"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/version"
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update tsuku to the latest version",
	Long:  `Downloads and installs the latest tsuku release, replacing the current binary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		current := buildinfo.Version()
		if updates.IsDevBuild(current) {
			return fmt.Errorf("self-update is not available for development builds (version: %s)", current)
		}

		// Resolve latest version
		res := version.New()
		provider := version.NewGitHubProvider(res, updates.SelfRepo)
		latest, err := provider.ResolveLatest(cmd.Context())
		if err != nil {
			return fmt.Errorf("resolve latest version: %w", err)
		}

		// Normalize versions for comparison (strip "v" prefix from both sides)
		currentNorm := strings.TrimPrefix(current, "v")
		latestNorm := strings.TrimPrefix(latest.Version, "v")

		cmp := updates.CompareSemver(currentNorm, latestNorm)
		if cmp == 0 {
			fmt.Fprintf(os.Stderr, "tsuku is already up to date (%s)\n", currentNorm)
			return nil
		}
		if cmp > 0 {
			fmt.Fprintf(os.Stderr, "Current version (%s) is newer than latest release (%s)\n", currentNorm, latestNorm)
			return nil
		}

		// Acquire non-blocking lock
		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cacheDir := updates.CacheDir(cfg.HomeDir)
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("create cache directory: %w", err)
		}
		lockPath := filepath.Join(cacheDir, updates.SelfUpdateLockFile)
		lock := install.NewFileLock(lockPath)
		acquired, err := lock.TryLockExclusive()
		if err != nil {
			return fmt.Errorf("check self-update lock: %w", err)
		}
		if !acquired {
			return fmt.Errorf("another self-update is running")
		}
		defer func() { _ = lock.Unlock() }()

		// Resolve binary path
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			return fmt.Errorf("resolve symlinks: %w", err)
		}

		assetName := fmt.Sprintf("tsuku-%s-%s", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(os.Stderr, "Downloading tsuku %s...\n", latestNorm)

		if err := updates.ApplySelfUpdate(cmd.Context(), exePath, latest.Tag, assetName); err != nil {
			return fmt.Errorf("self-update failed: %w. Current binary restored", err)
		}

		fmt.Fprintf(os.Stderr, "Updated tsuku from %s to %s\n", currentNorm, latestNorm)
		return nil
	},
}

