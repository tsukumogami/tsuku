# Exploration Summary: Embedded Recipe List Validation

## Problem (Phase 1)

The recipe registry separation design requires a validated embedded recipe list, but there's no runtime enforcement that action dependencies can actually be resolved from embedded recipes.

## Decision Drivers (Phase 1)

- **Ground truth**: Validation must use actual loader behavior, not a parallel implementation
- **Incremental migration**: Must support known gaps during the migration period
- **Actionable failures**: CI failures must clearly indicate what's missing
- **Simplicity**: Prefer flag in existing code over new tools
- **Maintainability**: Documentation should be manually curated, not generated

## Key Research Findings

### Dependencies() Infrastructure (Issue #644 - CLOSED)
- `ActionDeps` struct defines InstallTime, Runtime, EvalTime, plus platform-specific variants
- `aggregatePrimitiveDeps()` in resolver.go automatically inherits deps from primitive actions
- 22 action files implement Dependencies()
- Transitive resolution with cycle detection exists in ResolveTransitive()

### Existing Loader Infrastructure
- Loader has priority chain: cache → local → embedded → registry
- Can be modified to restrict to embedded-only for action dependencies
- No need for separate static analysis tool

## Options (Phase 3)

1. **Validation Approach**: Static analysis tool (1A) vs Runtime flag (1B)
2. **CI Enforcement**: Full validation (2A) vs Exclusions (2B)
3. **Documentation**: Generated (3A) vs Manual (3B)

## Decision (Phase 5)

**Problem:** The recipe registry separation design requires a validated embedded recipe list, but there's no runtime enforcement that action dependencies can actually be resolved from embedded recipes.

**Decision:** Add a `--require-embedded` flag to the loader that fails if action dependencies can't be resolved from the embedded registry. Use CI with this flag to iteratively discover and validate the embedded recipe list.

**Rationale:** Runtime validation is the ground truth - it uses the actual loader to verify embedded recipes work. This enables incremental migration with an exclusions file to track known gaps, rather than building a separate static analysis tool.

## Current Status
**Phase:** 8 - Final Review (revised per user feedback)
**Last Updated:** 2026-01-18
