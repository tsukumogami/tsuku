# Issue 1586 Implementation Plan

## Summary

Implement content-based plan hashing by adding `computePlanContentHash()` and `planContentForHashing()` functions to plan_cache.go, update `PlanCacheKey` to include `ContentHash`, and integrate content hash validation into the cache flow.

## Approach

The approach adds deterministic content hashing to installation plans by:
1. Creating a normalized struct representation that excludes non-functional fields (`generated_at`, `recipe_source`)
2. Using sorted maps for step params to ensure deterministic JSON output
3. Computing SHA256 of the normalized JSON
4. Updating cache validation to compare content hashes

This follows the design in DESIGN-plan-hash-removal.md and uses the same hashing patterns already established in the codebase (e.g., `internal/registry/cache.go`, `internal/actions/download_cache.go`).

### Alternatives Considered

- **Hash during plan generation in plan_generator.go**: Rejected because plan_cache.go is the natural home for cache-related hashing logic. The generator focuses on producing plans, not cache keys.

- **Store content hash in plan struct**: Rejected because the hash is a cache key concern, not plan content. Adding it to the plan would create circular dependency (hash includes itself).

- **Use encoding/gob instead of JSON**: Rejected because JSON is already used throughout the codebase for plan serialization and Go's json.Marshal sorts map keys by default since Go 1.12.

## Files to Modify

- `internal/executor/plan_cache.go` - Add `ContentHash` field to `PlanCacheKey`, add `computePlanContentHash()` and `planContentForHashing()` functions, update `ValidateCachedPlan()` to use content hash
- `internal/executor/plan_cache_test.go` - Add tests for content hashing functions and updated validation logic
- `cmd/tsuku/install_deps.go` - Update cache flow to compute content hash after plan generation and pass to cache validation

## Files to Create

None - all new functionality fits naturally in existing files.

## Implementation Steps

- [ ] 1. Add normalized structs for hashing in `plan_cache.go`
  - Create `planForHashing` struct mirroring `InstallationPlan` but without `GeneratedAt` and `RecipeSource`
  - Create `stepForHashing` struct mirroring `ResolvedStep`
  - Create `depForHashing` struct mirroring `DependencyPlan`
  - Use sorted key order by defining fields in alphabetical order

- [ ] 2. Implement `sortedParams()` helper in `plan_cache.go`
  - Convert `map[string]interface{}` to a slice of key-value pairs sorted by key
  - Handle nested maps recursively
  - Handle slices by recursively sorting any nested maps

- [ ] 3. Implement `planContentForHashing()` in `plan_cache.go`
  - Convert `InstallationPlan` to normalized `planForHashing` struct
  - Call `sortedParams()` for each step's Params map
  - Recursively handle Dependencies

- [ ] 4. Implement `computePlanContentHash()` in `plan_cache.go`
  - Call `planContentForHashing()` to get normalized representation
  - Marshal to JSON using `json.Marshal`
  - Compute SHA256 hash
  - Return hex-encoded string

- [ ] 5. Add `ContentHash` field to `PlanCacheKey` struct
  - Add `ContentHash string` field with `json:"content_hash,omitempty"` tag
  - Update comment to explain when ContentHash is used

- [ ] 6. Update `ValidateCachedPlan()` to compare content hashes
  - If `key.ContentHash` is non-empty, compute cached plan's content hash
  - Compare computed hash to `key.ContentHash`
  - Return error if hashes don't match (cache invalidation)

- [ ] 7. Update `getOrGeneratePlanWith()` in `install_deps.go` to compute content hash
  - After generating a fresh plan, compute its content hash
  - After loading a cached plan, compute its content hash for comparison
  - Pass content hash in cache key for validation

- [ ] 8. Add unit tests for content hashing
  - Test `computePlanContentHash()` produces deterministic output
  - Test two plans with identical content produce identical hashes
  - Test plans differing only in `GeneratedAt` produce identical hashes
  - Test plans differing in steps produce different hashes
  - Test nested dependencies are included in hash
  - Test step params with different map ordering produce same hash

- [ ] 9. Add test for `ValidateCachedPlan()` with content hash
  - Test cache validation passes when content hashes match
  - Test cache validation fails when content hashes differ

## Testing Strategy

- **Unit tests**: Test `computePlanContentHash()`, `planContentForHashing()`, `sortedParams()` with various inputs including:
  - Plans with empty steps
  - Plans with nested dependencies
  - Step params with nested maps and slices
  - Different generation times (should produce same hash)

- **Determinism test**: Create two identical plans with different `GeneratedAt` values and verify they produce the same hash

- **Integration**: Existing `getOrGeneratePlan` tests will exercise the cache flow with content hashing

- **Manual verification**:
  ```bash
  go test ./internal/executor/... -v -run TestComputePlanContentHash
  go test ./cmd/tsuku/... -v -run TestGetOrGeneratePlan
  go test ./...
  ```

## Risks and Mitigations

- **Non-deterministic map iteration**: Mitigated by using `sortedParams()` helper that explicitly sorts map keys before marshaling. Go's json.Marshal also sorts map keys by default since Go 1.12.

- **Nested data structures**: Mitigated by recursively handling maps and slices in `sortedParams()` and testing with complex step params.

- **Performance overhead**: Content hashing adds a small cost per plan generation/cache lookup. Mitigated by only computing hash when needed (not on every cache hit that already validates).

## Success Criteria

- [ ] `computePlanContentHash()` function exists and returns consistent SHA256 hex strings
- [ ] `planContentForHashing()` creates normalized representation excluding `generated_at` and `recipe_source`
- [ ] `PlanCacheKey` struct has `ContentHash` field
- [ ] `ValidateCachedPlan()` compares content hashes when `ContentHash` is set
- [ ] Two plans with identical content produce identical hashes regardless of generation order
- [ ] Cache flow in `install_deps.go` generates content hash after plan generation
- [ ] All existing tests pass
- [ ] New unit tests cover content hashing functionality

## Open Questions

None - the design document provides clear guidance and the codebase has established patterns for hashing.
