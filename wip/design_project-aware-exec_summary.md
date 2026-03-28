# Design Summary: project-aware-exec

## Input Context (Phase 0)
**Source:** Issue #2168, Block 6 of shell integration building blocks
**Problem:** Track A (auto-install) and Track B (project config) don't converge at command invocation time in non-interactive contexts. CI, scripts, and hook-free users can't get project-declared tool versions installed on first use.
**Constraints:** Must work without shell hooks, <50ms for cached tools, leverage existing ProjectVersionResolver interface, clear security model for shims.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-28
