# Design Summary: auto-install

## Input Context (Phase 0)
**Source:** Issue #1679 — docs: design auto-install flow
**Problem:** Users must manually install tools before using them. The `tsuku run <command>` flow should detect if a tool is missing and install it on demand, with configurable consent modes.
**Constraints:**
- Builds on Binary Index (#1677) — `lookupBinaryCommand` is the lookup interface
- Prepares integration point for Project Configuration (#1680) for version pinning
- `auto` mode is opt-in with explicit warning (security requirement)
- Must preserve the executed command's exit code
- Must handle non-TTY contexts gracefully (CI/scripts)

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-25
