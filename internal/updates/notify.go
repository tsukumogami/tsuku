package updates

import (
	"fmt"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// NotifiedSentinel is the filename used to track when available-update
// summaries were last shown. Lives in the cache/updates/ directory.
const NotifiedSentinel = ".notified"

// DisplayNotifications renders all pending notification types to stderr.
// Replaces the former displayUnshownNotices. Called from PersistentPreRun.
//
// Notification types rendered (in order):
//  1. Auto-apply results (success and failure) from MaybeAutoApply
//  2. Unshown notices (self-update results, tool update failures from prior runs)
//  3. Available-update summary (when auto_apply is disabled)
func DisplayNotifications(cfg *config.Config, userCfg *userconfig.Config, quiet bool, results []ApplyResult) {
	if ShouldSuppressNotifications(quiet) {
		return
	}

	noticesDir := notices.NoticesDir(cfg.HomeDir)
	cacheDir := CacheDir(cfg.HomeDir)

	// 1. Render auto-apply results from this invocation
	for _, r := range results {
		if r.Err == nil {
			fmt.Fprintf(os.Stderr, "Updated %s %s -> %s\n", r.Tool, r.OldVersion, r.NewVersion)
		} else {
			fmt.Fprintf(os.Stderr, "\nUpdate failed: %s -> %s: %s\n", r.Tool, r.NewVersion, r.Err)
			fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details, 'tsuku rollback %s' to revert.\n", r.Tool)
		}
	}

	// 2. Render unshown notices (from prior runs: self-update, tool failures)
	renderUnshownNotices(noticesDir)

	// 3. Render available-update summary (when auto_apply is disabled)
	if userCfg != nil && !userCfg.UpdatesAutoApplyEnabled() {
		renderAvailableSummary(cacheDir)
	}

	// 4. Render out-of-channel notifications (when enabled)
	if userCfg != nil && userCfg.UpdatesNotifyOutOfChannel() {
		renderOutOfChannelNotifications(cacheDir, time.Now())
	}
}

// DisplayAvailableSummary renders available-update summaries to stderr.
// Best-effort supplement called from PersistentPostRun on successful exits.
func DisplayAvailableSummary(cfg *config.Config, userCfg *userconfig.Config, quiet bool) {
	if ShouldSuppressNotifications(quiet) {
		return
	}
	if userCfg == nil || userCfg.UpdatesAutoApplyEnabled() {
		return
	}
	renderAvailableSummary(CacheDir(cfg.HomeDir))
}

// renderUnshownNotices reads and displays any unshown notices on stderr.
// KindVersionFallback and KindShellInitChange notices are single-view: they are
// removed after display. All other kinds use the Shown=true persistence convention.
//
// Notice.Verb selects the user-facing message: install/update/rollback/remove.
// Empty Verb falls back to the legacy "updated to" / "Update failed" phrasing
// so notice files written before this field was introduced still render.
func renderUnshownNotices(noticesDir string) {
	unshown, err := notices.ReadUnshownNotices(noticesDir)
	if err != nil || len(unshown) == 0 {
		return
	}

	for _, n := range unshown {
		switch {
		case n.Kind == notices.KindVersionFallback:
			fmt.Fprintf(os.Stderr, "\nWarning: %s\n", n.Tool)
			for _, msg := range n.Messages {
				fmt.Fprintf(os.Stderr, "  %s\n", msg)
			}
		case n.Kind == notices.KindShellInitChange:
			fmt.Fprintf(os.Stderr, "\nNote: shell init changed for %s\n", n.Tool)
			for _, msg := range n.Messages {
				fmt.Fprintf(os.Stderr, "  %s\n", msg)
			}
		case n.Tool == SelfToolName:
			renderSelfUpdateNotice(n)
		default:
			renderToolNotice(n)
		}

		// Single-view kinds are removed after display; others are marked shown.
		if n.Kind == notices.KindVersionFallback || n.Kind == notices.KindShellInitChange {
			_ = notices.RemoveNotice(noticesDir, n.Tool)
		} else {
			_ = notices.MarkShown(noticesDir, n.Tool)
		}
	}
}

// renderSelfUpdateNotice formats the user-facing message for a tsuku
// self-update notice. Self-update uses the same verb vocabulary as
// regular tool notices but with custom phrasing for the success case
// (the binary itself was updated, not a tool under $TSUKU_HOME).
func renderSelfUpdateNotice(n notices.Notice) {
	if n.Error == "" {
		// Empty Verb is treated as update for backward compat.
		fmt.Fprintf(os.Stderr, "\ntsuku has been updated to %s\n", n.AttemptedVersion)
		return
	}
	fmt.Fprintf(os.Stderr, "\ntsuku self-update failed: %s\n", n.Error)
	fmt.Fprintf(os.Stderr, "  Run 'tsuku self-update' to retry.\n")
}

// renderToolNotice formats the user-facing message for a tool-lifecycle
// notice. The Verb field selects phrasing; empty Verb falls back to
// today's "updated to" / "Update failed" wording.
func renderToolNotice(n notices.Notice) {
	if n.Error == "" {
		switch n.Verb {
		case notices.VerbInstall:
			fmt.Fprintf(os.Stderr, "\n%s has been installed (%s)\n", n.Tool, n.AttemptedVersion)
		case notices.VerbRollback:
			fmt.Fprintf(os.Stderr, "\n%s has been rolled back to %s\n", n.Tool, n.AttemptedVersion)
		case notices.VerbRemove:
			// Successful remove is reported via RemoveNotice, not WriteNotice,
			// so this branch shouldn't normally fire. Render nothing to keep
			// the output clean if a notice with Verb=remove and no Error
			// somehow gets written.
		default: // VerbUpdate or empty (legacy)
			fmt.Fprintf(os.Stderr, "\n%s has been updated to %s\n", n.Tool, n.AttemptedVersion)
		}
		return
	}
	// Failure path: per-verb framing.
	switch n.Verb {
	case notices.VerbInstall:
		fmt.Fprintf(os.Stderr, "\nInstall failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details.\n")
	case notices.VerbRollback:
		fmt.Fprintf(os.Stderr, "\nRollback failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details.\n")
	case notices.VerbRemove:
		fmt.Fprintf(os.Stderr, "\nRemove failed: %s %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details.\n")
	default: // VerbUpdate or empty (legacy)
		fmt.Fprintf(os.Stderr, "\nUpdate failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details, 'tsuku rollback %s' to revert.\n", n.Tool)
	}
}

// renderAvailableSummary counts tools with available updates from cache entries
// and prints an aggregated summary. Uses a sentinel file for deduplication so the
// summary shows once per check cycle rather than on every command.
func renderAvailableSummary(cacheDir string) {
	sentinelPath := cacheDir + "/" + NotifiedSentinel

	// Check if we already showed this cycle's results
	if !isSentinelStale(cacheDir, sentinelPath) {
		return
	}

	entries, err := ReadAllEntries(cacheDir)
	if err != nil {
		return
	}

	var count int
	for _, e := range entries {
		if e.LatestWithinPin != "" && e.Error == "" && e.LatestWithinPin != e.ActiveVersion {
			count++
		}
	}

	if count == 0 {
		return
	}

	if count == 1 {
		fmt.Fprintf(os.Stderr, "\n1 update available. Run 'tsuku update' to apply.\n")
	} else {
		fmt.Fprintf(os.Stderr, "\n%d updates available. Run 'tsuku update' to apply.\n", count)
	}

	// Touch sentinel so we don't repeat until the next check cycle
	touchSentinel(sentinelPath)
}

// isSentinelStale returns true if the sentinel file is older than the cache
// directory (meaning new check results have arrived) or doesn't exist.
func isSentinelStale(cacheDir, sentinelPath string) bool {
	sentinelInfo, err := os.Stat(sentinelPath)
	if err != nil {
		return true // sentinel doesn't exist, show summary
	}

	dirInfo, err := os.Stat(cacheDir)
	if err != nil {
		return false // can't stat cache dir, don't show
	}

	return dirInfo.ModTime().After(sentinelInfo.ModTime())
}

// touchSentinel creates or updates the sentinel file's mtime.
func touchSentinel(path string) {
	f, err := os.Create(path)
	if err == nil {
		f.Close()
	}
}

// renderOutOfChannelNotifications checks cache entries for tools where a newer
// version exists outside the pin boundary and shows a notification if not throttled.
func renderOutOfChannelNotifications(cacheDir string, now time.Time) {
	entries, err := ReadAllEntries(cacheDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		// Skip entries without overall version data
		if e.LatestOverall == "" || e.Error != "" {
			continue
		}
		// Skip if overall matches within-pin (no out-of-channel version)
		if e.LatestOverall == e.LatestWithinPin || e.LatestOverall == e.ActiveVersion {
			continue
		}
		// Skip if within-pin is empty and overall equals active (already current)
		if e.LatestWithinPin == "" && e.LatestOverall == e.ActiveVersion {
			continue
		}

		// Check throttle
		if IsOOCThrottled(cacheDir, e.Tool, now) {
			continue
		}

		pin := e.Requested
		if pin == "" {
			pin = "latest"
		}
		fmt.Fprintf(os.Stderr, "\n%s %s available (pinned to %s)\n", e.Tool, e.LatestOverall, pin)
		_ = TouchOOCThrottle(cacheDir, e.Tool)
	}
}
