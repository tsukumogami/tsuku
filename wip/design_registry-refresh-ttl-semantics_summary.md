# Design Summary: registry-refresh-ttl-semantics

## Input Context (Phase 0)

**Source:** Freeform topic
**Problem:** `tsuku update-registry` applies a 24-hour TTL to skip recently-cached recipes, so recipe fixes merged upstream don't reach users who run the command explicitly. The TTL is correct for implicit cache access and background automation but wrong for explicit user invocations.
**Constraints:**
- TTL must be preserved for implicit fetches (install, info, search) and automated calls
- Auto-update background process (`check-updates`) does not call registry refresh — no change needed
- Minimal API change preferred

## Current Status

**Phase:** 1 - Decision Decomposition
**Last Updated:** 2026-04-16
