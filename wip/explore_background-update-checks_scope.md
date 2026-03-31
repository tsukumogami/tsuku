# Explore Scope: background-update-checks

## Visibility

Public

## Core Question

How should tsuku's background update check infrastructure work? This covers the layered trigger model (shell hook > shim > command), time-cached check results, detached background process lifecycle, cache file format, and configuration surface. The design must integrate with the existing shell hook system and version resolution infrastructure from Feature 1, while keeping the hot path (shell prompt, tool execution) under 5ms.

## Context

This is Feature 2 of the auto-update roadmap. Feature 1 (channel-aware version resolution) is done -- ResolveWithinBoundary, pin-level semantics, and cached version lists are in place. The existing shell hook system runs `tsuku hook-env` on every prompt for PATH activation. Shims are shell scripts that `exec tsuku run`. No background process infrastructure exists yet (except the unrelated LLM lifecycle manager). The telemetry system uses fire-and-forget goroutines with 2s timeout. Config lives in `internal/userconfig/userconfig.go` with no `[updates]` section.

PRD requirements: R4 (time-cached checks, 24h default, 1h-30d range), R5 (layered triggers), R19 (zero added latency, 10s timeout).

## In Scope

- Layered trigger architecture (shell hook, shim, tsuku command)
- Background check process lifecycle (spawn, dedup, timeout)
- Cache file format (update-check.json)
- [updates] config section design
- Integration with existing hook-env and tsuku run paths
- Staleness detection mechanism

## Out of Scope

- Auto-apply logic (Feature 3 -- reads check results but doesn't produce them)
- Notification display (Feature 5 -- consumes check results)
- Self-update check inclusion (Feature 4 -- independent track)
- Rollback mechanism (Feature 3)
- Out-of-channel notifications (Feature 6)

## Research Leads

1. **How do other CLI tools implement background update checks without blocking?**
   Homebrew, rustup, gh, proto, volta -- what patterns exist for detached checker processes and staleness detection? Understanding prior art helps avoid reinventing failure modes.

2. **What's the right cache file schema for update check results?**
   Per-tool entries with version info, check timestamps, pin boundaries? Single file vs per-tool files? What do downstream consumers (auto-apply, notifications) need from this file?

3. **How should the shell hook trigger avoid duplicate background spawns?**
   PID files, lock files, or just relying on mtime? What happens when a prompt fires while a check is already running? Need a dedup mechanism that doesn't add latency.

4. **What's the concrete hook-env modification needed for <5ms staleness detection?**
   The current implementation runs ComputeActivation on every prompt. Can the stat check be embedded there, or does it need a separate code path? What's the spawn mechanism for the detached process?

5. **How should the [updates] config section be structured?**
   Keys, types, defaults, validation. How do env var overrides interact? Precedence chain from PRD: CLI flag > env var > .tsuku.toml > config.toml > default.

6. **What's the shim trigger path and how do shim-based tools handle update checks?**
   Shims currently exec tsuku run -- the check would happen inside tsuku run. But tsuku run needs to be fast. How do other shim-based tools (asdf, mise) handle this?
