<!-- decision:start id="notification-timing-and-types" status="assumed" -->
### Decision: Notification timing, types, formatting, and data sources

**Context**

Tsuku needs to display four categories of notifications from the auto-update system. Two categories already exist (failure notices, self-update results) and two are new (tool update applied successfully, tool updates available). The challenge is where in Cobra's lifecycle these render, given competing constraints: the PRD says available-update messages should appear "after the primary command's output," but Cobra's PersistentPostRun doesn't fire when a command returns an error, and auto-apply results happen during PersistentPreRun (before the command runs).

The existing auto-apply design (DESIGN-auto-apply-rollback.md, Decision 1) already evaluated and rejected PostRun for auto-apply execution itself, citing stale command output and the error-path gap. That reasoning applies equally to notifications produced by auto-apply. The question is whether "available update" summaries (a new notification type with no existing implementation) can use PostRun even if auto-apply results stay in PreRun.

**Assumptions**

- Cobra's PersistentPostRun non-firing on error is a stable, documented behavior that won't change. If it did change in a future Cobra version, Option B would become viable, but the risk of silent notification loss in the meantime is unacceptable.
- Users tolerate "available update" messages appearing before command output. Tools like Homebrew show update reminders at the start of commands, and this is a well-established UX pattern.
- The PRD's "after the primary command's output" language for R12 is a preference, not a hard requirement. The PRD doesn't address the Cobra PostRun error-path gap, suggesting the author assumed PostRun was reliable.
- PR #2199's pattern (DisplayUnshownNotices called independently in PersistentPreRun, outside MaybeAutoApply) is the direction the codebase is already heading.

**Chosen: Option A -- Split timing with PreRun as primary, PostRun as best-effort supplement**

All four notification types render in PersistentPreRun by default, using the existing `displayUnshownNotices` pattern extended to cover new types. This handles auto-apply results (success and failure), self-update results, and "available update" summaries. The unified PreRun rendering point means notifications always display, even when the subsequent command fails.

For "available update" summaries specifically, an optional PostRun hook can re-check whether any un-rendered summaries remain and display them there. This is a best-effort enhancement, not a requirement. The PostRun hook fires only on successful command exit, so it supplements PreRun rather than replacing it.

The four notification types, their data sources, and formatting:

| Type | Data Source | Format | Dedup |
|------|------------|--------|-------|
| Tool update applied | Success path in MaybeAutoApply (no notice file; inline print) | `Updated <tool> <old> -> <new>` | Per-tool, consumed on apply |
| Tool update failed | `$TSUKU_HOME/notices/<tool>.json` (Notice struct, Error non-empty) | `Update failed: <tool> -> <version>: <error>\n  Run 'tsuku notices' for details` | MarkShown flag, one-time display |
| Self-update result | `$TSUKU_HOME/notices/self-update.json` (Notice struct, success or failure) | Success: `tsuku updated to <version>`. Failure: `tsuku self-update failed: <error>` | MarkShown flag, one-time display |
| Update available | `$TSUKU_HOME/cache/updates/<tool>.json` (UpdateCheckEntry, LatestWithinPin set) | `Updates available: <tool> <current> -> <latest> (N tools total)\n  Run 'tsuku update' to apply` | Aggregated summary, shown once per check cycle via a separate shown-flag or sentinel |

Deduplication works through two mechanisms: (1) the existing MarkShown flag on Notice files for failure/self-update types, and (2) a separate "last-notified" sentinel for available-update summaries that resets when new check results arrive.

**Rationale**

Option A wins because it handles the error-path gap without complexity. The core constraint is that Cobra's PersistentPostRun is unreliable -- it silently drops notifications when commands fail. For auto-apply results this is particularly bad: a user runs `tsuku install foo`, the install fails, and they never see that `ripgrep` was auto-updated (or that the update failed and rolled back). PreRun guarantees display.

The PRD's "after command output" language is aspirational but doesn't account for Cobra's execution model. Splitting timing (Option A) gives the best of both worlds: PreRun guarantees all notifications are seen, and PostRun optionally catches the "after output" case for available-update summaries on happy paths. This is strictly better than Option B (all-PostRun) because it doesn't lose notifications on error, and strictly better than Option C (all-PreRun with no PostRun) because it can still place available-update summaries after output when possible.

The codebase is already moving in this direction. PR #2199 keeps `displayUnshownNotices` in PersistentPreRun and extends it for self-update notices. Option A formalizes that pattern and adds the two missing notification types.

**Alternatives Considered**

- **Option B: All notifications in PostRun via deferred rendering.** Auto-apply writes results to notice files during PreRun, then a single PostRun handler renders everything after command output. Rejected because Cobra's PersistentPostRun doesn't fire when the command returns an error. This creates a silent notification loss path that's hard to detect and impossible to fix without patching Cobra or wrapping os.Exit. Workarounds like `defer` in main() or runtime.SetFinalizer are fragile and don't compose well with Cobra's error handling. The error path is precisely when notifications matter most (failed auto-apply with rollback).

- **Option C: All notifications in PreRun, no PostRun involvement.** Keep everything in PersistentPreRun. Simplest option and fully reliable. Rejected in favor of Option A only because Option A is a strict superset -- it uses PreRun as the primary path (same as C) but adds an optional PostRun supplement for available-update summaries. Option C is the fallback if PostRun adds too much complexity during implementation; the design is compatible with dropping the PostRun piece entirely.

**Consequences**

Users see auto-apply results (success and failure) and self-update results before the command they typed executes. This is standard for package managers (Homebrew, apt) but may feel unusual to users who expect notifications only after their command finishes. The mitigation is clear formatting: a blank line separator and consistent prefix make it obvious these are system messages, not command output.

"Available update" summaries appear before command output by default, with best-effort PostRun display on happy paths. If the PostRun supplement proves confusing (duplicate display risk), it can be dropped without affecting the core design.

The notification framework adds a new `internal/notifications/` package (or extends `internal/notices/`) with a `Renderer` type that encapsulates suppression logic, TTY detection, and formatting for all four types. This replaces the ad-hoc `fmt.Fprintf` calls in `displayUnshownNotices`.
<!-- decision:end -->
