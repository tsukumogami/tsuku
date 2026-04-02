package updates

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// FailureSuppressionThreshold is the number of consecutive failures before
// a notice is shown to the user. Fewer than this = transient, suppressed.
const FailureSuppressionThreshold = 3

// InstallFunc is the callback type for the install flow.
// Injected by cmd/tsuku/main.go wrapping runInstallWithTelemetry.
// Parameters: toolName, version (the resolved version to install), constraint (the Requested pin).
type InstallFunc func(toolName, version, constraint string) error

// ApplyResult captures the outcome of a single auto-apply attempt.
// Returned by MaybeAutoApply for rendering by DisplayNotifications.
type ApplyResult struct {
	Tool       string
	OldVersion string
	NewVersion string
	Err        error
}

// effectivePin returns the version constraint to use for auto-apply filtering.
// Project config takes precedence over the cached global pin when the tool is declared.
func effectivePin(tool string, entry UpdateCheckEntry, projectCfg *project.ConfigResult) string {
	if projectCfg != nil && projectCfg.Config != nil {
		if req, ok := projectCfg.Config.Tools[tool]; ok {
			if err := install.ValidateRequested(req.Version); err != nil {
				log.Default().Debug("auto-apply: invalid project pin", "tool", tool, "version", req.Version, "error", err)
				return entry.Requested
			}
			return req.Version
		}
	}
	return entry.Requested
}

// MaybeAutoApply reads cached update check entries and installs pending updates.
// It returns results for each attempted update so callers can render notifications.
//
// It uses a non-blocking TryLock on state.json.lock to avoid blocking when another
// tsuku process is running. If the lock can't be acquired, auto-apply silently skips
// and the cached entries persist for the next command invocation.
//
// On install failure, auto-rollback activates the previous version and writes a
// failure notice to $TSUKU_HOME/notices/.
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config, projectCfg *project.ConfigResult, installFn InstallFunc, tc *telemetry.Client) []ApplyResult {
	if userCfg == nil || !userCfg.UpdatesAutoApplyEnabled() {
		return nil
	}

	cacheDir := CacheDir(cfg.HomeDir)
	noticesDir := notices.NoticesDir(cfg.HomeDir)

	// Read cached check entries
	entries, err := ReadAllEntries(cacheDir)
	if err != nil {
		log.Default().Debug("auto-apply: read cache entries", "error", err)
		return nil
	}

	// Filter for actionable entries
	var pending []UpdateCheckEntry
	for _, e := range entries {
		if IsSelfUpdate(&e) {
			continue
		}
		if e.LatestWithinPin != "" && e.Error == "" && e.LatestWithinPin != e.ActiveVersion {
			pending = append(pending, e)
		}
	}

	if len(pending) == 0 {
		return nil
	}

	// Apply project-level pin overrides
	if projectCfg != nil && projectCfg.Config != nil {
		var filtered []UpdateCheckEntry
		for _, entry := range pending {
			pin := effectivePin(entry.Tool, entry, projectCfg)
			pinLevel := install.PinLevelFromRequested(pin)
			if pinLevel == install.PinExact || pinLevel == install.PinChannel {
				log.Default().Debug("auto-apply: project pin suppresses update",
					"tool", entry.Tool, "pin", pin, "level", pinLevel)
				continue
			}
			if pin != "" && pin != "latest" && !install.VersionMatchesPin(entry.LatestWithinPin, pin) {
				log.Default().Debug("auto-apply: project pin narrows boundary",
					"tool", entry.Tool, "pin", pin, "candidate", entry.LatestWithinPin)
				continue
			}
			filtered = append(filtered, entry)
		}
		pending = filtered
		if len(pending) == 0 {
			return nil
		}
	}

	// Probe state lock (non-blocking): check if another tsuku process is running.
	// If the lock is held, skip auto-apply entirely. If free, release immediately
	// and proceed -- the install flow's own per-operation locking handles safety.
	lockPath := filepath.Join(cfg.HomeDir, "state.json.lock")
	lock := install.NewFileLock(lockPath)
	acquired, err := lock.TryLockExclusive()
	if err != nil {
		log.Default().Debug("auto-apply: try lock", "error", err)
		return nil
	}
	if !acquired {
		return nil
	}
	// Release probe lock immediately -- install flow acquires its own locks
	_ = lock.Unlock()

	mgr := install.New(cfg)
	var results []ApplyResult

	for _, entry := range pending {
		// Read current active version for rollback target
		ts, _ := mgr.GetState().GetToolState(entry.Tool)
		var previousVersion string
		if ts != nil {
			previousVersion = ts.ActiveVersion
		}

		result := applyUpdate(entry, installFn)

		ar := ApplyResult{
			Tool:       entry.Tool,
			OldVersion: entry.ActiveVersion,
			NewVersion: entry.LatestWithinPin,
			Err:        result.err,
		}
		results = append(results, ar)

		if result.err == nil {
			if tc != nil {
				tc.SendUpdateOutcome(telemetry.NewUpdateOutcomeSuccessEvent(
					entry.Tool, previousVersion, entry.LatestWithinPin, "auto"))
			}
			// Success resets the failure counter by removing the notice
			_ = notices.RemoveNotice(noticesDir, entry.Tool)
			// Garbage collect old version directories
			retention := userCfg.UpdatesVersionRetention()
			_ = GarbageCollectVersions(cfg.ToolsDir, entry.Tool, entry.LatestWithinPin, previousVersion, retention, time.Now())
		}

		if result.err != nil {
			// Emit failure event
			if tc != nil {
				tc.SendUpdateOutcome(telemetry.NewUpdateOutcomeFailureEvent(
					entry.Tool, entry.LatestWithinPin, telemetry.ClassifyError(result.err), "auto"))
			}

			// Auto-rollback: activate previous version
			if previousVersion != "" {
				if rollbackErr := mgr.Activate(entry.Tool, previousVersion); rollbackErr != nil {
					log.Default().Debug("auto-apply: rollback failed",
						"tool", entry.Tool, "error", rollbackErr)
				} else {
					if tc != nil {
						tc.SendUpdateOutcome(telemetry.NewUpdateOutcomeRollbackEvent(
							entry.Tool, previousVersion, entry.LatestWithinPin, "auto"))
					}
				}
			}

			// Write failure notice with consecutive-failure tracking
			consecutiveCount := 1
			if existing, _ := notices.ReadNotice(noticesDir, entry.Tool); existing != nil {
				consecutiveCount = existing.ConsecutiveFailures + 1
			}

			// Actionable errors bypass the suppression threshold
			if isActionableError(result.err) {
				consecutiveCount = FailureSuppressionThreshold
			}

			notice := &notices.Notice{
				Tool:                entry.Tool,
				AttemptedVersion:    entry.LatestWithinPin,
				Error:               result.err.Error(),
				Timestamp:           time.Now(),
				Shown:               consecutiveCount < FailureSuppressionThreshold,
				ConsecutiveFailures: consecutiveCount,
			}
			_ = notices.WriteNotice(noticesDir, notice)
		}

		// Remove consumed cache entry regardless of success/failure
		_ = RemoveEntry(cacheDir, entry.Tool)
	}

	return results
}

// applyResult captures the internal outcome of a single update attempt.
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

// isActionableError returns true for errors that should bypass the consecutive-
// failure suppression threshold and always produce a visible notice.
func isActionableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pattern := range []string{"checksum", "disk full", "no space", "recipe"} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}
