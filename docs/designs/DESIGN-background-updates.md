---
status: Proposed
problem: |
  tsuku runs full tool installs synchronously in PersistentPreRun before the
  command the user asked for, blocking even fast read-only commands like `tsuku
  list`. The update check is already non-blocking; the apply step is not. This
  document decides how to move auto-apply to a background mechanism without
  sacrificing reliability or adding system footprint.
---

# DESIGN: Background Updates

## Status

Proposed

## Context and Problem Statement

Every tsuku command (except a small skip-list) passes through `PersistentPreRun`
in `cmd/tsuku/main.go`, which calls two functions before the user's command runs:

1. `CheckAndSpawnUpdateCheck` — already non-blocking. It stat-checks a sentinel
   file and, if stale, spawns a detached `tsuku check-updates` subprocess via
   `cmd.Start()` without `Wait()`. This takes under 1 ms.

2. `MaybeAutoApply` — synchronous. It reads cached update entries and, if
   auto-apply is enabled and updates are pending, calls `runInstallWithTelemetry`
   for each one before the user's command runs. A user with three pending
   auto-updates waits through three complete install operations — including
   downloads — before `tsuku list` prints anything.

This is the source of the blocking users experience. It makes tsuku feel broken.

A secondary blocking path exists in `main.go init()`: when distributed registries
are configured, `NewDistributedRegistryProvider` calls `DiscoverManifest`
synchronously with `context.Background()` (no timeout) for each configured source,
adding unbounded HTTP roundtrips at binary startup.

The update check itself (`CheckAndSpawnUpdateCheck` + `trigger.go`) already
implements the right pattern — fire-and-forget subprocess with file-lock dedup
and sentinel freshness — and the notification system (file-backed, pull-per-command
via `$TSUKU_HOME/notices/`) already delivers async results without new IPC.
The design question is how to extend this to the apply step.

## Decision Drivers

- **Zero foreground blocking:** The user's command must start immediately.
  Any update-related work that can be deferred must be.
- **Lighter footprint first:** No persistent daemons, no OS schedulers (cron,
  systemd timers, launchd). The detached-subprocess pattern already in use is
  the starting point.
- **Use existing infrastructure:** The notice system and the `trigger.go`
  subprocess pattern are already proven. Minimize new primitives.
- **Backward-compatible schema changes:** The `Notice` struct is serialized to
  disk files on user machines. Any schema extension (e.g., a `Kind` field) must
  deserialize existing files without error.
- **Safe concurrent installs:** Auto-apply running in background and an explicit
  `tsuku install foo` running in foreground must not corrupt tool state. The
  existing `state.json.lock` flock is the gate; the design must account for
  it explicitly.
- **Platform scope:** Linux and macOS only (current GoReleaser targets).
  Windows must not be broken but is not a release target.

## Decisions Already Made

From exploration (Round 1):

- OS schedulers (cron, systemd timers, launchd) are eliminated. They require
  system footprint and lifecycle management that contradicts the project
  philosophy.
- Persistent daemon is eliminated for the same reason.
- The detached-subprocess pattern in `trigger.go` is the confirmed mechanism.
  No new background primitives needed.
- The notice system (file-backed, pull-per-command) is the correct delivery
  channel for background activity results. No new IPC needed.
- "Registry cache refresh" is not the primary blocking concern. Registry refresh
  has no automatic trigger; only `tsuku update-registry` (explicit) or inline
  recipe fetches on cache miss. The blocking is from `MaybeAutoApply`.
- Notices should appear after command output, not before.
