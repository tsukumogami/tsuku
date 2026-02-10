# Issue 1587 Baseline

## Environment
- Date: 2026-02-10
- Branch: docs/plan-hash-removal (continuing from issues #1585 and #1586)
- Base commit: 8be22c2626375becd36e3c982a96bcafcd9b98fe

## Test Results
- `internal/executor`: PASS
- `internal/install`: PASS

## Current State Analysis

### RecipeHash References in Tests
None found - already cleaned up in prior issues.

### Content Hash Tests (from #1586)
Already implemented:
- `TestComputePlanContentHash` - determinism, uniqueness, GeneratedAt exclusion
- `TestCacheKeyWithHash` - cache key generation
- `TestValidateCachedPlan_ContentHash` - cache validation with content hash

### Missing: Portability Test
The acceptance criteria requires a portability test that verifies:
- Two structurally different recipes producing identical plans
- Must have identical content hashes

This is the primary work item for this issue.

## Acceptance Criteria Status

- [x] Update `internal/executor/plan_test.go` to remove `RecipeHash` field assertions - N/A (no RecipeHash)
- [x] Update `internal/executor/plan_cache_test.go` to test content-based cache validation - Done in #1586
- [x] Update `internal/executor/plan_generator_test.go` to remove recipe hash computation tests - N/A (no RecipeHash)
- [x] Update `internal/executor/plan_conversion_test.go` to remove `RecipeHash` field mapping - N/A (no RecipeHash)
- [x] Update `internal/install/state_test.go` to remove `RecipeHash` field expectations - N/A (no RecipeHash)
- [x] Add test for `computePlanContentHash()` determinism - Done in #1586
- [x] Add test for `computePlanContentHash()` uniqueness - Done in #1586
- [ ] **Add portability test** - NOT YET DONE

## Notes
Most acceptance criteria are already satisfied from issues #1585 and #1586.
The remaining work is to add the portability test demonstrating that different recipes
producing identical plans have the same content hash.
