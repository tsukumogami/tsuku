# Design Summary: project-configuration

## Input Context (Phase 0)
**Source:** Freeform topic (issue #1680, Block 4 of shell integration building blocks)
**Problem:** Tsuku has no per-directory tool requirements. Projects can't declare which tools and versions they need, forcing manual discovery and installation by each developer.
**Constraints:** Must use TOML (existing convention), parse within 50ms, produce stable ProjectConfig interface for #1681 and #2168, support monorepos via directory traversal.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-27
