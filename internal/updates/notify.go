package updates

import (
	"fmt"
	"os"
	"strings"
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

	// Background auto-apply success notices surface here under an unrelated
	// foreground command (they were not shown inline at apply time, unlike
	// foreground successes which are now written Shown=true). Group them under
	// one header so they read as background activity rather than the current
	// command's own output. Everything else (failures, self-update, library,
	// single-view kinds) renders individually below.
	background, rest := partitionBackgroundSuccess(unshown)
	renderBackgroundSuccess(noticesDir, background)

	for _, n := range rest {
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
		case strings.HasPrefix(n.Tool, notices.LibraryNoticePrefix):
			renderLibraryNotice(n)
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

// partitionBackgroundSuccess splits unshown notices into background auto-apply
// successes and everything else. A background success is a tool-level success
// notice (no Error) tagged KindAutoApplyResult — i.e. produced by the
// background auto-apply subprocess, which prints nothing inline. Library and
// self-update notices are excluded so they keep their existing per-notice
// rendering. Factored out so #2409's failure-placement work can reuse the same
// partition for its own head-of-output block.
func partitionBackgroundSuccess(in []notices.Notice) (background, rest []notices.Notice) {
	for _, n := range in {
		if isBackgroundSuccess(n) {
			background = append(background, n)
		} else {
			rest = append(rest, n)
		}
	}
	return background, rest
}

// isBackgroundSuccess reports whether a notice is a background auto-apply
// success for a regular tool (the batch that otherwise prints bare lines at the
// head of an unrelated command).
func isBackgroundSuccess(n notices.Notice) bool {
	return n.Error == "" &&
		n.Kind == notices.KindAutoApplyResult &&
		n.Tool != SelfToolName &&
		!strings.HasPrefix(n.Tool, notices.LibraryNoticePrefix)
}

// renderBackgroundSuccess prints the grouped "Background updates applied:"
// block and marks each notice shown. A no-op when there are no background
// successes.
func renderBackgroundSuccess(noticesDir string, ns []notices.Notice) {
	if len(ns) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\nBackground updates applied:\n")
	for _, n := range ns {
		switch n.Verb {
		case notices.VerbInstall:
			fmt.Fprintf(os.Stderr, "  %s installed (%s)\n", n.Tool, n.AttemptedVersion)
		case notices.VerbRollback:
			fmt.Fprintf(os.Stderr, "  %s rolled back to %s\n", n.Tool, n.AttemptedVersion)
		default: // VerbUpdate or empty (legacy)
			fmt.Fprintf(os.Stderr, "  %s -> %s\n", n.Tool, n.AttemptedVersion)
		}
		_ = notices.MarkShown(noticesDir, n.Tool)
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

// renderLibraryNotice formats the user-facing message for a library
// lifecycle notice. Library notices are stored with the lib-- prefix
// on disk so they do not collide with tool notices; the renderer strips
// the prefix before display. The Verb field selects phrasing; library
// installs have no Updated variant today, so VerbUpdate and an empty
// Verb both render as install.
func renderLibraryNotice(n notices.Notice) {
	library := strings.TrimPrefix(n.Tool, notices.LibraryNoticePrefix)
	if n.Error == "" {
		switch n.Verb {
		case notices.VerbRemove:
			// Successful library remove is reported via RemoveNotice,
			// not WriteNotice, so this branch shouldn't normally fire.
			// Stay quiet to keep output clean if a stray success notice
			// with VerbRemove appears.
		default: // VerbInstall or empty (legacy / future-compat)
			fmt.Fprintf(os.Stderr, "\n%s library has been installed (%s)\n", library, n.AttemptedVersion)
		}
		return
	}
	// Failure path: per-verb framing.
	switch n.Verb {
	case notices.VerbRemove:
		fmt.Fprintf(os.Stderr, "\nLibrary remove failed: %s %s: %s\n", library, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details.\n")
	default: // VerbInstall or empty
		fmt.Fprintf(os.Stderr, "\nLibrary install failed: %s -> %s: %s\n", library, n.AttemptedVersion, n.Error)
		fmt.Fprintf(os.Stderr, "  Run 'tsuku notices' for details.\n")
	}
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
