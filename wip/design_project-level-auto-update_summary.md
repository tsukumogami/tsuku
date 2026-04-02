# Design Summary: project-level-auto-update

## Input Context (Phase 0)
**Source:** Issue #2188 (feat(update): project-level auto-update interaction with .tsuku.toml)
**Upstream:** PRD-auto-update.md (R17)
**Roadmap:** ROADMAP-auto-update.md (Feature 8)
**Problem:** Auto-update ignores .tsuku.toml. MaybeAutoApply has no project awareness. Exact pins in project config should suppress auto-update; prefix pins should allow it within boundaries.
**Constraints:** Precedence over global config, zero latency, CWD-based project detection, existing pin semantics

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-04-01
