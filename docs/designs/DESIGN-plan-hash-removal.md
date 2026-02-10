---
status: Planned
problem: |
  Plans include a `recipe_hash` field (SHA256 of recipe TOML content) that creates
  artificial coupling between recipe source and plan identity. Different recipes
  that produce functionally identical plans (same URLs, checksums, steps) generate
  incompatible plans due to different hashes. This blocks plan portability and
  complicates golden file maintenance. The hash provides no security benefit since
  download checksums protect against tampering at execution time.
decision: |
  Remove `recipe_hash` from plan content entirely. Replace cache invalidation
  (currently based on recipe hash) with a content-based approach that hashes the
  plan's immutable fields (tool, version, platform, steps, dependencies). This
  decouples plans from their recipe source while maintaining reliable cache behavior.
  Migrate all golden files (local and R2) by regenerating without the hash field.
rationale: |
  Download checksums already provide the security guarantee - they're verified at
  execution time and detect any tampering with downloaded assets. Recipe hashes only
  served cache invalidation, which can be achieved by hashing plan content instead.
  This enables plan portability (different recipes can produce interchangeable plans)
  and simplifies golden file maintenance since plans change only when their functional
  content changes, not when recipe formatting or metadata changes.
---

# DESIGN: Plan Recipe Hash Removal

## Status

**Status:** Planned

## Context and Problem Statement

Installation plans currently include a `recipe_hash` field containing the SHA256 hash of the recipe TOML content. This field was added for cache invalidation: if a recipe changes, the cached plan is invalidated and regenerated.

However, this design creates several problems:

1. **Blocks plan portability**: A plan generated from a homebrew-sourced recipe and a hand-written recipe will have different hashes even if they produce identical installation steps, URLs, and checksums. This means plans are tied to their recipe source rather than their functional content.

2. **Complicates golden file maintenance**: Any change to a recipe (even whitespace or comment changes after TOML parsing) changes the hash, requiring all affected golden files to be regenerated.

3. **Provides no security benefit**: The recipe hash is never verified during plan execution. Download checksums provide the actual security guarantee by verifying downloaded assets match expected values.

### Scope

**In scope:**
- Removing `recipe_hash` from `InstallationPlan` and `DependencyPlan` structs
- Replacing cache key generation with content-based hashing
- Migrating local golden files (~600 files in `testdata/golden/plans/`)
- Migrating R2-stored golden files
- Updating validation scripts and CI workflows

**Out of scope:**
- Plan signing or cryptographic verification (future work)
- Changes to download checksum verification (already working correctly)
- Changes to version resolution or recipe parsing

## Decision Drivers

- **Plan portability**: Different recipes producing functionally identical plans should be interchangeable
- **Simplified maintenance**: Golden files should only change when functional content changes
- **Cache reliability**: Cache invalidation must still work correctly
- **Security preservation**: Must not weaken existing security guarantees
- **Migration feasibility**: Must be able to migrate existing golden files

## Considered Options

### Decision 1: How to handle cache invalidation without recipe hash

Cache invalidation currently works by comparing the recipe hash in the cache key with the recipe hash in the cached plan. Without recipe hash, we need an alternative mechanism.

#### Chosen: Content-based plan hashing

Hash the plan's immutable content (excluding timestamps and source info) to create a cache key. The hash includes:
- `format_version`
- `tool`, `version`
- `platform` (os, arch, family)
- `steps` (action, parameters, checksums)
- `dependencies` (recursively)

This approach means cache invalidation happens when the plan's functional content changes, not when the recipe source changes. Two different recipes producing identical plans will share the same cache.

