# Maintainer Review: #1826 - Add satisfies metadata field and loader fallback

**Review focus**: maintainability (clarity, readability, duplication)
**Files changed**: `internal/recipe/types.go`, `internal/recipe/loader.go`, `internal/recipe/validate.go`, `internal/recipe/recipes/openssl.toml`, `internal/recipe/satisfies_test.go`

## Findings

### 1. `ToTOML()` silently drops `Satisfies` field -- Advisory

**File**: `internal/recipe/types.go:52-69`
**Severity**: Advisory

`ToTOML()` manually serializes each metadata field but does not include the new `Satisfies` map. A recipe round-tripped through `ToTOML()` -> parse will silently lose its satisfies data.

Current callers (`internal/validate/executor.go:211`, `internal/validate/source_build.go:127`) serialize recipes for verification lookup, where `satisfies` isn't needed. So there's no functional bug today. But the next developer who uses `ToTOML()` for a different purpose (e.g., recipe export, recipe editing) will lose satisfies data with no warning.

**Suggestion**: Add a serialization block for `Satisfies` in `ToTOML()` (after the VersionFormat block), or add a comment on the `Satisfies` field noting it is intentionally excluded from `ToTOML()` and why.

Not blocking because the current callers don't need this data, and `ToTOML()` already omits many metadata fields (Dependencies, RuntimeDependencies, Tier, Type, etc.), so the pattern of incomplete serialization is pre-existing. The new code follows the existing pattern.

### 2. Recursive `GetWithContext` call in satisfies fallback -- Advisory

**File**: `internal/recipe/loader.go:134-135`

```go
if canonicalName, ok := l.lookupSatisfies(name); ok {
    return l.GetWithContext(ctx, canonicalName, opts)
}
```

The recursive call passes the same `opts` through. If a satisfies entry points to a name that also doesn't exist (a bad data scenario -- e.g., embedded recipe deleted but satisfies index cached), the recursive call will:
1. Miss all 4 tiers
2. Hit `lookupSatisfies` again, which returns `false` (the canonical name isn't in the index)
3. Return the registry error

So infinite recursion can't happen because the canonical name won't be in the satisfies index (the index maps alias -> canonical, not canonical -> anything). This is safe but non-obvious. The next developer seeing this recursion will need to reason through the index structure to confirm it terminates.

**Suggestion**: A one-line comment above the recursive call would save the next reader that reasoning:
```go
// Safe: canonical names aren't in the satisfies index, so recursion depth is at most 1
```

### 3. `buildSatisfiesIndex` re-parses every embedded recipe -- Advisory

**File**: `internal/recipe/loader.go:290-316`

The index builder calls `toml.Unmarshal` on every embedded recipe file. These same recipes may already be parsed and cached in `l.recipes`. The builder doesn't check the cache first.

For the current embedded recipe count (tens of files), this is negligible. But the code pattern is worth noting: if the embedded set grows significantly, this duplicates parsing work. The lazy-once pattern means it happens at most once per Loader lifetime, which is a strong mitigating factor.

Not a readability concern -- the code is clear about what it does. Just noting for future scaling context.

### 4. Clear naming and consistent structure -- Positive

The three-level API (`lookupSatisfies` private, `lookupSatisfiesEmbeddedOnly` private, `LookupSatisfies` public) is well-named. The private methods clearly indicate their restriction (embedded-only vs. all sources), and the public method has a godoc comment explaining it's the API for downstream callers like #1827.

The `validateSatisfies` function in `validate.go` follows the same pattern as existing validation functions (`ValidateStructural` calls into it, returns `[]ValidationError`). The field path format (`metadata.satisfies.<ecosystem>`) is consistent with how other validation errors reference fields.

### 5. Test file organization and naming -- Positive

Tests are organized in clear sections with comment headers (`--- Schema Tests ---`, `--- Satisfies Index Tests ---`, etc.). Test names accurately describe what they test. The `setupSatisfiesTestLoader` helper is well-documented with its comment explaining why it pre-populates the index manually.

### 6. `ClearCache` resets `sync.Once` -- Advisory

**File**: `internal/recipe/loader.go:274-278`

```go
func (l *Loader) ClearCache() {
    l.recipes = make(map[string]*Recipe)
    l.satisfiesIndex = nil
    l.satisfiesOnce = sync.Once{}
}
```

Reassigning `sync.Once` with a zero value is valid Go but uncommon. The next developer might wonder if this is safe (it is -- the zero value of `sync.Once` is "not done"). A test exists (`TestSatisfies_ClearCacheResetsIndex`) that validates the behavior, which documents the intent.

This is fine as-is. The test coverage makes the intent clear.

### 7. Duplicate detection warning uses `fmt.Printf` -- Advisory

**File**: `internal/recipe/loader.go:310-311`

```go
fmt.Printf("Warning: duplicate satisfies entry %q (claimed by %q and %q)\n",
    pkgName, l.satisfiesIndex[pkgName], name)
```

This uses raw `fmt.Printf` for runtime warnings. Looking at the codebase, this matches the existing pattern in the same file (e.g., `loader.go:191-198` in `warnIfShadows`), so it's consistent. The design doc notes that CI-time validation (#1829) will catch duplicates before they reach runtime, so this is a belt-and-suspenders warning.

Consistent with existing patterns. No action needed.

## Summary

The implementation is clean and well-structured. Names match behavior, the public/private API boundary is clear, tests cover the interesting paths (lazy init, exact-match priority, embedded-only restriction, validation edge cases), and the code follows existing patterns in the loader and validation modules.

The only item worth flagging to future developers is the recursive `GetWithContext` call in the satisfies fallback. It's safe but requires understanding the index's key structure to see why it terminates. A one-line comment would prevent that reasoning detour.

The `ToTOML()` gap is pre-existing (many metadata fields are already omitted) and doesn't affect current callers, so it's not attributable to this change.

No blocking findings.
