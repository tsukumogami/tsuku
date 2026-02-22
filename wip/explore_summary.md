# Exploration Summary: Structured Error Subcategories

## Problem (Phase 1)
Pipeline failure subcategories are extracted from error message text via substring matching, which produces false positives when unrelated text contains trigger words.

## Decision Drivers (Phase 1)
- Subcategory accuracy (current regex approach has known false positives)
- Backward compatibility with existing JSONL data
- Minimal cross-component coupling
- Incremental adoption (don't require all-or-nothing migration)

## Research Findings (Phase 2)
- CLI already emits structured JSON with category, exit_code, and message via --json flag
- Orchestrator already parses this JSON via parseInstallJSON()
- Subcategory is the only classification that relies on heuristic text parsing
- The existing design doc (DESIGN-dashboard-observability) explicitly notes this should be replaced with structured output

## Options (Phase 3)
- Option A: Add subcategory to CLI --json output, propagate through orchestrator to JSONL
- Option B: Add subcategory at orchestrator level only (classify after CLI exits)
- Option C: Keep heuristic parsing but make it more precise

## Decision (Phase 5)

**Problem:**
Pipeline failure subcategories are derived by parsing error message text with substring matching in the dashboard generator. This is fragile: the word "verify" in a suggestion like "Verify the recipe name is correct" triggers `verify_pattern_mismatch` for `recipe_not_found` errors. The existing design doc acknowledges this should be replaced with structured output from the CLI.

**Decision:**
Add a `subcategory` field to the CLI's `--json` error output, determined alongside the existing category in `classifyInstallError()`. The orchestrator passes it through to `FailureRecord`, which writes it to JSONL. The dashboard prefers the structured field when present and falls back to heuristic parsing for older records that lack it.

**Rationale:**
The classification logic already exists at the CLI level (exit codes map to categories), and extending it to subcategories keeps the source of truth where the error context is richest. Adding the field at the orchestrator level would mean reimplementing error classification outside the CLI. Keeping heuristic parsing indefinitely accepts known false positives as permanent. The fallback-with-preference approach handles backward compatibility without a migration.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-22
