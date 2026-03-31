# Design Summary: background-update-checks

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** Tsuku has no mechanism to detect when newer tool versions are available without manual intervention. No background process infrastructure, no update cache, no configuration surface for update behavior.
**Constraints:** <5ms prompt latency for staleness check, 10s absolute timeout for background process (R19), must compose existing patterns (filelock, version cache, telemetry), per-tool cache files at $TSUKU_HOME/cache/updates/<toolname>.json, advisory flock for dedup, detached process model.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-03-31
