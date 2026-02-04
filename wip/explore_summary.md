# Exploration Summary: Batch PR CI Validation

## Problem (Phase 1)

The batch-generate workflow merges PRs before all CI checks complete or with failing checks. GitHub auto-merge only waits for *required* checks, but making recipe-specific checks required would block unrelated PRs that don't trigger those checks.

## Decision Drivers (Phase 1)

- Batch PRs must not merge with failing checks
- Non-batch PRs must not be blocked by recipe-specific required checks
- Golden file validation is expected to fail for new recipes (files don't exist in R2 yet)
- Solution must work within GitHub's branch protection model
- Minimize CI runtime for batch workflow (currently ~30 min for full validation)

## Research Findings (Phase 2)

**GitHub Limitations:**
- No native "required if run" feature - checks are either always required or never required
- Workflow-level path filtering causes checks to stay "Pending" forever (blocks merging if required)
- Job-level conditionals report "Success" when skipped (workaround for path filtering)

**Rollup Job Pattern:**
- Create a job that runs `always()` and checks all dependency results
- Make only this rollup job required
- Works within a single workflow, but not across multiple workflows

**Current Architecture Issues:**
1. 41 separate workflow files - can't use single rollup job across all
2. Golden file validation expects files in R2, but new recipes have no golden files yet
3. Error message acknowledges this ("post-merge workflow will regenerate") but still fails
4. Batch workflow uses auto-merge, which only waits for required checks

**Checks That Failed on Batch PRs:**
- `Validate: swiftformat`, `Validate: opentofu`, `Validate: terragrunt` (Validate Golden Files workflow)
- Root cause: Golden files don't exist in R2 for new recipes - chicken-and-egg problem

## Options (Phase 3)

**Decision 1: How should batch workflow wait for CI?**
- A: Use `gh pr checks --watch` - wait for ALL checks from all workflows
- B: Use auto-merge with rollup job - consolidate checks into single required check
- C: Explicit check list - batch workflow waits for specific named checks

**Decision 2: How should golden file validation handle new recipes?**
- A: Skip validation when no golden files exist - distinguish new vs modified recipes
- B: Pre-generate golden files - batch workflow generates before creating PR
- C: Advisory mode - validation runs but doesn't block merge
- D: Defer validation - only validate at merge time or nightly

## Current Status
**Phase:** 3 - Options
**Last Updated:** 2026-02-04