**Implementation:**
```go
func computePlanContentHash(plan *InstallationPlan) string {
    // Create normalized representation excluding:
    // - generated_at (non-deterministic)
    // - recipe_source (not functional)
    // - recipe_hash (being removed)
    normalized := planContentForHashing(plan)
    data, _ := json.Marshal(normalized)
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

#### Alternatives Considered

**Keep recipe hash for cache only (not in plan content)**: Compute recipe hash for cache lookup but don't store it in the plan. Rejected because it still couples cache behavior to recipe source rather than plan content. A recipe formatting change would still invalidate caches unnecessarily.

**Use file modification time**: Check if recipe file is newer than cached plan. Rejected because it doesn't work across machines, CI environments, or with downloaded recipes.

**Remove caching entirely**: Always regenerate plans. Rejected because plan generation involves network calls (version resolution, checksum computation) that should be avoided when possible.

### Decision 2: How to handle state.json migration

The `$TSUKU_HOME/state.json` file stores installation state including `recipe_hash` for each installed tool.

#### Chosen: Remove field, no migration needed

Simply remove the `RecipeHash` field from the `Plan` struct in `state.go`. The JSON decoder will ignore the field in existing state files, and new installations won't include it.

This works because:
- State is local to each machine
- Cache validation uses the new content-based approach
- Old recipe_hash values are meaningless going forward

#### Alternatives Considered

**Write migration script**: Explicitly remove recipe_hash from existing state files. Rejected as unnecessary - Go's JSON decoder handles missing/extra fields gracefully.

**Bump state version**: Increment schema version and migrate on read. Rejected as over-engineering for a field removal.

## Decision Outcome

**Chosen: Content-based hashing + field removal**

### Summary

Remove `recipe_hash` from all plan structures and replace cache invalidation with content-based plan hashing.

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

Cache validation logic in `ValidateCachedPlan()` changes from comparing recipe hashes to comparing content hashes. The cached plan's content is hashed and compared against the expected content hash in the cache key.

Plans become truly portable: any recipe that produces the same functional plan will work with the same cached artifacts.

### Rationale

This combination works because:
1. Content hashing captures what actually matters for caching (the functional plan)
2. Download checksums already protect against tampering (unchanged)
3. Plans become self-contained artifacts not tied to their recipe source
4. Migration is straightforward (regenerate golden files without the field)

## Solution Architecture

### Overview

The change affects four areas:
1. Plan structures (remove `recipe_hash` field)
2. Cache key generation (use content hash instead)
3. Golden file format (field no longer present)
4. Migration tooling (regenerate all golden files)

### Components

```
Before:
  Recipe TOML → [hash] → recipe_hash → Plan JSON
                           ↓
                        Cache Key

After:
  Recipe TOML → Plan JSON → [hash] → content_hash
                              ↓
                           Cache Key
