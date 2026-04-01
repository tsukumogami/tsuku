# DESIGN: Notification system

## Status

Proposed

## Context and Problem Statement

Tsuku's auto-update system (Features 1-4) can check for updates, apply them, and self-update the binary. But it has no coherent notification layer. The current `DisplayUnshownNotices` function in `internal/updates/apply.go` handles failure notices and self-update results, but it runs unconditionally on stderr with no suppression logic, no TTY awareness, and no "update available" messages for the common case where `auto_apply = false`.

Three gaps exist:

1. **No suppression.** Notifications print to stderr regardless of context. CI pipelines, piped output (`tsuku list | grep node`), and `--quiet` mode all see update noise. The PRD requires layered suppression: non-TTY stdout, `CI=true`, `--quiet`, and `TSUKU_NO_UPDATE_CHECK=1` (R12, R16).

2. **No "available update" notifications.** When `auto_apply = false`, users who check for updates but don't auto-apply get no indication that updates exist. The cached check results sit unused until the user runs `tsuku outdated` manually.

3. **No shared framework.** Self-update success, self-update failure, tool update failure, and tool update available are four distinct notification types with different formatting needs. They're currently handled by ad-hoc `fmt.Fprintf` calls with no common structure.

This is Feature 5 of the [auto-update roadmap](../roadmaps/ROADMAP-auto-update.md), implementing PRD requirements R12 (update notifications) and R16 (CI environment detection). It depends on Feature 2 (check infrastructure, done) and Feature 3 (auto-apply with rollback, done).

## Decision Drivers

- **CI safety is non-negotiable.** CI pipelines must never see unexpected stderr output from update notifications. The `CI=true` convention is standard across GitHub Actions, GitLab CI, CircleCI, and others. Suppression must be the default in these environments.
- **Explicit opt-in overrides suppression.** `TSUKU_AUTO_UPDATE=1` must override CI detection so users who want update behavior in CI can get it (PRD R16).
- **Notifications go to stderr, suppression checks stdout.** The PRD specifies suppression "when stdout is not a TTY" even though notifications write to stderr. This is deliberate: piped stdout means scripted context, so stderr noise should be suppressed too.
- **Existing infrastructure to build on.** The `internal/notices/` package, `DisplayUnshownNotices`, `UpdateCheckEntry` cache, and the `quietFlag` global in `cmd/tsuku/main.go` all exist. The design should extend these, not replace them.
- **Feature 7 (resilience) adds consecutive-failure suppression later.** This design handles one-shot display logic. Consecutive-failure counting is out of scope.
- **Self-update PR (#2199) changes.** PR #2199 moves `DisplayUnshownNotices` to be called independently from PersistentPreRun (outside the MaybeAutoApply gate) and adds self-update-specific formatting. This design should subsume and formalize that pattern.
