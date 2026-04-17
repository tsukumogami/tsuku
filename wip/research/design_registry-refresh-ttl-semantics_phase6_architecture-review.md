# Architecture Review: Registry Refresh TTL Semantics

## What the design proposes

The design calls for removing the `--all` flag from `tsuku update-registry` and making unconditional force refresh the only behavior for the command. The claimed change:

- Remove `registryRefreshAll` flag variable and `--all` from `init()`
- Remove `forceRegistryRefreshAll` helper
- Rewrite `runRegistryRefreshAll` to always call `cachedReg.Refresh(ctx, name)` per recipe
- Assemble `RefreshStats` inline rather than delegating to the removed helper

## Findings

### 1. The architecture is accurate and implementable

The proposed data flow matches the actual code exactly. `cachedReg.Refresh(ctx, name)` already performs unconditional network fetch (no TTL check). `cachedReg.RefreshAll(ctx)` is TTL-aware (skips fresh entries). The distinction is real and already encoded in the library layer.

### 2. `forceRegistryRefreshAll` already exists and already does what the new `runRegistryRefreshAll` should do

Lines 212-238 of `update_registry.go` contain `forceRegistryRefreshAll`. The proposed rewrite would produce functionally identical code inlined into `runRegistryRefreshAll`. There is no benefit to inlining -- the simpler path is to rename `forceRegistryRefreshAll` to become the body of `runRegistryRefreshAll` (or just call it directly after removing the `if registryRefreshAll` branch). Either way is straightforward.

### 3. `RefreshAll` becomes dead code at the command layer

After the change, `cachedReg.RefreshAll(ctx)` is no longer called from `cmd/`. It remains available for automated callers (background update checks, etc.) as the design states, so it should not be removed from the package. No action needed here, but the design should note that `RefreshAll` stays to preserve the internal API.

### 4. Phase sequencing is correct

Phase 1 (rewrite function + remove flag) must precede Phase 2 (update help text + tests) because Phase 2 test changes will fail to compile if Phase 1 hasn't removed the flag variable that some tests may reference. However, no existing tests cover `runRegistryRefreshAll` or `forceRegistryRefreshAll` directly -- all current tests target `refreshDistributedSources`. Phase 2 test work is therefore additive (new tests for the rewritten path), not a fix to broken existing tests.

### 5. Missing: test coverage for the new unconditional path

The design mentions updating `update_registry_test.go` but does not specify what test cases to add. The rewritten `runRegistryRefreshAll` should be tested for: empty cache, all-success, partial-error, and the cache-clear (`loader.ClearCache()`) side effect. This should be called out explicitly in Phase 2 deliverables.

### 6. No simpler alternatives overlooked

The design is already at minimum viable scope. The only plausible simplification -- delegating to `RefreshAll` with a `forceAll` parameter -- would mean modifying the `CachedRegistry` API, which the design correctly avoids.

## Recommendations

1. The architecture is sound and ready to implement as described.
2. In Phase 1, replace the `if registryRefreshAll { ... } else { ... }` branch with a direct call body (inline or via the existing helper -- both work).
3. In Phase 2, enumerate test cases explicitly: empty cache, all-success, partial-error, and cache-clear verification.
4. Note in the design that `CachedRegistry.RefreshAll` is intentionally retained for non-command callers.
5. Help text update (removing `--all` mention from `Long`) is a Phase 1 concern, not Phase 2 -- it belongs with the flag removal to keep the command consistent at each commit.
