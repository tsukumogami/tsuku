# Architecture Review: Issue #1826 -- satisfies metadata field and loader fallback

**Review focus**: architect (design patterns, separation of concerns)
**Files changed**: `internal/recipe/types.go`, `internal/recipe/loader.go`, `internal/recipe/validate.go`, `internal/recipe/recipes/openssl.toml`, `internal/recipe/satisfies_test.go`

---

## Finding 1: ToTOML does not serialize the Satisfies field

**File**: `internal/recipe/types.go:52-145` (ToTOML method)
**Severity**: Advisory

The `ToTOML()` method on `Recipe` manually serializes each metadata field but does not include the new `Satisfies` field. This means a recipe loaded from TOML and re-serialized via `ToTOML()` will silently drop its satisfies entries.

`ToTOML()` is called from `internal/validate/executor.go:211` and `internal/validate/source_build.go:127` -- both in the validation/build pipeline. If a recipe with `satisfies` entries passes through these paths, the serialized output won't include them.

This is advisory rather than blocking because: (1) the current consumers of `ToTOML()` are in the validation executor which re-parses for evaluation, not for persisting the recipe back, and (2) `satisfies` entries are currently only on embedded recipes (openssl) which don't go through the create/serialize pipeline. However, as more recipes gain `satisfies` entries and if `ToTOML()` is used by `tsuku create` or other serialization paths, the silent data loss will become a real bug.

**Suggestion**: Add serialization of the `Satisfies` field in `ToTOML()` alongside the other metadata fields, or add a comment explaining why it's intentionally omitted.

---

## Finding 2: Dual validation paths don't share satisfies validation

**File**: `internal/recipe/validate.go:55` vs `internal/recipe/validator.go:101-110`
**Severity**: Advisory

There are two independent validation paths in the recipe package:

1. `ValidateStructural()` in `validate.go` -- called by `ValidateFull()`, includes `validateSatisfies()` at line 55
2. `runRecipeValidations()` in `validator.go` -- called by `ValidateFile()`, `ValidateBytes()`, `ValidateRecipe()`. Does NOT include satisfies validation.

The `tsuku validate` CLI command calls `ValidateFile()` which routes through `runRecipeValidations()`, meaning satisfies validation doesn't run for CLI-level recipe validation. This is a pre-existing architectural split -- the two validation paths existed before this change. The implementation correctly added satisfies validation to `ValidateStructural` (which is the newer, more modular path), and didn't attempt to wire it into the older `validator.go` path.

This is advisory because it doesn't introduce a new architectural divergence -- it follows the existing split. But it's worth noting for #1829 (registry integration), which adds CI-time validation. The CI validation should use `ValidateStructural` or `ValidateFull` to get satisfies checks, not `ValidateFile`.

---

## Finding 3: Satisfies index placement and lazy init -- correct architectural fit

**File**: `internal/recipe/loader.go:30-31, 286-343`
**Severity**: No issue (positive observation)

The satisfies index lives on the `Loader` struct with `sync.Once` for lazy initialization. This matches the existing pattern for the `Loader` struct: it holds state (the `recipes` cache, the `embedded` registry, the `registry` client) and provides the single resolution path for all callers.

The design doc's key requirement -- "every code path that loads recipes automatically benefits" -- is achieved by placing the fallback in `GetWithContext()` at line 133-136, after the 4-tier chain. This is exactly the right integration point. No caller needs changes.

The `LookupSatisfies()` public method (line 345-350) provides an explicit lookup for #1827 (`tsuku create`), which is the correct way to expose index queries without bypassing the dispatch pattern. Callers that need name resolution without loading a full recipe can use this API.

---

## Finding 4: Recursive GetWithContext call preserves loader contract

**File**: `internal/recipe/loader.go:134-136`
**Severity**: No issue (positive observation)

```go
if canonicalName, ok := l.lookupSatisfies(name); ok {
    return l.GetWithContext(ctx, canonicalName, opts)
}
```

The recursive call back to `GetWithContext` means the resolved canonical name goes through the full 4-tier chain. The resolved recipe gets cached under its canonical name (not the alias), which is correct -- subsequent lookups for the canonical name hit the cache. There's no infinite recursion risk because the satisfies index maps aliases to canonical names that exist as real recipes.

---

## Finding 5: buildSatisfiesIndex re-parses embedded recipes

**File**: `internal/recipe/loader.go:290-316`
**Severity**: Advisory

`buildSatisfiesIndex()` calls `toml.Unmarshal` on every embedded recipe to extract satisfies entries. The embedded recipes are also parsed when loaded via `getEmbeddedOnly()` or the main `GetWithContext()` path. This means embedded recipes with satisfies entries are parsed twice: once during index build and once when actually loaded.

This is a minor inefficiency, not an architectural concern. The design doc anticipated this: "The index build scans embedded recipes (fast, tens of recipes)." With ~50 embedded recipes, the double-parse adds negligible overhead. The alternative (sharing parsed state between the index builder and the loader cache) would couple the two paths and add complexity for no measurable benefit.

---

## Finding 6: ClearCache resets satisfies index correctly

**File**: `internal/recipe/loader.go:274-278`
**Severity**: No issue (positive observation)

```go
func (l *Loader) ClearCache() {
    l.recipes = make(map[string]*Recipe)
    l.satisfiesIndex = nil
    l.satisfiesOnce = sync.Once{}
}
```

Resetting `satisfiesOnce` to a new `sync.Once{}` allows the index to be rebuilt on next fallback lookup. This follows the existing cache invalidation pattern (`recipes` map is also cleared). The `ClearCache` contract -- "forces recipes to be re-fetched" -- is extended cleanly to include the satisfies index.

---

## Finding 7: Dependency direction is correct

**Severity**: No issue

All changes are within the `internal/recipe` package. No new cross-package imports were introduced. The satisfies index is built from data within the recipe layer (embedded recipes, eventually registry manifest). The `Loader` doesn't import higher-level packages like `cmd/` or `internal/controller`.

---

## Summary

The implementation is a clean fit with the existing architecture. The satisfies index lives in the right place (on the Loader), integrates at the right point (after the 4-tier chain in GetWithContext), and follows existing patterns (lazy init with sync.Once, cache invalidation in ClearCache). The public API surface is minimal and well-placed.

Two advisory items:

1. `ToTOML()` doesn't serialize the `Satisfies` field. Currently non-impactful but will cause silent data loss if recipes with satisfies entries are ever serialized through that path.

2. The `validator.go:runRecipeValidations` path doesn't include satisfies validation. Pre-existing architectural split, not introduced by this change.

No blocking findings.
