# Exploration Summary: dev-environment-isolation

## Problem (Phase 1)
Developers working on tsuku need isolated environments that don't interfere with their regular tsuku installation, support parallel invocations without conflicts, and still allow state reuse across multiple runs.

## Decision Drivers (Phase 1)
- Isolation from the user's real `$TSUKU_HOME`
- Parallel-safe: multiple concurrent dev invocations must not corrupt state
- Stateful: a dev environment should persist tools, cache, and state across runs
- Simple: setting up a dev environment should require minimal ceremony
- Discoverable: easy to understand what environment you're operating in

## Research Findings (Phase 2)
- CI already uses per-test TSUKU_HOME with shared download cache (symlinked)
- Sandbox uses container-level isolation with read-only cache mount
- DefaultConfig() derives all paths from TSUKU_HOME; modifying just TSUKU_HOME gives full isolation
- Download cache is content-addressed (sha256 of URL), safe to share
- Cache security check rejects symlinks for writes but allows read-only

## Options (Phase 3)
- Option 1: `--env <name>` CLI flag (environments under $TSUKU_HOME/envs/)
- Option 2: `TSUKU_ENV` environment variable (same layout, env-var driven)
- Option 3: Standalone TSUKU_HOME with shared cache via config.toml

## Decision (Phase 5)
**Problem:** Developers working on tsuku lack a low-ceremony way to run against isolated environments without interfering with their real installation or each other.
**Decision:** Add `--env <name>` flag and `TSUKU_ENV` environment variable that create named environments under `$TSUKU_HOME/envs/<name>/` with automatic download cache sharing.
**Rationale:** The combined flag + env var pattern eliminates manual TSUKU_HOME juggling, provides discoverability through `tsuku env list`, and shares cache automatically without requiring per-environment config files.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-01-29
