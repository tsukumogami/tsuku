# Maintainer Review: Issue 1 - RecipeProvider interface and Loader refactor

## Summary

The refactor from a hardcoded four-source Loader to `[]RecipeProvider` is clean and well-structured. The core abstractions (RecipeProvider, SatisfiesProvider, RefreshableProvider) are easy to understand and the provider chain resolution logic in `resolveFromChain` is straightforward. Tests cover the important behavioral paths including cycle prevention, priority ordering, and embedded-only filtering.

## Findings

### 1. `Loader.Registry()` returns `interface{}` and has no callers -- dead code that misleads

**File:** `internal/recipe/loader.go:265`
**Severity:** Blocking

`Loader.Registry()` returns `interface{}`, which discards all type information. The next developer will see this method, assume it's the intended way to get the registry, and write code like `loader.Registry().(*registry.Registry)` -- not realizing the actual call site (`cmd/tsuku/update_registry.go:33-43`) uses `ProviderBySource` + type assertion on `*CentralRegistryProvider` instead.

Grepping confirms zero callers of `loader.Registry()`. The `update_registry.go` command already uses the `ProviderBySource` escape hatch as designed. This method is dead code left from the refactor that will confuse the next person into using the wrong access pattern.

**Suggestion:** Remove `Loader.Registry()` entirely. The `ProviderBySource` + type assertion pattern documented in the comment on line 252 is the intended escape hatch and is already used correctly.

### 2. `warnIfShadows` uses concrete type assertions, defeating the provider abstraction

**File:** `internal/recipe/loader.go:189-212`
**Severity:** Advisory

`warnIfShadows` type-asserts `*EmbeddedProvider` and `*CentralRegistryProvider` to check if they contain a recipe, rather than calling `p.Get()` on the providers. The next developer adding a new provider type (e.g., `DistributedProvider` from Issue 6) would expect this method to detect shadowing automatically -- but it silently skips any provider it doesn't recognize by concrete type.

The method could call `p.Get(ctx, name)` on non-local providers instead, which would work for any provider implementing the interface. The registry check intentionally avoids network (uses `GetCached`), but `CentralRegistryProvider.Get()` already checks cache first, and the embedded provider's `Get()` is purely in-memory. The only concern is preventing a network fetch for an uncached registry recipe during a shadow warning -- which is a legitimate reason for the current approach.

This won't cause a bug today, but it's a trap for Issue 6. A comment explaining "concrete type assertions are intentional to avoid network I/O during shadow detection" would prevent the next developer from thinking this is an oversight.

### 3. Divergent satisfies-parsing logic across LocalProvider and EmbeddedProvider

**File:** `internal/recipe/provider_local.go:84-127` and `internal/recipe/provider_embedded.go:44-69`
**Severity:** Advisory

Both `LocalProvider.SatisfiesEntries()` and `EmbeddedProvider.SatisfiesEntries()` contain nearly identical loops: unmarshal TOML, iterate `r.Metadata.Satisfies`, check for duplicates, print warnings. The only differences are how they read bytes (filesystem vs. embedded registry) and the duplicate-warning format. `CentralRegistryProvider.SatisfiesEntries()` takes a different path (reads from manifest JSON) which is appropriately separate.

If the satisfies iteration logic needs to change (e.g., to support ecosystem-scoped keys per the design doc), a developer would need to update both Local and Embedded -- and might miss one. Consider extracting the common "iterate satisfies map, build result, warn on duplicates" into a shared helper that takes `map[string][]string` (the already-parsed satisfies map).

### 4. `fmt.Printf` for warnings bypasses any structured logging or output control

**File:** `provider_local.go:119`, `provider_embedded.go:62`, `provider_registry.go:42`, `loader.go:197,206`
**Severity:** Advisory

Five places use `fmt.Printf("Warning: ...")` to emit warnings. This goes directly to stdout, which means:
- Tests can't capture or assert on warnings
- If a caller wants to suppress warnings (e.g., in a library context), there's no way to do so
- The warnings mix with command output

This is a pre-existing pattern in the codebase, so it's not a regression from this refactor. But now that warnings come from providers (a layer below the CLI), it would be worth noting in a follow-up that these should eventually use a logger or callback.

### 5. `isNotFoundError` relies on string matching in error messages

**File:** `internal/recipe/loader.go:424-436`
**Severity:** Advisory

`isNotFoundError` and `containsNotFound` check for substrings "not found" and "no such file" in `err.Error()`. If any provider returns an error whose message happens to contain "not found" for a non-404 reason (e.g., "configuration not found" from a network error), `resolveFromChain` will swallow it and continue to the next provider instead of failing fast.

This is fragile but pre-existing. The design doc's `ErrRecipeNotFound` sentinel would be the clean fix. Since Issue 1 is a pure refactor, this is out of scope, but worth noting that `resolveFromChain`'s error classification depends on this.

## Overall Assessment

The refactor is well-executed. The provider interface is minimal and clear, `resolveFromChain` replaces four separate methods with a single generic one, and the satisfies index correctly tags entries by source for filtered lookups. The test suite covers priority ordering, shadowing, cycle prevention, and embedded-only paths.

The one blocking finding is the dead `Loader.Registry()` method returning `interface{}` -- it will actively mislead the next developer into using a defunct API path. The advisory findings are real but won't cause misreads that lead to bugs in the near term.
