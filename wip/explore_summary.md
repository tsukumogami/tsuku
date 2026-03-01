# Exploration Summary: Dependency Validation in Batch Pipeline

## Problem (Phase 1)
The Homebrew builder silently writes unresolvable dependencies into generated recipes because `create.go` never passes a `RegistryChecker` to `NewHomebrewBuilder()`. The batch pipeline then treats these as successes, masking the true blocker landscape in the dashboard.

## Decision Drivers (Phase 1)
- Must use existing infrastructure (RegistryChecker, FailureCategoryMissingDep, ExitDependencyFailed)
- Must work with satisfies lookups (e.g., `openssl@3` -> `openssl`)
- Must produce error messages compatible with `extractBlockedByFromOutput()` regex
- Must propagate correctly through the batch pipeline (blocked status, not failed)
- Minimal code changes -- infrastructure already exists, just needs connecting

## Research Findings (Phase 2)
- RegistryChecker interface exists but is never passed from CLI
- FailureCategoryMissingDep defined but never used
- ExitDependencyFailed (code 8) defined but never triggered
- Batch orchestrator already maps exit code 8 to missing_dep category
- extractBlockedByFromOutput() already parses "recipe X not found in registry"
- Dashboard correctly shows blocked packages and top blockers
- Requeue system already promotes blocked -> pending when deps resolve

## Options (Phase 3)
- Option 1A: Validate at builder level (before writing recipe)
- Option 1B: Validate at CLI level (after builder returns recipe)
- Option 2A: Fail on first missing dep
- Option 2B: Collect all missing deps then fail

## Decision (Phase 5)

**Problem:**
The Homebrew builder writes unresolvable dependency names into generated recipes without checking whether those dependencies exist in the tsuku registry. The batch pipeline classifies these as successes rather than blocked entries, hiding the true scope of missing dependencies from the dashboard. This means the batch priority queue can't properly order work by blocker impact, and tools appear installable when they actually can't resolve their dependency chain.

**Decision:**
Validate dependencies inside the Homebrew builder before writing `RuntimeDependencies` to the recipe. Pass a `RegistryChecker` from `create.go` at both call sites, backed by the recipe `Loader` with satisfies support. When any dependency is missing, return a `DeterministicFailedError` with `FailureCategoryMissingDep`, listing all missing names in the error message using the format `"recipe X not found in registry"`. The CLI exits with code 8 (`ExitDependencyFailed`), which the batch orchestrator already maps to the `missing_dep` category.

**Rationale:**
The infrastructure for this validation already exists but isn't wired up. The `RegistryChecker` interface, the `missing_dep` error category, exit code 8, the orchestrator's regex-based blocker extraction, and the dashboard's blocker reporting are all in place. The fix is a wiring change, not new architecture. Validating inside the builder (rather than at the CLI level) catches problems before the recipe is written, keeping the builder's output always valid. Collecting all missing deps before failing (rather than failing on the first one) gives the batch pipeline complete blocker information in a single pass.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-03-01
