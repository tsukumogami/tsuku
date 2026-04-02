# DESIGN: Update polish

## Status

Proposed

## Context and Problem Statement

The auto-update system's core is complete (Features 1-5), but three UX gaps remain. First, `tsuku outdated` only shows the latest version within the pin boundary. Users pinned to `node@20` don't see that Node 22 exists unless they check manually. The PRD calls for dual columns showing both "within pin" and "overall" versions (R15b).

Second, there's no proactive notification when a newer version exists outside the user's pin. A developer pinned to Go 1.21 gets patches automatically but has no idea Go 1.23 shipped months ago. Out-of-channel notifications (R13) solve this, but they need per-tool weekly throttling to avoid nagging. The throttle requires persistent state and an injectable clock for testing.

Third, `tsuku update` takes exactly one tool name. Updating all tools requires running the command once per tool. `tsuku update --all` (R14) is a simple batch operation that iterates installed tools and updates each within its pin boundary.

This is Feature 6 of the [auto-update roadmap](../roadmaps/ROADMAP-auto-update.md), implementing PRD requirements R13, R14, and R15b. All dependencies are complete: Feature 1 (version resolution), Feature 3 (auto-apply), and Feature 5 (notification system).

## Decision Drivers

- **Existing infrastructure.** The `UpdateCheckEntry` cache already has `LatestWithinPin` and `LatestOverall` fields. `outdated.go` resolves within-pin but ignores overall. The notification system (`ShouldSuppressNotifications`, `DisplayNotifications`) is in place.
- **Weekly throttle needs persistence.** Out-of-channel notifications must appear at most once per week per tool (R13). This requires storing the last notification timestamp per tool somewhere that survives across invocations.
- **Testability.** The weekly throttle's time dependency must be injectable for testing. Tests can't wait a week or manipulate system clocks.
- **Minimal new surface.** These are polish features, not new infrastructure. The design should reuse existing patterns (cache files, config keys, notification rendering) rather than introducing new concepts.
- **Backward compatibility.** `tsuku outdated` and `tsuku update` have existing users. New columns and flags shouldn't break scripts that parse the current output. JSON output is the stable contract; text output can change.
