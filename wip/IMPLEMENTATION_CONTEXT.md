---
summary:
  constraints:
    - Must hash plan content deterministically (Go map iteration order is not guaranteed)
    - Hash must exclude non-functional fields (generated_at, recipe_source)
    - Hash must include all functional fields (format_version, tool, version, platform, steps, dependencies)
    - PlanFormatVersion already bumped to 4 in issue #1585
  integration_points:
    - plan_cache.go (PlanCacheKey struct, ValidateCachedPlan function)
    - plan_generator.go (add computePlanContentHash, remove computeRecipeHash)
    - install_deps.go (update cache flow to generate content hash after plan generation)
  risks:
    - Non-deterministic hashing if map iteration order leaks into JSON
    - Cache lookup timing change (now happens after plan generation, not before)
  approach_notes: |
    1. Add planContentForHashing() helper that creates a normalized struct with fixed field order
    2. Add computePlanContentHash() that marshals the normalized struct and computes SHA256
    3. Update PlanCacheKey to use ContentHash instead of RecipeHash
    4. Update ValidateCachedPlan() to compare content hashes
    5. Update cache flow in install_deps.go to compute content hash after plan generation

    Key insight: computeRecipeHash was already removed in issue #1585, so this issue focuses on
    adding the new content-based hashing functions and updating cache validation logic.
---

# Implementation Context: Issue #1586

**Source**: docs/designs/DESIGN-plan-hash-removal.md (Phase 2, items 4-9)

## Design Excerpt

The cache key changes from:
```go
type PlanCacheKey struct {
    Tool       string
    Version    string
    Platform   string
    RecipeHash string  // REMOVED
}
```

To:
```go
type PlanCacheKey struct {
    Tool        string
    Version     string
    Platform    string
    ContentHash string  // NEW: hash of plan content
}
```

### Implementation Pattern

```go
func computePlanContentHash(plan *InstallationPlan) string {
    // Create normalized representation excluding:
    // - generated_at (non-deterministic)
    // - recipe_source (not functional)
    normalized := planContentForHashing(plan)
    data, _ := json.Marshal(normalized)
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

### Hash Normalization

Go maps have non-deterministic iteration order. The `planContentForHashing()` function must:
1. Use a struct with fixed field order for the normalized representation
2. Convert step Params maps to sorted representation

### Cache Key Timing

The flow changes to:
1. Resolve version
2. Check cache with `(tool, version, platform)` key (without content hash)
3. If cache hit: compute content hash of cached plan, compare to freshly generated plan
4. If cache miss or mismatch: generate fresh plan

## Key Files to Modify

1. **internal/executor/plan_cache.go**: Add ContentHash to PlanCacheKey, update ValidateCachedPlan
2. **internal/executor/plan_generator.go**: Add computePlanContentHash and planContentForHashing
3. **cmd/tsuku/install_deps.go**: Update cache flow to use content hash