```

### Key Changes

**plan.go:**
- Remove `RecipeHash` field from `InstallationPlan`
- Remove `RecipeHash` field from `DependencyPlan`

**plan_generator.go:**
- Remove `computeRecipeHash()` function
- Add `computePlanContentHash()` function with deterministic JSON marshaling

**plan_cache.go:**
- Change `PlanCacheKey.RecipeHash` to `PlanCacheKey.ContentHash`
- Update `ValidateCachedPlan()` to compare content hashes
- Add `planContentForHashing()` helper

**plan_conversion.go:**
- Update `ToStoragePlan()` and `FromStoragePlan()` to remove `RecipeHash` copying

**state.go:**
- Remove `RecipeHash` field from storage `Plan` struct

**install_deps.go:**
- Change cache key generation to use content hash after plan generation

**validate-golden.sh:**
- Update jq filter to exclude `recipe_hash` (now automatic since field doesn't exist)

### Implementation Notes

**Hash Normalization**: Go maps have non-deterministic iteration order. The `planContentForHashing()` function must canonicalize the plan structure before JSON marshaling to ensure identical plans produce identical hashes. This can be achieved by:
1. Using `json.Marshal` with sorted map keys (Go 1.12+ sorts by default)
2. Using a struct with fixed field order for the normalized representation

**Cache Key Timing**: The current cache lookup happens before plan generation. With content-based hashing, the flow changes:
1. Resolve version
2. Check cache with `(tool, version, platform)` key (without content hash)
3. If cache hit: compute content hash of cached plan, compare to freshly generated plan
4. If cache miss or mismatch: generate fresh plan

This is a minor performance trade-off (may generate plan even with cache hit) but ensures correctness.

### Data Flow

```
Installation Flow (no change to user experience):
1. Load recipe
2. Resolve version
3. Generate plan (no recipe_hash computed)
4. Compute content hash of generated plan
5. Check cache with content hash
6. If miss: save plan, download assets
7. Execute plan (checksums verified)
```

## Implementation Approach

### Phase 1: Prep Validation Scripts

Before code changes, update validation to be forward-compatible:
1. Update `validate-golden.sh` to strip `recipe_hash` during comparison (via jq filter)
2. This allows the migration to proceed without CI failures

### Phase 2: Code Changes

1. Remove `RecipeHash` from plan structs (`plan.go`)
2. Remove `RecipeHash` from storage plan (`state.go`)
3. Update plan conversion functions (`plan_conversion.go`)
4. Remove `computeRecipeHash()` function
5. Add `computePlanContentHash()` with deterministic marshaling
6. Update `PlanCacheKey` to use `ContentHash`
7. Update `ValidateCachedPlan()` logic
8. Update cache flow in `install_deps.go`
9. Bump `PlanFormatVersion` from 3 to 4

### Phase 3: Test Updates

1. Update plan struct tests (`plan_test.go`)
2. Update cache validation tests (`plan_cache_test.go`)
3. Update plan generator tests (`plan_generator_test.go`)
4. Update plan conversion tests (`plan_conversion_test.go`)
5. Update state tests (`state_test.go`)
6. Update install deps tests (`install_deps_test.go`)
7. Add portability test: verify two different recipes producing identical plans have same content hash

### Phase 4: Golden File Migration (Local)

1. Update `regenerate-golden.sh` to generate new format (v4)
2. Run `./scripts/regenerate-all-golden.sh` to update all ~600 local files
3. Commit golden files atomically with code changes

### Phase 5: Golden File Migration (R2)

1. Update `publish-golden-to-r2.yml` to generate new format
2. Trigger manual regeneration for all recipes:
   ```bash
   gh workflow run publish-golden-to-r2.yml --ref main -f recipes="*"
   ```
3. Verify via nightly validation workflow

### Phase 6: Cleanup

1. Remove `recipe_hash` from R2 object metadata (`x-tsuku-recipe-hash`)
2. Revert validation script changes from Phase 1 (no longer needed)
3. Update documentation to clarify that plans are self-contained artifacts

## Security Considerations

### Download Verification

**Not affected.** Download checksums are embedded in plan steps and verified at execution time. This is unchanged and provides the actual security guarantee.

### Execution Isolation

**Not applicable.** This change doesn't affect how plans are executed or what permissions are required.

### Supply Chain Risks

**Not affected.** Plans still capture checksums at generation time from upstream sources. The security model is unchanged: if upstream is compromised at eval time, the plan inherits that compromise.

**Improvement**: By removing recipe_hash, we make it clearer that plans are self-contained artifacts. The security comes from checksums, not from recipe provenance.

### User Data Exposure

**Not applicable.** This change doesn't affect what data is accessed or transmitted.

## Consequences

### Positive

- Plans become portable across recipe sources
- Golden files change only when functional content changes
- Simpler mental model (plans are self-contained)
- Cache behavior based on what matters (plan content)
- Easier recipe maintenance (formatting changes don't cascade)

### Negative

- Breaking change requires format version bump (v3 → v4)
- All golden files must be regenerated (~600 local + R2)
- Cached plans from v3 format will be invalidated (one-time cost)

### Mitigations

- Format version bump provides clean migration path
- Regeneration scripts already exist and are automated
- Cache invalidation is a one-time cost per installation

## Implementation Issues

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| [#1584](https://github.com/tsukumogami/tsuku/issues/1584) | chore(golden): prep validation scripts for recipe_hash removal | None | simple |
| [#1585](https://github.com/tsukumogami/tsuku/issues/1585) | refactor(executor): remove recipe_hash from plan structs | [#1584](https://github.com/tsukumogami/tsuku/issues/1584) | testable |
| [#1586](https://github.com/tsukumogami/tsuku/issues/1586) | feat(executor): implement content-based plan hashing | [#1585](https://github.com/tsukumogami/tsuku/issues/1585) | testable |
| [#1587](https://github.com/tsukumogami/tsuku/issues/1587) | test(executor): update tests for content-based caching | [#1586](https://github.com/tsukumogami/tsuku/issues/1586) | testable |
| [#1588](https://github.com/tsukumogami/tsuku/issues/1588) | chore(golden): regenerate local golden files for v4 format | [#1587](https://github.com/tsukumogami/tsuku/issues/1587) | simple |
| [#1589](https://github.com/tsukumogami/tsuku/issues/1589) | chore(golden): regenerate R2 golden files and cleanup | [#1588](https://github.com/tsukumogami/tsuku/issues/1588) | simple |

**Milestone:** [Plan Hash Removal](https://github.com/tsukumogami/tsuku/milestone/75)

```mermaid
graph LR
    I1584["#1584: prep validation"]
    I1585["#1585: remove RecipeHash"]
    I1586["#1586: content hashing"]
    I1587["#1587: test updates"]
    I1588["#1588: local golden files"]
    I1589["#1589: R2 + cleanup"]

    I1584 --> I1585
    I1585 --> I1586
    I1586 --> I1587
    I1587 --> I1588
    I1588 --> I1589

    classDef done fill:#c8e6c9
    classDef ready fill:#bbdefb
    classDef blocked fill:#fff9c4
    classDef needsDesign fill:#e1bee7

    class I1584 done
    class I1585 ready
    class I1586,I1587,I1588,I1589 blocked
```

**Legend**: Green = done, Blue = ready, Yellow = blocked, Purple = needs-design
