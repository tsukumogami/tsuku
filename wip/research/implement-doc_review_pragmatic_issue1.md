# Pragmatic Review: Issue 1 (RecipeProvider interface refactor)

## Findings

### 1. Dead code: `Loader.Registry()` returns `interface{}` with zero callers

**File:** `internal/recipe/loader.go:262-274`
**Severity:** Blocking

`Loader.Registry()` is a convenience method that walks the provider list, finds `CentralRegistryProvider`, and returns its inner `*registry.Registry` as `interface{}`. It has zero callers anywhere in the codebase. The actual `update-registry` command uses `ProviderBySource()` + type-assertion instead (which is the documented pattern per the acceptance criteria). This method is dead weight and the `interface{}` return type is worse than useless -- any future caller would need a type assertion anyway.

**Fix:** Delete `Loader.Registry()` (lines 262-274).

---

### 2. Dead code: `Loader.ListEmbedded()` has zero callers

**File:** `internal/recipe/loader.go:392-399`
**Severity:** Blocking

`ListEmbedded()` is defined but never called from production code or tests. It's speculative API surface for a caller that doesn't exist.

**Fix:** Delete `ListEmbedded()` (lines 392-399).

---

### 3. No-op `CentralRegistryProvider.Refresh()` implements `RefreshableProvider` for future use

**File:** `internal/recipe/provider_registry.go:108-112`
**Severity:** Advisory

`Refresh()` returns nil and does nothing. The comment says the real logic lives in `update-registry`. `RefreshableProvider` itself has no callers doing type assertions. This is speculative generality -- Issue 10 will use it, but it doesn't exist yet. However, the interface is defined in the acceptance criteria for this issue, and the no-op is small and inert.

---

### 4. Two-function indirection for not-found check

**File:** `internal/recipe/loader.go:424-436`
**Severity:** Advisory

`isNotFoundError` delegates to `containsNotFound` as a separate function. `containsNotFound` has exactly one caller. Could be inlined into `isNotFoundError` but it's 3 lines and named clearly enough.

---

### 5. `warnIfShadows` uses type assertions to concrete types

**File:** `internal/recipe/loader.go:189-212`
**Severity:** Advisory

`warnIfShadows` type-asserts `*EmbeddedProvider` and `*CentralRegistryProvider` to check for shadowed recipes. This partially defeats the provider abstraction -- a future provider type won't get shadow warnings. The acceptance criteria says "refactored to detect shadowing across providers instead of hardcoding `l.registry.GetCached()`." The current approach still hardcodes provider types, just different ones. However, a generic approach would require a `Has(name)` method on the interface, which is scope creep for this issue. The current approach is correct for the three known provider types.

## Overall Assessment

The refactor is structurally sound. The provider chain, satisfies index, and resolution logic are clean and match the design doc's intent. Two dead code items (`Loader.Registry()` and `ListEmbedded()`) should be removed before merge -- they add API surface that will confuse future contributors about what's actually used. The advisory items are minor and bounded.
