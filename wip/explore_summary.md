# Exploration Summary: Recipe CI Batching

## Problem (Phase 1)
Recipe-triggered CI workflows spawn one job per changed recipe, causing PR #1770 to generate 153 per-recipe jobs. Each job independently checks out the repo and builds tsuku, wasting ~30-45s of setup per job. This doesn't scale to wholesale refactors touching hundreds of recipes.

## Decision Drivers (Phase 1)
- Must scale to PRs changing 100+ recipes without generating 100+ jobs
- Must preserve per-recipe failure visibility (which recipe broke?)
- Must work across platforms: plain Linux, Linux container families, macOS
- macOS aggregation pattern already exists and works well
- Minimize changes to existing detection/filtering logic
- Keep individual job duration under ~15 minutes

## Research Findings (Phase 2)
- macOS aggregation in test-changed-recipes.yml is the proven template
- Container-family batching in validate-golden-execution.yml works similarly
- recipe-validation-core.yml builds binaries once via artifacts (complementary pattern)
- validate-golden-execution.yml has 3 additional per-recipe jobs not covered by this design

## Options (Phase 3)
- Ceiling-division batching (chosen)
- Time-estimated batching (rejected: no timing data)
- Single-job aggregation (rejected: no parallelism)
- max-parallel throttling (rejected: doesn't fix per-job overhead)

## Decision (Phase 5)

**Problem:**
Recipe-triggered CI workflows spawn one GitHub Actions job per changed recipe. PR #1770 produced 153 per-recipe jobs, each paying ~30-45 seconds of cold-start overhead (checkout, Go setup, binary build) before testing a single recipe. This doesn't scale to bulk recipe changes like schema migrations or builder upgrades that touch 100+ recipes at once.

**Decision:**
Split per-recipe matrix jobs into fixed-size batches using ceiling division (default: 15 recipes per batch). Each batch runs on one runner that builds tsuku once and loops through its recipes with per-recipe TSUKU_HOME isolation and failure accumulation. The batch size is configurable and clamped to 1-50. Multi-platform workflows cross the batch dimension with the platform dimension.

**Rationale:**
Ceiling-division batching is the simplest strategy that bounds job count without requiring per-recipe timing data or changes to the detection logic. The inner-loop pattern with grouping and failure accumulation already runs reliably in the macOS aggregated path. Extending it to Linux per-recipe jobs converges on a single tested pattern rather than introducing something new.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-21
