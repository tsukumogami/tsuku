# Architect Review: Issue #1 - RecipeProvider Interface and Loader Refactor

## Summary

Issue #1 extracts a `RecipeProvider` interface and refactors the Loader from a hardcoded priority chain into an ordered `[]RecipeProvider` slice. This is the foundation for the distributed recipes feature.

## Findings

### 1. warnIfShadows type-asserts to concrete providers (Advisory)

**File:** `internal/recipe/loader.go:189-212`

`warnIfShadows` type-asserts to `*EmbeddedProvider` and `*CentralRegistryProvider` to check for shadowing. This partially defeats the provider abstraction -- if a new provider type is added (e.g., `DistributedProvider`), this function won't detect shadowing against it.

The design doc's acceptance criteria says: "warnIfShadows refactored to detect shadowing across providers instead of hardcoding `l.registry.GetCached()`." The current implementation replaces the registry-specific hardcoding with provider-type-specific hardcoding. It's better than before (uses the provider list instead of a direct field), but it still requires knowledge of concrete types.

A fully generic approach would call `p.Get(ctx, name)` on lower-priority providers, but that could trigger network fetches for the registry provider. The current approach is a pragmatic compromise -- it avoids network calls by using `GetCached`/`Has` -- but it will need updating when `DistributedProvider` arrives.

**Advisory**, not blocking: the function only has one caller, the pattern doesn't compound (Issue #6 will need to update this one function), and the workaround is documented by the concrete type checks being visible in the code.

### 2. CentralRegistryProvider.Refresh is a no-op (Advisory)

**File:** `internal/recipe/provider_registry.go:105-112`

`CentralRegistryProvider` declares it implements `RefreshableProvider` but `Refresh()` is a no-op that returns nil. The real refresh logic lives in `update_registry.go` via the `Registry()` escape hatch.

This means code that iterates providers calling `Refresh()` (planned for Issue #10) will silently skip the central registry. The escape hatch is documented and the design doc explicitly calls this out, so the intent is clear. But the no-op implementation means `CentralRegistryProvider` claims a capability it doesn't deliver through the interface.

**Advisory**: The design doc explicitly blesses this as an intentional escape hatch. Issue #10 will need to handle this when extending `update-registry` to distributed sources. Not blocking because the no-op is documented in comments and the escape hatch pattern is used by exactly one command.

### 3. Loader.Registry() returns interface{} (Advisory)

**File:** `internal/recipe/loader.go:262-274`

`Loader.Registry()` returns `interface{}` instead of `*registry.Registry`. This was likely done to avoid importing `internal/registry` from `internal/recipe`. However, callers must type-assert the return value, which is fragile and provides no compile-time safety.

Checking the codebase, `loader.Registry()` doesn't appear to have any callers outside tests -- `update_registry.go` uses `ProviderBySource()` + type-assertion directly (the documented escape hatch). If this method has no real callers, it should be removed. If it's intended for future use, returning a concrete type via a method on `CentralRegistryProvider` (which already has `Registry()` returning `*registry.Registry`) would be cleaner, and that path already exists via `ProviderBySource`.

**Advisory**: No callers use this method currently. It's dead code that introduces an untyped return value. Low priority but worth removing to avoid confusion.

### 4. Design alignment: all acceptance criteria met

The implementation matches the design doc's intent for Issue #1:

- `RecipeProvider`, `SatisfiesProvider`, `RefreshableProvider` interfaces defined in `provider.go`
- `satisfiesEntry` struct tracks source for filtered lookups
- `LocalProvider`, `EmbeddedProvider`, `CentralRegistryProvider` adapters wrapping existing sources
- Loader holds `[]RecipeProvider`, single `NewLoader(providers ...)` constructor
- Four chain methods collapsed into `resolveFromChain`
- `RequireEmbedded` filters providers by source
- `ProviderBySource()` escape hatch for `update-registry`
- In-memory cache stays in Loader, providers return `[]byte`
- Consumer-side interfaces (`RecipeLoader` in actions and verify) unchanged
- All call sites (main.go, tests) updated to build provider slices

### 5. Dependency direction is correct

`internal/recipe` imports `internal/registry` (for `CentralRegistryProvider`). This is the expected direction: the recipe package (which owns the Loader) depends on the registry package (which owns `*Registry`). The reverse doesn't happen.

### 6. Provider registration follows existing patterns

The version provider system in `internal/version/` uses a similar pluggable pattern (providers implement an interface, registered by name). The `RecipeProvider` pattern mirrors this. Adding `DistributedProvider` in Issue #6 will be a single implementation + registration in the provider chain, matching the extensibility model.

## Overall Assessment

The implementation is a clean extraction of the provider interface that aligns with the design doc and follows the codebase's existing patterns. The Loader refactor is surgical -- it replaces hardcoded source chains with an ordered provider slice without changing the public API or consumer interfaces.

The two advisory items (`warnIfShadows` concrete type knowledge, no-op `Refresh`) are documented trade-offs that will need targeted updates when `DistributedProvider` arrives in Issue #6, but neither compounds into a structural problem. The dead `Registry() interface{}` method is a minor cleanup opportunity.

No blocking issues found.
