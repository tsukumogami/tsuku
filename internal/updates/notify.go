package updates

import (
	"fmt"
	"os"

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
func renderUnshownNotices(noticesDir string) {
	unshown, err := notices.ReadUnshownNotices(noticesDir)
	if err != nil || len(unshown) == 0 {
		return
	}

	for _, n := range unshown {
		if n.Tool == SelfToolName && n.Error == "" {
			// Self-update success
			fmt.Fprintf(os.Stderr, "\ntsuku has been updated to %s\n", n.AttemptedVersion)
		} else if n.Tool == SelfToolName {
			// Self-update failure
			fmt.Fprintf(os.Stderr, "\ntsuku self-update failed: %s\n", n.Error)
			fmt.Fprintf(os.Stderr, "  Run 'tsuku self-update' to retry.\n")
		} else if n.Error == "" {
			// Tool update success from background
			fmt.Fprintf(os.Stderr, "\n%s has been updated to %s\n", n.Tool, n.AttemptedVersion)
		} else {
			// Tool update failure
			fmt.Fprintf(os.Stderr, "\nUpdate failed: %s -> %s: %s\n", n.Tool, n.AttemptedVersion, n.Error)
			fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details, 'tsuku rollback %s' to revert.\n", n.Tool)
		}
		_ = notices.MarkShown(noticesDir, n.Tool)
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
