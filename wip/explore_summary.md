# Exploration Summary: Pipeline Blocker Tracking

## Problem (Phase 1)

The pipeline dashboard's "Top Blockers" panel is nearly empty despite hundreds of failed recipes, because failure records don't consistently populate the `blocked_by` field. The root cause spans three layers: the generate phase doesn't extract dependency information at all, the CLI exit code classification misses some dependency-related failures, and the dashboard has no fallback extraction from error messages.

## Decision Drivers (Phase 1)

- Recording accuracy: dependency failures must be classified correctly at the source
- Data completeness: existing failure records need remediation
- Dashboard utility: blocker display should show transitive impact, not just direct counts
- Backward compatibility: changes must not break existing queue processing or CI workflows
- Minimal disruption: prefer focused changes over pipeline rewrites

## Research Findings (Phase 2)

### Root Causes Identified

**1. Generate phase never extracts dependency info**: `orchestrator.generate()` uses `categoryFromExitCode()` directly without parsing JSON output. No `blocked_by` is ever set for generate-phase failures. Since `tsuku create` can fail when dependency recipes don't exist (e.g., "recipe bdw-gc not found in registry"), these get classified as `validation_failed` with no dependency tracking.

**2. CLI exit code classification has precedence bug**: In `classifyInstallError()`, `errors.As(err, &regErr)` fires before the "failed to install dependency" string check. When a dependency recipe is not found, the wrapped error contains a `RegistryError{NotFound}`, so it returns exit code 3 (`ExitRecipeNotFound`) instead of 8 (`ExitDependencyFailed`). Exit code 3 is not in the orchestrator's `categoryFromExitCode()` switch, falling to the default `"validation_failed"`.

**3. Transitive blocker computation uses mismatched keys**: `computeTransitiveBlockers()` looks up blocked packages (format: "homebrew:ffmpeg") in the blockers map, but blockers map keys are dependency names (format: "dav1d"). The key format mismatch means transitivity never triggers in practice.

**4. No remediation for existing data**: Hundreds of failure records have `validation_failed` category and no `blocked_by` despite containing "recipe X not found in registry" in their message text. No script exists to retroactively fix these.

### Upstream Design Constraints
- Dashboard consumes a single `dashboard.json` file (currently ~800KB)
- Failure records capped at 200 in dashboard.json, full data in JSONL files
- Two coexisting JSONL formats (legacy batch, per-recipe)
- Subcategory extraction already happens at dashboard generation time via message parsing

## Options (Phase 3)

1. **Where to fix classification**: Fix at both CLI and orchestrator levels (chosen) vs dashboard-only parsing vs adding --json to tsuku create
2. **How to remediate data**: One-time script patching JSONL + queue (chosen) vs wait for natural turnover vs dashboard-side extraction
3. **How to compute/display transitive blockers**: Normalized keys + two-count Blocker struct (chosen) vs full dependency graph from recipes vs flat non-transitive model

## Decision (Phase 5)

**Problem:**
The pipeline dashboard's "Top Blockers" panel is nearly empty despite hundreds of dependency-related failures, because failure records don't populate the `blocked_by` field. The generate phase skips dependency extraction entirely, the CLI misclassifies dependency errors via an exit code precedence bug, and the transitive blocker computation has a key format mismatch that prevents it from working. Operators can't use the dashboard to prioritize which dependencies to unblock.

**Decision:**
Fix at the recording level across CLI and orchestrator: reorder `classifyInstallError()` so dependency string check precedes the `errors.As` type check, add exit code 3 to the orchestrator's category mapping, and extract `blocked_by` from generate-phase output using the existing regex. Remediate existing data with a one-time script that patches failure records and queue entries. Fix the dashboard's transitive blocker computation by normalizing keys and adding `direct_count`/`total_count` fields to the Blocker struct.

**Rationale:**
Recording-level fixes ensure all data consumers see consistent, correct data without duplicating parsing logic. The CLI reorder is a minimal change (swapping two conditions) that eliminates the most common misclassification. Remediation is necessary because the pipeline processes packages infrequently -- natural data turnover would take weeks. Transitive counting directly serves the operator question "what should I unblock next?" by surfacing dependencies with the deepest downstream impact.

## Current Status
**Phase:** 5 - Decision (awaiting review feedback)
**Last Updated:** 2026-02-18
