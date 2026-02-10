# Exploration Summary: Plan Recipe Hash Removal

## Problem (Phase 1)

Plans include a `recipe_hash` field (SHA256 of recipe TOML) that creates artificial coupling between recipe source and plan identity. Different recipes that produce functionally identical plans (same URLs, checksums, steps) generate incompatible plans due to different hashes. The hash provides no security benefit beyond cache invalidation, which could be achieved through simpler means.

## Decision Drivers (Phase 1)

- Plans should be portable across recipe sources (homebrew-generated, hand-written, etc.)
- Download checksums already protect against tampering at execution time
- Golden file maintenance should not require changes when recipe formatting changes
- Cache invalidation must still work reliably
- Migration must handle ~600 local golden files and R2-stored files

## Research Findings (Phase 2)

### Current Usage

1. **Cache invalidation only**: `recipe_hash` is compared in `ValidateCachedPlan()` to detect recipe changes
2. **Not verified at execution**: When running a plan, recipe hash is never checked
3. **Blocks plan portability**: Different recipe sources produce different hashes for equivalent plans

### Golden File System

- Local: `testdata/golden/plans/` with ~600 files
- R2: `tsuku-golden-registry` bucket with directory structure
- Validation strips `generated_at` and `recipe_source` but compares `recipe_hash`
- Changing recipe hash requires regenerating all affected golden files

### Security Assessment

The recipe hash provides zero security value:
- Download checksums protect against tampering (verified at execution)
- Plans are treated as trusted input regardless of recipe hash
- No signature or cryptographic verification exists

## Options (Phase 3)

1. **Remove recipe_hash entirely**: Simplest approach, but breaks cache invalidation
2. **Move hash to metadata only**: Keep for debugging but don't include in plan content
3. **Replace with content hash**: Hash the plan content (minus timestamps) for cache invalidation

## Decision (Phase 5)

**Problem:**
Plans include a `recipe_hash` field that couples plan identity to recipe source. This prevents plan portability (different recipes can't produce interchangeable plans) and complicates golden file maintenance. The hash provides no security benefit since download checksums protect against tampering.

**Decision:**
Remove `recipe_hash` from plan content and replace cache invalidation with a content-based approach. The cache key will use a hash of the plan's immutable content (steps, checksums, dependencies) rather than the recipe source. This decouples plans from recipes while maintaining reliable cache behavior.

**Rationale:**
Download checksums provide the actual security guarantee. Recipe hashes only served cache invalidation, which can be achieved by hashing plan content instead. This enables plan portability and simplifies golden file maintenance since plans change only when their functional content changes.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-09
