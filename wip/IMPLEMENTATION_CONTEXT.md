---
summary:
  constraints:
    - Cross-platform: must work on Linux (ELF, glibc/musl) and macOS (Mach-O)
    - No false positives: system libraries must not trigger failures
    - No external tools: use Go's debug/elf and debug/macho packages only
    - Fail on unknown deps pre-GA to surface corner cases
    - MaxTransitiveDepth=10 for cycle prevention
  integration_points:
    - internal/verify/abi.go: ValidateABI() for PT_INTERP check
    - internal/verify/rpath.go: ExtractRpaths(), ExpandPathVariables() for path resolution
    - internal/verify/index.go: BuildSonameIndex() for soname->recipe lookup
    - internal/verify/classify.go: ClassifyDependency() for dep categorization
    - internal/recipe/types.go: IsExternallyManagedFor() for managed vs external check
    - internal/install/state.go: State, LibraryVersionState with Sonames field
  risks:
    - macOS dyld cache: system libraries may not exist as files since macOS 11
    - Path expansion complexity: $ORIGIN, @rpath, @loader_path need careful handling
    - Cycle detection: must track visited paths to prevent infinite recursion
    - Static binaries: need to detect and handle gracefully (no PT_INTERP = no deps)
  approach_notes: |
    Implement ValidateDependencies() as the central orchestrator that:
    1. Validates ABI via ValidateABI() - catches glibc/musl mismatch
    2. Extracts deps from HeaderInfo.Dependencies (already implemented in header.go)
    3. Expands path variables via ExpandPathVariables()
    4. Classifies each dep via ClassifyDependency()
    5. Validates based on category (system: file check, tsuku: soname match)
    6. Recurses into tsuku-managed deps (skip externally-managed and system)

    Key types: DepResult (validation result), DepCategory (from classify.go)
    Existing types can be reused - ClassifyResult from classify.go has what we need
---

# Implementation Context: Issue #989

## Key Integration Points

### From #981 - ABI Validation
- `ValidateABI(path string) error` - validates PT_INTERP interpreter exists

### From #982 - RPATH Expansion
- `ExtractRpaths(path string) ([]string, error)` - extracts RPATH entries
- `ExpandPathVariables(dep, binaryPath string, rpaths []string, allowedPrefix string) (string, error)` - expands $ORIGIN, @rpath, etc.

### From #986 - SonameIndex and Classification
- `BuildSonameIndex(state *install.State) *SonameIndex` - builds reverse lookup
- `ClassifyDependency(dep string, index *SonameIndex, registry *SystemLibraryRegistry, targetOS string) (DepCategory, recipe, version)`
- DepCategory: DepPureSystem, DepTsukuManaged, DepExternallyManaged, DepUnknown

### From #984 - IsExternallyManagedFor
- `Recipe.IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool`

### Header Info (existing)
- `HeaderInfo.Dependencies` - already extracts DT_NEEDED/LC_LOAD_DYLIB

## Validation Flow

```
ValidateDependencies(binaryPath, state, recipeLoader, visited, recurse)
  1. Check visited[binaryPath] -> skip if already validated
  2. Check len(visited) > MaxTransitiveDepth -> error if exceeded
  3. ValidateABI(binaryPath) -> error if ABI mismatch
  4. ExtractRpaths(binaryPath) -> get RPATH entries
  5. ValidateHeader(binaryPath) -> get HeaderInfo with Dependencies
  6. For each dep in HeaderInfo.Dependencies:
     a. ExpandPathVariables(dep, binaryPath, rpaths, allowedPrefix)
     b. ClassifyDependency(expanded, index, registry, targetOS)
     c. Validate based on category
     d. If TSUKU_MANAGED && !externally-managed && recurse: recurse
```
