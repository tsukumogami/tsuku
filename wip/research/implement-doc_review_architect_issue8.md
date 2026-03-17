# Architect Review: Issue 8

**Issue:** #8 feat(cli): add source-directed loading to update, outdated, and verify
**Focus:** architect
**Reviewer:** architect-reviewer

## Structural Assessment

### Pattern Consistency: Single Helper Pattern

After the scrutiny fix commit, all three commands (update, outdated, verify) now use the same `loadRecipeForTool()` helper for source-directed loading. This converges on a single pattern:

1. Read `ToolState.Source` from state
2. If distributed (`strings.Contains(source, "/")`), call `addDistributedProvider` to register the provider, then `loader.GetFromSource()` for targeted fetch
3. If non-distributed or on error, fall back to `loader.Get()` (normal chain)

This is structurally sound. No parallel patterns for the same concern.

### Provider Chain Integration

The implementation correctly routes through the existing provider architecture:

- **`GetFromSource`** (loader.go:185): A new Loader method that bypasses the priority chain to fetch from a specific provider by source string. This is architecturally appropriate -- it's a peer to `GetWithContext` (chain-based) and `GetWithSource` (chain-based + source tracking), adding a third resolution strategy at the same abstraction level.

- **`ParseAndCache`** (loader.go:445): Parses raw bytes and caches the result. Used by `loadRecipeForTool` to bridge `GetFromSource` (returns `[]byte`) with consumers that need `*Recipe`. This belongs on the Loader because it uses `parseBytes` (validation, step analysis) and writes to the in-memory cache. Not a bypass.

- **`addDistributedProvider`** (install_distributed.go:146): Reused from Issue 7 for dynamically adding a provider. No new registration path.

### Placement of `loadRecipeForTool`

The shared helper lives in `outdated.go` (lines 141-175) and is called from `update.go` and `verify.go`. All three files are in `package main` (cmd/tsuku), so this is fine at the Go level. However, `isDistributedSource` lives in `update.go` (line 106) while `loadRecipeForTool` (which calls it) lives in `outdated.go`. This cross-file dependency within the same package works but the placement is arbitrary.

**Severity: Advisory.** The function placement is a minor organizational concern -- both functions could move to a dedicated `source_directed.go` file for clarity. But since they're in the same package and the pattern is clean, this doesn't compound.

### Loader API Surface Growth

Issue 8 adds `ParseAndCache` to the Loader. Combined with Issue 7's additions (`CacheRecipe`, `AddProvider`), the Loader now has three new public methods beyond the original provider-based API. These are all justified:

- `AddProvider`: dynamic provider registration (Issue 7)
- `CacheRecipe`: bare-name aliasing for dependency resolution (Issue 7)
- `ParseAndCache`: bridge between `GetFromSource` raw bytes and parsed cache (Issue 8)

Each serves a distinct purpose and doesn't duplicate existing functionality. The Loader remains the single point of recipe resolution -- no bypass.

### State Contract Compliance

`ToolState.Source` (added in Issue 2) is now read by three consumers: update, outdated, and verify. No new state fields are added. No state fields are orphaned by these changes.

### Dependency Direction

All imports flow correctly:
- `cmd/tsuku` -> `internal/recipe` (Loader, RecipeSource, LoaderOptions)
- `cmd/tsuku` -> `internal/install` (State, ToolState)
- `cmd/tsuku` -> `internal/config` (Config)
- `cmd/tsuku` -> `internal/distributed` (via `addDistributedProvider` in install_distributed.go)

No lower-level packages import higher-level ones. No circular dependencies.

### `update.go` CacheRecipe Pattern

The update command's approach (line 68-70) is:
```go
if r, err := loadRecipeForTool(...); err == nil && r != nil {
    loader.CacheRecipe(toolName, r)
}
```

This pre-populates the cache before `runInstallWithTelemetry` runs. The install flow then calls `loader.Get(toolName, ...)` and gets the cached recipe. This is the same pattern used by the install flow for distributed recipes (install.go:241, 373). Consistent.

### Design Doc Alignment

The design doc (PLAN-distributed-recipes.md, Issue 8 section) specifies:
- update: reads `ToolState.Source`, calls `GetFromSource` -- **satisfied** (via `loadRecipeForTool`)
- outdated: iterates tools against recorded source -- **satisfied**
- verify: uses cached recipe from recorded source -- **satisfied**

The implementation matches the design doc's structural intent after the scrutiny fix.

## Findings

### Finding 1: Helper function placement is arbitrary

**File:** `cmd/tsuku/outdated.go:141-175` (loadRecipeForTool), `cmd/tsuku/update.go:106-108` (isDistributedSource)
**Severity:** Advisory

`loadRecipeForTool` is defined in `outdated.go` but called from `update.go` and `verify.go`. `isDistributedSource` is defined in `update.go` but called from `outdated.go` (via `loadRecipeForTool`). These cross-references within the package work but the placement doesn't signal that these are shared utilities. A `source_directed.go` file (matching the test file name `source_directed_test.go`) would be more discoverable.

This doesn't compound -- the functions are correctly scoped and tested. No structural fix needed.

## Summary

No blocking findings. The implementation follows the established provider architecture: source-directed loading routes through `GetFromSource` on the Loader, distributed providers are registered via the existing `addDistributedProvider`, and all three commands converge on a single `loadRecipeForTool` helper. The Loader API additions (`ParseAndCache`) are justified bridges between raw-bytes and parsed-recipe layers, not bypasses.
