package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var applyUpdatesCmd = &cobra.Command{
	Use:           "apply-updates",
	Hidden:        true,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 5-minute timeout to prevent indefinite hangs from stalled network connections
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Redirect stdout/stderr to devnull for truly silent background operation
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return nil
		}

		userCfg, err := userconfig.Load()
		if err != nil {
			return nil
		}

		if !userCfg.UpdatesAutoApplyEnabled() {
			return nil
		}

		cacheDir := updates.CacheDir(cfg.HomeDir)
		noticesDir := notices.NoticesDir(cfg.HomeDir)

		// Read cached check entries
		entries, err := updates.ReadAllEntries(cacheDir)
		if err != nil {
			log.Default().Debug("apply-updates: read cache entries", "error", err)
			return nil
		}

		// Filter for actionable entries (same logic as MaybeAutoApply)
		var pending []updates.UpdateCheckEntry
		for _, e := range entries {
			if updates.IsSelfUpdate(&e) {
				continue
			}
			if e.LatestWithinPin != "" && e.Error == "" && e.LatestWithinPin != e.ActiveVersion {
				pending = append(pending, e)
			}
		}

		if len(pending) == 0 {
			return nil
		}

		// Acquire state lock non-blocking; exit silently if held by another process
		lockPath := filepath.Join(cfg.HomeDir, "state.json.lock")
		lock := install.NewFileLock(lockPath)
		acquired, err := lock.TryLockExclusive()
		if err != nil {
			log.Default().Debug("apply-updates: try lock", "error", err)
			return nil
		}
		if !acquired {
			return nil
		}
		// Release probe lock immediately; install flow acquires its own locks
		_ = lock.Unlock()

		for _, entry := range pending {
			toolName := entry.Tool
			newVersion := entry.LatestWithinPin
			constraint := entry.Requested

			// Attempt install
			installErr := runInstallWithTelemetry(toolName, newVersion, constraint, false, "", nil)

			// Write notice regardless of success or failure
			var notice *notices.Notice
			if installErr == nil {
				notice = &notices.Notice{
					Tool:             toolName,
					AttemptedVersion: newVersion,
					Error:            "",
					Timestamp:        time.Now(),
					Shown:            false,
					Kind:             notices.KindAutoApplyResult,
				}
			} else {
				notice = &notices.Notice{
					Tool:             toolName,
					AttemptedVersion: newVersion,
					Error:            installErr.Error(),
					Timestamp:        time.Now(),
					Shown:            false,
					Kind:             notices.KindAutoApplyResult,
				}
			}
			_ = notices.WriteNotice(noticesDir, notice)

			// Consume the cache entry regardless of success or failure
			_ = updates.RemoveEntry(cacheDir, toolName)

			// Check context cancellation between iterations
			if ctx.Err() != nil {
				break
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(applyUpdatesCmd)
}
