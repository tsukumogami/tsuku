# Design Summary: update-polish

## Input Context (Phase 0)
**Source:** Freeform topic (issue #2186)
**Problem:** Three UX gaps in the auto-update system: single-column outdated display, no out-of-channel notifications, and no batch update command.
**Constraints:** Must reuse existing cache entries, notification system, and config patterns. Out-of-channel throttle needs persistent per-tool state with injectable clock.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-04-01
