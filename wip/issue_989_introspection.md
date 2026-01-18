# Issue 989 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-library-verify-deps.md`
- Sibling issues reviewed: #978, #979, #980, #981, #982, #983, #984, #985, #986 (all closed)
- Prior patterns identified:
  - Error categories use explicit int values (10, 11, 12...) per design decision #2
  - Panic recovery pattern consistent across all verify functions
  - `ValidationError` struct is the standard error type for verify package
  - Function naming: `Validate*` for validators, `Extract*` for extractors
  - `SonameIndex` uses `*install.State` as input
  - `ClassifyDependency` returns `(DepCategory, recipe, version)`
  - `IsExternallyManagedFor` requires `Matchable` target and action lookup function

## Gap Analysis

### Minor Gaps

1. **Dependency extraction already exists in header.go**: The `HeaderInfo.Dependencies` field and `ImportedLibraries()` call in `validateELFPath()`/`validateMachOPath()` already extract dependencies. Issue #989's proposed `ExtractDependencies()` should either:
   - Reuse the existing header validation logic
   - Or be a thin wrapper that calls `ValidateHeader()` and returns `info.Dependencies`
   - Pattern established: header.go handles extraction, #989 handles classification/recursion

2. **RecipeLoader interface undefined in issue**: The issue proposes `RecipeLoader` as a parameter type but this interface doesn't exist. The actual implementation from #984 shows `IsExternallyManagedFor()` takes:
   - `target Matchable` (not Target)
   - `actionLookup func(string) interface{}` (not a loader interface)
   - The function exists on `*Recipe`, not as a standalone function

3. **MaxTransitiveDepth location**: Issue mentions using "existing MaxTransitiveDepth=10" but doesn't specify where it's defined. This constant needs to be either:
   - Added to `internal/verify/types.go` (new)
   - Or reused from an existing location if present

4. **Static binary detection**: Issue proposes `isStaticallyLinked()` checking PT_INTERP absence, but `ValidateABI()` already handles this - returns nil for no PT_INTERP. The #989 implementation should use ValidateABI result to detect static binaries rather than duplicating logic.

5. **DepCategory vs issue's DepCategory**: The issue proposes `DepCategoryUnknown`, `DepCategoryPureSystem`, etc. but #986 already implemented these as `DepUnknown`, `DepPureSystem`, `DepTsukuManaged`, `DepExternallyManaged` in `classify.go`. The implementation should use existing constants.

### Moderate Gaps

None identified. All gaps are resolvable from prior work without user input.

### Major Gaps

None identified. The core integration points are stable and well-defined.

## Recommendation

**Proceed**

All prerequisites are complete and merged (#981, #982, #984, #986). The gaps identified are minor naming/signature discrepancies between the issue spec and actual implementations. These can be incorporated during implementation without changing the issue's scope or intent.

## Implementation Notes

Based on prior work, the actual function signature should be:

```go
// ValidateDependencies performs recursive dependency validation for a binary.
func ValidateDependencies(
    binaryPath string,
    state *install.State,
    index *SonameIndex,
    registry *SystemLibraryRegistry,
    recipe *recipe.Recipe,          // For IsExternallyManagedFor check
    target recipe.Matchable,        // Platform target
    actionLookup func(string) interface{},  // For action lookup
    visited map[string]bool,
    recurse bool,
    targetOS string,
) ([]DepResult, error)
```

Key integration points from prior issues:
- `ValidateABI(path)` from #981 - returns nil for static binaries
- `ExtractRpaths(path)` and `ExpandPathVariables(dep, binaryPath, rpaths, allowedPrefix)` from #982
- `BuildSonameIndex(state)` from #986
- `ClassifyDependency(dep, index, registry, targetOS)` from #986 - note: returns `(DepTsukuManaged, recipe, version)` initially; caller refines to `DepExternallyManaged` via recipe check
- `recipe.IsExternallyManagedFor(target, actionLookup)` from #984
- `HeaderInfo.Dependencies` from header.go for extraction
