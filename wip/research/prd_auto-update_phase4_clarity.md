# Clarity Review

## Verdict: PASS
The PRD is well-structured with specific, testable requirements, but contains several ambiguities that could lead to divergent implementations in edge cases.

## Ambiguities Found

1. **R1 (Version channel pinning):** "An empty string or 'latest' tracks the latest stable version" -> What defines "stable"? Does this mean non-pre-release semver tags? Some providers (e.g., npm, PyPI) have their own stability conventions. Two developers could disagree on whether a release candidate from PyPI counts as stable. -> Clarify: define "stable" as "the version the provider's ResolveLatest returns, which already excludes pre-releases" or specify the filtering rule explicitly.

2. **R1 (Version channel pinning):** "The number of dot-separated components in the Requested field determines the pin level" -> CalVer versions (e.g., `2024.1`) have two components but semantically represent an exact release, not a "major.minor" range. D2 acknowledges this as an "acceptable edge case" but doesn't say what happens. A developer pinning `ubuntu@24.04` might expect exact-pin behavior but get auto-updates within 24.04.z. -> Clarify: state explicitly whether calver tools need recipe-level metadata to override the dot-counting heuristic, or document the expected (possibly wrong) behavior as a known limitation.

3. **R3 (Automatic update application):** "The update happens during a tsuku command invocation as a non-blocking background operation. The user sees the result on the current or next invocation." -> Does "non-blocking background operation" mean the update downloads and installs in a background goroutine while the foreground command runs? What if the foreground command is `tsuku install foo` and the background auto-update is also modifying state.json? The concurrency model for state writes is unspecified. -> Clarify: state whether auto-update can run concurrently with state-mutating foreground commands, or whether it only runs during read-only commands (e.g., `tsuku list`).

4. **R3 (Automatic update application):** "The user sees the result on the current or next invocation" -> "Current or next" is two different behaviors. If the background goroutine finishes before the foreground command exits, does the notification appear immediately? If not, is the result persisted to disk for next invocation? The mechanism for "current invocation" notification vs. deferred notification is unclear. -> Clarify: specify that if the goroutine completes before the command exits, the notification is printed to stderr at command exit; otherwise, results are written to a cache file and displayed on the next invocation.

5. **R5 (Non-blocking checks):** "Update checks run as a background goroutine within the tsuku process" -> R3 says updates are also applied in the background. R5 only mentions "checks." Are checks and application both background, or is application synchronous after a check completes? -> Clarify: distinguish between the check phase (always background) and the apply phase (background? synchronous? deferred to next invocation?).

6. **R7 (Self-update):** "renames the old binary aside" -> Where? What filename? The acceptance criteria say `tsuku.old` but R7 doesn't specify the name or location. -> Clarify: state the backup path explicitly in R7 (e.g., `$TSUKU_HOME/bin/tsuku.old` or alongside the current binary).

7. **R11 (Deferred failure reporting):** "Transient failures (single network timeout) are suppressed. Only persistent failures (3+ consecutive)" -> The parenthetical says "single" but the threshold is "3+ consecutive," leaving the behavior for exactly 2 consecutive failures undefined. Is 2 transient or persistent? -> Clarify: state the threshold explicitly: "Failures with fewer than 3 consecutive occurrences are considered transient and suppressed."

8. **R12 (Update notifications):** "Notifications are suppressed when stdout is not a TTY" -> Should this be stderr? Notifications go to stderr (stated in the same requirement). Checking stdout's TTY status to decide whether to write to stderr is a valid pattern (many CLIs do this) but could confuse implementers. -> Clarify: state explicitly "notifications are written to stderr but suppressed when stdout is not a TTY (indicating piped or scripted usage)."

9. **R18 (Old version retention):** "at least one auto-update cycle (configurable, default 7 days)" -> "At least one auto-update cycle" and "7 days" could differ. If the check interval is set to 30 days, is retention 7 days or 30 days? Which takes precedence? -> Clarify: define retention as `max(check_interval, configured_retention_days)` or simply use the configured retention period independent of the check interval.

10. **R9 (Rollback):** "switches to the previously active version" -> "Previously active" is ambiguous when multiple auto-updates have occurred. If a tool went v1.0 -> v1.1 -> v1.2 via auto-update, does rollback go to v1.1 or v1.0? How many previous versions does rollback track? -> Clarify: state that rollback targets the immediately preceding version (one step back), and that further rollback requires `tsuku install tool@version`.

11. **Acceptance criteria, Failure handling:** "tsuku doctor detects orphaned staging directories and stale notices" -> "Stale" is subjective. How old is stale? What action does doctor take (report only, or clean up)? -> Clarify: define stale (e.g., "notices older than 30 days") and specify whether doctor reports or remediates.

12. **Phase 1 vs Phase 2 split:** R15 appears in both Phase 1 (as "Fix tsuku outdated to use ProviderFactory") and Phase 2 (as "Pin-aware outdated display with dual columns"). The acceptance criteria for R15 include dual columns. It's unclear whether Phase 1 delivers basic all-provider outdated without dual columns, or the full R15. -> Clarify: split R15 into R15a (all-provider resolution, Phase 1) and R15b (dual-column display, Phase 2), or rewrite the phasing to make the split explicit.

13. **Configuration, acceptance criteria:** "Precedence order: CLI flag > env var > .tsuku.toml > config.toml > default" -> The CLI flag that overrides update behavior is not specified anywhere in the requirements. R4 mentions `--check-updates` but there's no `--no-auto-update` or similar flag defined. -> Clarify: enumerate the specific CLI flags that participate in this precedence chain.

14. **D1 (Decisions):** "Users who want notification-only can set updates.auto_apply = false in config.toml" -> This config key is not listed in the acceptance criteria's config.toml `[updates]` section, which only lists `enabled`, `check_interval`, `notify_out_of_channel`, `self_update`. -> Clarify: add `auto_apply` to the acceptance criteria config keys, or remove the mention from D1.

## Suggested Improvements

1. **Add a glossary of pin levels with examples**: A table showing input -> Requested field -> pin level -> update behavior for 5-6 concrete examples (including edge cases like calver, single-component like "latest", and tools without versions) would eliminate most pinning ambiguity.

2. **Specify the concurrency model for background updates**: The PRD should state whether background auto-apply can run during any tsuku command or only during specific commands, and how state.json write conflicts are prevented (file locking, single-writer constraint, etc.).

3. **Add a state machine or sequence diagram for auto-update lifecycle**: The flow from "check interval elapsed" through "check" -> "resolve" -> "download" -> "verify" -> "install" -> "notify" has several branch points (failure, cache hit, offline). A diagram would make the expected behavior unambiguous.

4. **Reconcile the config.toml keys between D1 and acceptance criteria**: The `auto_apply` key mentioned in D1 is missing from the acceptance criteria. Either add it or remove the D1 reference.

5. **Define "previously active version" precisely**: State whether the system tracks one previous version or a version history, and what rollback means after multiple sequential auto-updates.

## Summary

This PRD is above average in specificity. The requirements are mostly concrete, the acceptance criteria are largely binary, and the phasing is clear. The main gaps are around concurrency semantics (background check vs. background apply, state.json contention), edge cases in the pin-level heuristic (calver), and a few config/threshold inconsistencies between the requirements text and acceptance criteria. Fixing these 14 ambiguities would bring the document to implementation-ready quality.
