# Design Summary: curated-recipes

## Input Context (Phase 0)
**Source:** Freeform topic — expanded from explore/install-claude
**Problem:** tsuku has ~1,400 recipes but ~20-50 high-impact tools are missing, Linux-only, or poorly served. No system to identify, protect, or verify critical recipes.
**Constraints:** Zero infrastructure change preferred; backward-compatible with 184 existing handcrafted recipes; no provider chain or URL construction changes.

## Decisions Made

| ID | Question | Chosen | Status |
|----|----------|--------|--------|
| D1 | Curated recipe provenance signal | `curated = true` flag in `[metadata]` | confirmed |
| D2 | Periodic install testing mechanism | `ci.curated` array in `test-matrix.json` | confirmed |
| D3 | Discovery strategy for npm name-mismatch tools | Handcrafted recipe + companion discovery entry; NpmBuilder fix deferred | confirmed |

## Security Review (Phase 5)
**Outcome:** Option 2 — Document considerations
**Summary:** No design changes required. Key considerations: npm supply chain trust via org-scoped packages, checksum best practices for direct binary downloads, `issues: write` permission scoped to nightly job only.

## Current Status
**Phase:** 6 - Final Review
**Last Updated:** 2026-04-16
