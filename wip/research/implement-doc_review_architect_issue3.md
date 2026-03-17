# Architecture Review: Issue 3 (feat(config): add registry configuration and GetFromSource)

**Reviewer:** architect-reviewer
**Scope:** structural fit, dependency direction, pattern consistency

## Summary

The changes fit cleanly into the existing architecture. Registry configuration extends `userconfig.Config` without introducing new packages or parallel patterns. `GetFromSource` is a well-placed method on `Loader` that reuses the existing `RecipeProvider` interface and provider chain. No blocking findings.

## Findings

### 1. Advisory: SourceCentral is untyped while siblings are typed

`internal/recipe/loader.go:523` -- `SourceCentral` is declared as an untyped string constant (`SourceCentral = "central"`), while `SourceLocal`, `SourceRegistry`, and `SourceEmbedded` are all typed `RecipeSource`. This is intentional (it's a user-facing alias, not a provider tag), but it's subtle. A reader might expect all constants in the block to share the same type. The inconsistency is contained -- `GetFromSource` takes `string`, not `RecipeSource`, so the code compiles correctly -- but a brief comment explaining the deliberate type difference would prevent future contributors from "fixing" it by adding the type.

**Severity: Advisory.** No structural impact; the distinction is correct for the use case.

### 2. Advisory: GetFromSource central case hardcodes provider traversal order

`internal/recipe/loader.go:122-143` -- The `SourceCentral` branch iterates all providers twice: first scanning for `SourceRegistry`, then for `SourceEmbedded`. This is distinct from `resolveFromChain`, which traverses once in insertion order. The semantic difference is intentional (registry always wins over embedded for source-directed lookups regardless of provider ordering), but the two-pass approach means if there are ever multiple registry providers, all of them are tried before any embedded provider. Today there's only one of each, so this is fine. If the provider set grows, the behavior is still correct but costs an extra linear scan per call.

**Severity: Advisory.** Contained to one method; no callers will copy this pattern since `GetFromSource` is the only source-directed entry point.

### 3. No findings: dependency direction

`userconfig` and `recipe` packages have no cross-imports. The registry configuration in `userconfig` is pure data (struct + TOML tags). The recipe loader consumes source strings, not config structs. A later issue will need to wire them together, but that wiring belongs in the command layer (`cmd/`), which sits above both. The dependency direction is clean.

### 4. No findings: provider interface compliance

`GetFromSource` delegates to `RecipeProvider.Get()` for all sources (central, local, distributed). It does not bypass the provider interface or instantiate providers inline. The mock provider in tests implements the same interface. The extensibility pattern is preserved.

### 5. No findings: cache contract

`GetFromSource` explicitly does not read from or write to the `recipes` cache (verified by tests `TestLoader_GetFromSource_BypassesCache` and `TestLoader_GetFromSource_DoesNotWriteToCache`). This matches the acceptance criteria and avoids stale-cache bugs for source-directed operations.

### 6. No findings: TOML round-trip for registry config

`RegistryEntry` and the new `Config` fields use `omitempty`, so empty/default values produce no spurious output. Test `TestRegistryEmptyMapDoesNotProduceSpuriousTOML` validates this. Backward compatibility is covered by `TestRegistryBackwardCompat_NoRegistries`.

## Verdict

**0 blocking, 2 advisory.** The changes extend the existing architecture without introducing parallel patterns or violating dependency direction. Both advisory items are cosmetic and contained.
