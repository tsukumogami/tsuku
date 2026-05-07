---
status: Proposed
problem: |
  When tsuku runs in background auto-update mode, all warnings and non-fatal events
  from the install engine are silently dropped because the subprocess uses a reporter
  that writes to /dev/null. Users never learn that a version fallback occurred or that
  their tool installed a different version than expected. The fix is a context-aware
  notification routing system: the same reporter call routes to the terminal when
  running interactively, and to the notices inbox when running in the background.
---

# DESIGN: Notification Routing

## Status

Proposed

## Context and Problem Statement

tsuku has two execution contexts for updates: interactive (`tsuku update <tool>`, with a terminal) and background auto-apply (the `apply-updates` subprocess, no terminal). Today, the background path constructs a TTY reporter against `/dev/null`, silently discarding all warnings, non-fatal errors, and progress. Users who rely on background auto-apply have no visibility into events like "version fallback occurred" or "shell init changed" unless an install fails outright.

The notices system (`internal/notices/`) already handles failure persistence: background failures write JSON files to `$TSUKU_HOME/notices/` and surface on the next interactive command via `DisplayNotifications`. But this coverage is narrow — only hard failures reach the inbox. Non-fatal events are lost entirely.

The design addresses three related gaps:

1. **Silent background warnings**: The `apply-updates` subprocess needs a reporter implementation that routes `Warn`/`DeferWarn` calls to the notices inbox rather than a terminal.

2. **Success notices never written**: `renderUnshownNotices` already handles `n.Error == ""` notices (displaying "X updated to Y"), but `MaybeAutoApply` never writes a success notice. The display half exists; the write half doesn't.

3. **Version fallback with no user signal**: When `github_archive` picks a release whose asset doesn't exist and falls back to a previous version, no notice is produced. Users don't know they're running an older version than expected.

The principle: any event worth showing inline during an interactive update is worth recording in the inbox for the background path. The execution channel determines the sink; the same call site works in both contexts.

## Decision Drivers

- **No duplicate logic**: Reporter call sites in the install engine must not branch on "is this interactive?". The routing decision lives at reporter construction time, not at each call site.
- **Backward compatibility**: Existing notice files on disk (no `Kind` field set, `Error` field for lifecycle) must continue to display and clear correctly.
- **Atomic per-tool notice**: The current schema stores one JSON file per tool. Multi-warn events during a single install need a clear accumulation strategy.
- **Fallback correctness**: Version fallback during `Decompose` creates a stale `UpdateCheckEntry.LatestWithinPin` in the background checker's cache. The design must address cache staleness.
- **Lifecycle explicitness**: The current `Error != ""` convention for persistent vs. single-view is fragile. The `Kind` field should become the lifecycle routing key.

## Decisions Already Made

From exploration (not to be reopened by the design):

- **Version-fallback notices are single-view**: A successful fallback installs the tool at a working version. The user sees it once in `tsuku notices` then it clears — same semantics as a success notice. It is not a persistent error.
- **`InboxReporter` is the right abstraction**: A new `progress.Reporter` implementation that routes `Warn()`/`DeferWarn()` calls to `notices.WriteNotice()`. The background path switches to this reporter at the single construction point in `cmd_apply_updates.go` via `runInstallWithExternalReporter`. Zero call-site changes in `actions/`, `executor/`, or `install/`.
- **`Kind` must become load-bearing**: New Kind values (`KindVersionFallback`, `KindAutoApplySuccess`, etc.) should drive display and deletion behavior, replacing the `Error != ""` convention where Kind is set. The zero value (`KindUpdateResult`) is preserved for backward compatibility.
- **Version fallback belongs in `Decompose`, not the version provider or checker.go**: The version provider has no knowledge of asset patterns. `GitHubArchiveAction.Decompose` already calls `FetchReleaseAssets` for wildcard patterns and has `ctx.Resolver` available for `ListGitHubVersions`. The resulting cache staleness in `UpdateCheckEntry.LatestWithinPin` is accepted as a known tradeoff for a targeted fix.
