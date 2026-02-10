# Issue 1586 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-plan-hash-removal.md`
- Sibling issues reviewed: #1584 (validation scripts), #1585 (remove RecipeHash)
- Prior patterns identified: Commit `5e925615` removed RecipeHash from plan structs

## Current State Analysis

The staleness signal indicated 4 files were modified since issue creation. Upon investigation:

1. **Commit `5e925615`** has landed on main, implementing #1585's work:
   - Removed `RecipeHash` field from `InstallationPlan` and `DependencyPlan` structs
   - Removed `RecipeHash` from `PlanCacheKey` in `plan_cache.go`
   - Removed `computeRecipeHash()` function from `plan_generator.go`
   - Bumped `PlanFormatVersion` from 3 to 4
   - Updated validation logic in `ValidateCachedPlan()` to remove recipe hash comparison

2. **GitHub Issue #1585 status**: Still shows as OPEN in GitHub (likely pending PR merge or issue closure)

3. **Current `PlanCacheKey` state** (from `plan_cache.go` line 12-17):
   ```go
   type PlanCacheKey struct {
       Tool     string `json:"tool"`
       Version  string `json:"version"`
       Platform string `json:"platform"`
       // Note: RecipeHash was removed in v4. Cache validation now uses content-based hashing.
   }
   ```

4. **Current `ValidateCachedPlan()` state**: Only checks format version and platform, with a comment noting content-based validation will be added.

## Gap Analysis

### Minor Gaps

1. **Cache key location**: The design shows `ContentHash` being added to `PlanCacheKey`, but the current implementation removed `RecipeHash` without adding `ContentHash` yet. This is expected - #1586 adds it.

2. **Code placement**: The issue spec says `computePlanContentHash()` should go in `plan_generator.go`, but the design doc also mentions `planContentForHashing()` helper in `plan_cache.go`. The issue acceptance criteria cover both locations, so this is clear.

3. **Cache flow change**: The design doc notes the cache flow changes from "compute recipe hash before plan generation" to "compute content hash after plan generation". The current `install_deps.go` already has the cache lookup before plan generation. Issue #1586 needs to update `ValidateCachedPlan()` to compare content hashes, which happens after plan generation as the design specifies.

### Moderate Gaps

None identified. The issue spec is detailed and aligns with the current codebase state.

### Major Gaps

None identified. The prerequisite work (#1585) has been completed as committed code, even though the GitHub issue is still open. All acceptance criteria in #1586 are implementable given the current state.

## Validation Checks

1. `computeRecipeHash` removed: PASS (grep returns no matches)
2. `RecipeHash` field gone from plan structs: PASS (only comment reference remains in plan_cache.go)
3. `PlanFormatVersion` is 4: PASS (line 19 of plan.go)
4. `computePlanContentHash` exists: NOT YET (this is what #1586 implements)

## Recommendation

**Proceed**

The codebase is in the expected state for implementing #1586. The prerequisite work from #1585 has been completed and merged. The issue spec is complete and accurate - all 9 acceptance criteria can be implemented against the current codebase without clarification.

## Implementation Notes

Based on review, the implementation should:
1. Add `computePlanContentHash()` to `plan_generator.go` or `plan_cache.go`
2. Add `planContentForHashing()` helper that creates normalized plan representation
3. Add `ContentHash` field to `PlanCacheKey` struct
4. Update `ValidateCachedPlan()` to compare content hashes
5. Ensure deterministic JSON marshaling (Go 1.12+ sorts map keys by default)
6. Update cache flow in `install_deps.go` if needed (may already work since content hash is computed from generated plan)
