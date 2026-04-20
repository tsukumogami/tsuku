# Design Summary: background-updates

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** `MaybeAutoApply` runs full tool installs synchronously in
`PersistentPreRun` before every command, blocking even fast read-only operations.
The update check is already non-blocking; the apply step is not. A secondary
blocking path exists in distributed registry initialization at startup (no timeout).
**Constraints:** No daemons, no OS schedulers; extend the existing detached-subprocess
pattern in `trigger.go`; notice schema changes must be backward-compatible; Linux
and macOS only.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-04-20
