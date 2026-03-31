---
status: Proposed
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2183
  repo: tsukumogami/tsuku
problem: |
  Tsuku has no mechanism to detect when newer tool versions are available without
  the user explicitly running tsuku outdated. The shell hook runs on every prompt
  but only handles PATH activation. There's no background process infrastructure,
  no update cache, and no configuration surface for update behavior. Without this
  plumbing, auto-apply (Feature 3) and notifications (Feature 5) have nothing to
  consume.
---

# DESIGN: Background update check infrastructure

## Status

Proposed

## Context and Problem Statement

Feature 1 established channel-aware version resolution: `ResolveWithinBoundary` respects pin boundaries, `CachedVersionLister` derives latest from cached lists, and `PinLevelFromRequested` computes pin levels at runtime. But there's no way to trigger these checks automatically. Users must run `tsuku outdated` manually, and no cached results exist for downstream features to consume.

The auto-update system needs plumbing that runs checks in the background without adding latency to shell prompts or tool execution. Three trigger entry points (shell hook, shim invocation, direct command) must coordinate to avoid duplicate checks while ensuring timely detection. The background process must query all installed tools' version providers, write structured results to a cache, and respect a configurable check interval.

This design covers PRD requirements R4 (time-cached checks), R5 (layered triggers), and the R19 cross-cutting constraint (zero added latency). It produces the cache that Feature 3 (auto-apply), Feature 5 (notifications), and Feature 6 (outdated polish) consume.

## Decision Drivers

- **Prompt latency budget**: The shell hook fires on every prompt. Any staleness check must complete in <5ms. Network I/O is forbidden on this path.
- **Existing patterns**: The codebase has advisory file locking (`internal/install/filelock.go`), per-provider version caching (`internal/version/cache.go`), and fire-and-forget telemetry (`internal/telemetry/client.go`). The design should compose these, not invent new infrastructure.
- **Downstream consumers**: Feature 3 reads check results to decide what to install. Feature 5 reads them to decide what to display. Feature 6 uses both within-pin and overall versions for dual-column outdated. The cache schema must serve all three.
- **Concurrent access**: Shell hooks, shim invocations, and direct commands can all trigger checks simultaneously. The system must handle this without lock contention or lost updates.
- **Configuration surface**: Users need to control check frequency, enable/disable, and opt out in CI. The config must follow existing patterns in `internal/userconfig/`.

## Decisions Already Made

These choices were settled during exploration and should be treated as constraints:

- **Per-tool cache files** at `$TSUKU_HOME/cache/updates/<toolname>.json` over a single aggregate file. Avoids lock contention on concurrent writes, matches the version cache precedent.
- **Advisory flock for spawn dedup** via existing `filelock.go`. Kernel-managed, auto-cleanup on crash, <1ms non-blocking check. Matches LLM lifecycle pattern.
- **Separate detached process** over goroutine. `hook-env` exits immediately after printing shell code and cannot hold goroutines. `exec.Command().Start()` spawns a process that survives the parent.
- **New hidden `tsuku check-updates` subcommand** as the background process entry point. Dedicated, not piggybacked on existing commands.
- **Shared `CheckUpdateStaleness` function** in `internal/updates/` called by all three trigger layers.
- **Notification throttle state in `$TSUKU_HOME/notices/`**, not in the check cache. Keeps the cache focused on "what's available."
- **Config follows LLMConfig pattern**: pointer types for optional values, getter methods checking env vars first.
