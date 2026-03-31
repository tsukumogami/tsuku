# Design Summary: auto-apply-rollback

## Input Context (Phase 0)
**Source:** Issue #2184 (needs-design)
**Problem:** Feature 2 produces update check results but nothing acts on them. Auto-apply must read cached results and install updates during tsuku commands, with auto-rollback on failure and manual rollback for runtime breakage.
**Constraints:** Reuse existing install flow, multi-version directories for cheap rollback, no concurrent state mutation, apply only during tsuku commands (not hooks/shims), basic notices for failures.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-31
