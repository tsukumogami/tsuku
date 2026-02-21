# Architectural Review: Issue #1827
**Issue**: feat(cli): check satisfies index before generating recipes in tsuku create
**Focus**: architecture (design patterns, separation of concerns)
**Reviewer**: architect-reviewer
**Date**: 2026-02-21

---

## Summary

The implementation is architecturally clean. No blocking findings. Two advisory observations.

---

## Architecture Map Applied

Relevant layers for this change:
- **CLI surface** (`cmd/tsuku/create.go`): The `tsuku create` command is the entry point being modified.
- **Version providers / Recipe loader** (`internal/recipe/loader.go`): The satisfies index and `GetWithContext()` were established in #1826 as the single integration point for ecosystem name resolution.

---

## Design Alignment

The design doc's Phase 2 decision is explicit: update `tsuku create` to check the loader (including satisfies fallback) before generating, with `--force` override. The implementation follows this exactly.

**Integration point**: `checkExistingRecipe()` at `cmd/tsuku/create.go:467` calls `l.GetWithContext(context.Background(), toolName, recipe.LoaderOptions{})`. This goes through the full 4-tier chain plus satisfies fallback defined in `internal/recipe/loader.go:87-143`. The design called for "every code path benefits automatically through the loader" -- this implementation honors that by using the standard `GetWithContext` rather than calling `LookupSatisfies` directly.

The design doc also exposes `LookupSatisfies()` as a public API on `Loader` (line 425 of `loader.go`). The implementation does not use it. Instead it uses `GetWithContext`, which internally triggers the satisfies fallback. This is actually the *better* choice architecturally: it means the `create` command gets the full resolution chain (cache, local, embedded, registry, satisfies) rather than just the satisfies index. Using `LookupSatisfies` directly would have bypassed the first four tiers, missing cases where an exact-name recipe exists in the registry. The implementation is more correct than the design's suggested public API implied.

---

## Pattern Consistency

**Loader usage**: The `loader` global variable (`cmd/tsuku/helpers.go:19`) is used consistently across all commands. This issue uses the same shared `loader` instance. No inline instantiation.

**Flag wiring**: `createForce` is already a registered cobra flag (`cmd/tsuku/create.go:134`). The new guard at line 485 (`if !createForce`) follows the same pattern as the existing guard at line 773. Both use the same flag variable.

**Error reporting**: `fmt.Fprintf(os.Stderr, "Error: ...")` followed by `exitWithCode(ExitGeneral)` matches the existing error handling pattern throughout `create.go`.

**Helper function extraction**: `checkExistingRecipe()` is a package-level function with a `*recipe.Loader` parameter rather than accessing `loader` directly. This is consistent with how other helpers in this file are structured and makes the function testable without global state.

---

## Separation of Concerns

**Appropriate.** The `checkExistingRecipe()` function wraps loader lookup and returns `(string, bool)`. It doesn't print, exit, or interpret the result -- all of that stays in `runCreate()`. The function has exactly one responsibility: determine whether a recipe exists.

The `loader` global is accessed in `runCreate()` at the call site, not inside the helper. This is the correct pattern: the helper is pure lookup logic; the command handler owns the decision and side effects.

---

## Dependency Direction

`cmd/tsuku/create.go` imports `internal/recipe`. This is the expected direction: CLI layer depends on internal packages, not the reverse. No violation.

---

## Advisory Finding 1: `LookupSatisfies` is now an unused public API

`internal/recipe/loader.go:425` exposes `LookupSatisfies()` as a public method with a comment saying "This is the public API for downstream callers (e.g., tsuku create in #1827)." Issue #1827 is the only planned consumer, but the implementation uses `GetWithContext` instead.

This is advisory, not blocking. The method doesn't introduce a parallel pattern -- it's a thin wrapper over the private `lookupSatisfies()` and causes no divergence. However, the comment is now misleading: `tsuku create` uses `GetWithContext`, not `LookupSatisfies`. If #1829 (registry integration) or a future caller wants only the satisfies lookup without the full tier chain, `LookupSatisfies` remains available. The issue is documentation drift, not structural damage.

**Suggestion**: Update the comment on `LookupSatisfies()` to remove the `#1827` reference and instead describe its actual intended use case (callers that want satisfies-only lookup without the full resolution chain). This is a one-line fix.

---

## Advisory Finding 2: `--force` flag description doesn't reflect expanded scope

`cmd/tsuku/create.go:134`:
```go
createCmd.Flags().BoolVar(&createForce, "force", false, "Overwrite existing local recipe")
```

The flag description says "Overwrite existing local recipe" but `--force` now overrides two distinct checks: (1) the new satisfies/loader check (line 485) and (2) the existing `os.Stat` check for custom output paths (line 773). The description only covers case 2.

This is advisory because the flag still works correctly and the behavior is consistent (force means force). The description just doesn't tell users that `--force` also bypasses the satisfies duplicate check. Users running `tsuku create openssl@3 --help` would see the old description and not know `--force` helps them.

**Suggestion**: Change the description to something like `"Override duplicate detection and overwrite existing recipe files"` to cover both cases the flag guards.

---

## Extensibility

The design doc anticipates downstream issues #1828 (data cleanup) and #1829 (registry manifest integration). This implementation doesn't constrain either:

- #1828 deletes `openssl@3.toml` and adds `satisfies` entries to recipes. The `checkExistingRecipe` logic is unaffected -- once `openssl@3.toml` is deleted, `create openssl@3` will fall through to the satisfies fallback exactly as designed.
- #1829 adds registry manifest entries to the satisfies index. The index is built in `buildSatisfiesIndex()` in the loader. `checkExistingRecipe()` in create.go calls `GetWithContext()` which calls `lookupSatisfies()` which builds the index. No changes to create.go will be needed when #1829 adds manifest entries.

The design's single integration point goal is preserved.

---

## Findings Summary

| Severity | Location | Description |
|----------|----------|-------------|
| Advisory | `internal/recipe/loader.go:423-427` | `LookupSatisfies()` comment references `#1827` as consumer but #1827 uses `GetWithContext` instead. Misleading documentation. |
| Advisory | `cmd/tsuku/create.go:134` | `--force` flag description only mentions "Overwrite existing local recipe", doesn't cover the new satisfies duplicate check bypass. |

No blocking findings.
