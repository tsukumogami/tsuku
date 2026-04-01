package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// InstallFunc is the callback type for the install flow.
// Injected by cmd/tsuku/main.go wrapping runInstallWithTelemetry.
// Parameters: toolName, version (the resolved version to install), constraint (the Requested pin).
type InstallFunc func(toolName, version, constraint string) error

// MaybeAutoApply reads cached update check entries and installs pending updates.
// It uses a non-blocking TryLock on state.json.lock to avoid blocking when another
// tsuku process is running. If the lock can't be acquired, auto-apply silently skips
// and the cached entries persist for the next command invocation.
//
// On install failure, auto-rollback activates the previous version and writes a
// failure notice to $TSUKU_HOME/notices/.
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config, installFn InstallFunc) {
	if userCfg == nil || !userCfg.UpdatesAutoApplyEnabled() {
		return
	}

	cacheDir := CacheDir(cfg.HomeDir)
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	// Read cached check entries
	entries, err := ReadAllEntries(cacheDir)
	if err != nil {
		log.Default().Debug("auto-apply: read cache entries", "error", err)
		return
	}

	// Filter for actionable entries
	var pending []UpdateCheckEntry
	for _, e := range entries {
		if e.LatestWithinPin != "" && e.Error == "" && e.LatestWithinPin != e.ActiveVersion {
			pending = append(pending, e)
		}
	}

	if len(pending) == 0 {
		return
	}

	// Probe state lock (non-blocking): check if another tsuku process is running.
	// If the lock is held, skip auto-apply entirely. If free, release immediately
	// and proceed -- the install flow's own per-operation locking handles safety.
	lockPath := filepath.Join(cfg.HomeDir, "state.json.lock")
	lock := install.NewFileLock(lockPath)
	acquired, err := lock.TryLockExclusive()
	if err != nil {
		log.Default().Debug("auto-apply: try lock", "error", err)
		return
	}
	if !acquired {
		return
	}
	// Release probe lock immediately -- install flow acquires its own locks
	_ = lock.Unlock()

	mgr := install.New(cfg)

	for _, entry := range pending {
		// Read current active version for rollback target
		ts, _ := mgr.GetState().GetToolState(entry.Tool)
		var previousVersion string
		if ts != nil {
			previousVersion = ts.ActiveVersion
		}

		result := applyUpdate(entry, installFn)

		if result.err != nil {
			// Auto-rollback: activate previous version
			if previousVersion != "" {
				if rollbackErr := mgr.Activate(entry.Tool, previousVersion); rollbackErr != nil {
					log.Default().Debug("auto-apply: rollback failed",
						"tool", entry.Tool, "error", rollbackErr)
				}
			}

			// Write failure notice
			notice := &notices.Notice{
				Tool:             entry.Tool,
				AttemptedVersion: entry.LatestWithinPin,
				Error:            result.err.Error(),
				Timestamp:        time.Now(),
				Shown:            false,
			}
			_ = notices.WriteNotice(noticesDir, notice)
		}

		// Remove consumed cache entry regardless of success/failure
		_ = RemoveEntry(cacheDir, entry.Tool)
	}

	// Display unshown notices on stderr (one-time)
	displayUnshownNotices(noticesDir)
}

// applyResult captures the outcome of a single update attempt.
type applyResult struct {
	err error
}

// applyUpdate attempts to install a single tool update via the callback.
func applyUpdate(entry UpdateCheckEntry, installFn InstallFunc) applyResult {
	if err := installFn(entry.Tool, entry.LatestWithinPin, entry.Requested); err != nil {
		return applyResult{
			err: fmt.Errorf("install %s@%s: %w", entry.Tool, entry.LatestWithinPin, err),
		}
	}
	return applyResult{}
}

// displayUnshownNotices reads and displays any unshown failure notices on stderr.
func displayUnshownNotices(noticesDir string) {
	unshown, err := notices.ReadUnshownNotices(noticesDir)
	if err != nil || len(unshown) == 0 {
		return
	}

	for _, n := range unshown {
		fmt.Fprintf(os.Stderr, "\nUpdate failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details, 'tsuku rollback %s' to revert.\n", n.Tool)
		_ = notices.MarkShown(noticesDir, n.Tool)
	}
}
