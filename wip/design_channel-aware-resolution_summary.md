# Design Summary: channel-aware-resolution

## Input Context (Phase 0)
**Source:** Issue #2181 (feat(update): channel-aware version resolution)
**Problem:** `tsuku update` ignores the Requested field, and `tsuku outdated` only checks GitHub providers. Need to establish pin-level semantics and fix both commands.
**Constraints:** Backward compatible with state.json, provider-agnostic, no new syntax, foundation for auto-update.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-30
**Mode:** --auto
