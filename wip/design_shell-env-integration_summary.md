# Design Summary: shell-env-integration

## Input Context (Phase 0)

**Source:** /explore handoff
**Problem:** `~/.tsuku/env` only sets PATH; it doesn't source `.init-cache.<shell>`, so
tools installed with `install_shell_init` (e.g., niwa) have their shell functions built
correctly but never loaded in new terminals. Universal gap — every user who installed via
the official script is affected.
**Constraints:**
- No subprocess overhead (rules out switching to `eval "$(tsuku shellenv)"`)
- Static env file with shell detection (`$BASH_VERSION`/`$ZSH_VERSION`)
- Migration via existing `EnsureEnvFile()` idempotency mechanism
- Must preserve `TSUKU_NO_TELEMETRY` and any other user customizations
- `tsuku doctor --rebuild-cache` referenced in error messages but not implemented

## Current Status

**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-04-03
