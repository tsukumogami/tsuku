# Issue 1586 Summary

## What Was Implemented

Added content-based plan hashing to replace the removed recipe-based hashing. The new `ComputePlanContentHash()` function computes a deterministic SHA256 hash of a plan's functional content, enabling cache validation that detects when plan content has changed.

## Changes Made

- `internal/executor/plan_cache.go`:
  - Added `ContentHash` field to `PlanCacheKey` struct
  - Added `CacheKeyWithHash()` convenience function
  - Updated `ValidateCachedPlan()` to compare content hashes when provided
  - Added `ComputePlanContentHash()` - main entry point for hashing
  - Added `planContentForHashing()` - creates normalized representation
  - Added normalized struct types (`planForHashing`, `stepForHashing`, `depForHashing`, etc.)
  - Added `sortedParams()` and `sortValue()` for deterministic map handling

- `internal/executor/plan_cache_test.go`:
  - Added comprehensive tests for `ComputePlanContentHash()` covering:
    - Deterministic output
    - Identical content produces identical hashes
    - GeneratedAt/RecipeSource changes don't affect hash
    - Different steps produce different hashes
    - Nested dependencies included in hash
    - Map ordering in params is deterministic
  - Added test for `CacheKeyWithHash()`
  - Added tests for `ValidateCachedPlan()` with content hash validation
  - Added tests for `sortedParams()` helper

## Key Decisions

- **Hash function placement**: Put hashing functions in `plan_cache.go` rather than `plan_generator.go` because caching is the concern, not generation.

- **Normalized struct approach**: Used separate structs with alphabetically-ordered fields rather than inline sorting to ensure deterministic JSON output regardless of Go's map iteration order.

- **Explicit map sorting**: Added `sortedParams()` helper even though Go's json.Marshal sorts map keys since Go 1.12, to be explicit about the determinism contract and handle nested structures.

- **Non-breaking ContentHash**: Made `ContentHash` optional in `PlanCacheKey` (empty string skips validation) to allow gradual rollout and backward compatibility.

## Trade-offs Accepted

- **Slight overhead**: Computing a hash requires serializing the normalized plan to JSON and hashing it. Acceptable because it only happens during cache operations, not during plan execution.

- **Duplicate struct definitions**: The normalized structs mirror the original plan types. This adds code but provides clear separation of concerns and guaranteed field ordering.

## Test Coverage

- New tests added: 11 (6 for `ComputePlanContentHash`, 1 for `CacheKeyWithHash`, 3 for `ValidateCachedPlan_ContentHash`, 4 for `SortedParams`)
- All new functions covered by tests
- Determinism verified with multiple test cases

## Known Limitations

- Cache flow in `install_deps.go` not yet updated to use content hashing (documented in issue as a future integration point)
- Content hash is optional; callers must explicitly request hash-based validation

## Future Improvements

- Integrate content hashing into the main cache flow in `cmd/tsuku/install_deps.go`
- Consider caching the computed hash on the plan struct to avoid recomputation
