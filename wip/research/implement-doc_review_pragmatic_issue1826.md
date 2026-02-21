# Review: #1826 - Pragmatic Focus

## Finding 1: Recursive `GetWithContext` call has no cycle guard (Blocking)

**File**: `internal/recipe/loader.go:134-135`

```go
if canonicalName, ok := l.lookupSatisfies(name); ok {
    return l.GetWithContext(ctx, canonicalName, opts)
}
```

If a satisfies entry maps to a canonical name that itself appears as a key in the satisfies index (e.g., recipe A satisfies "X", recipe B satisfies "A", and neither recipe file exists on disk), this recurses indefinitely. Validation catches self-referential entries on a *single* recipe, but cross-recipe cycles are not prevented by any validation in this issue. CI-time duplicate detection is deferred to #1829.

Same pattern exists in `getEmbeddedOnly` at line 162-163.

**Fix**: Add a `seen` set or single-level flag. Simplest: pass the original name through and refuse to re-enter the satisfies fallback. Or: don't recurse -- just call the 4-tier chain directly with `canonicalName`, skipping the satisfies fallback on that second call.

## Finding 2: `LookupSatisfies` public method has zero callers (Advisory)

**File**: `internal/recipe/loader.go:348-350`

```go
func (l *Loader) LookupSatisfies(name string) (string, bool) {
    return l.lookupSatisfies(name)
}
```

This is a public wrapper for #1827 (create command). The issue comment in the godoc says as much. As speculative generality it's minor since the body is a one-liner and the downstream issue exists. Not blocking, but could have been deferred to #1827's implementation.

## Finding 3: `ToTOML()` does not serialize `Satisfies` field (Advisory)

**File**: `internal/recipe/types.go:56-69`

`ToTOML()` manually builds the metadata section but omits `Satisfies`. Any recipe that roundtrips through `ToTOML()` (e.g., `tsuku create` output) will silently lose its satisfies entries. Not a current problem since generated recipes don't have satisfies, but breaks the invariant that `ToTOML()` is a complete serializer for `Recipe`.

Out of scope for this issue if `ToTOML` is only used by `tsuku create`, which won't set satisfies. Flagging it so it doesn't get forgotten.

## Finding 4: `buildSatisfiesIndex` re-parses every embedded recipe (Advisory)

**File**: `internal/recipe/loader.go:290-316`

The index build calls `toml.Unmarshal` on every embedded recipe. The main `Get` path also unmarshals when loading. This means any embedded recipe used via satisfies gets parsed twice (once for the index, once to return the recipe). For tens of recipes this is negligible. Mentioning it only because the design doc noted performance as an uncertainty -- the current implementation confirms it's fine for the embedded case.

## Summary

No over-engineering concerns. The implementation is tight: one struct field, one lazy index, one fallback in the loader, validation rules, tests. The `LookupSatisfies` public method is the only speculative addition and it's justified by the next issue in the milestone.

The `ToTOML()` gap and the recursive call without a cycle guard are the two actionable items. The recursion issue is blocking because a bad data state (which validation doesn't yet prevent) causes a stack overflow at runtime.
