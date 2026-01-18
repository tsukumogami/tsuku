# Issue 989 Implementation Plan

## Summary

Implement `ValidateDependencies()` as the central orchestrator for recursive dependency validation, integrating ValidateABI, ExtractRpaths/ExpandPathVariables, BuildSonameIndex/ClassifyDependency, and IsExternallyManagedFor from the prerequisite issues.

## Approach

The implementation follows the validation flow from DESIGN-library-verify-deps.md, creating a single `ValidateDependencies()` function that:
1. Performs cycle detection using a visited set with depth limiting (reusing `actions.MaxTransitiveDepth`)
2. Validates ABI compatibility via existing `ValidateABI()`
3. Extracts dependencies from binary headers via `ValidateHeader()`
4. Classifies each dependency and validates based on category
5. Recursively validates tsuku-managed dependencies (skipping externally-managed and system)

### Alternatives Considered

- **Separate validation functions per category**: Would require three functions (system, tsuku-managed, externally-managed) with duplicated orchestration logic. Rejected for code duplication.
- **Single-pass without recursion**: Would miss transitive dependency issues. Rejected per design requirement for recursive validation.
- **New MaxTransitiveDepth constant in verify package**: Adds duplicate constant. Rejected in favor of reusing `actions.MaxTransitiveDepth`.

## Files to Modify

- `internal/verify/types.go` - Add `DepResult` struct, `ValidationStatus` type, and `ErrMaxDepthExceeded` error category

## Files to Create

- `internal/verify/deps.go` - Main `ValidateDependencies()` function and helpers
- `internal/verify/deps_test.go` - Unit tests

## Implementation Steps

### Step 1: Add types to types.go

- [ ] Add `ErrMaxDepthExceeded` error category (value 16, continuing sequence)
- [ ] Add `ValidationStatus` type (Pass, Fail, Skip)
- [ ] Add `DepResult` struct with fields: Soname, Category, Recipe, Version, Status, Error, ResolvedPath, Transitive

### Step 2: Create deps.go with RecipeLoader interface

- [ ] Define `RecipeLoader` interface for recipe lookup (decouples from registry package)
- [ ] Add constant for allowed prefix (tools directory under $TSUKU_HOME)

### Step 3: Implement ValidateDependencies core function

- [ ] Implement path normalization with `filepath.Abs()` and `filepath.EvalSymlinks()`
- [ ] Add visited set check for cycle detection
- [ ] Add depth check using `actions.MaxTransitiveDepth`
- [ ] Call `ValidateABI()` for PT_INTERP validation
- [ ] Call `ValidateHeader()` to extract dependencies
- [ ] Handle static binaries (empty Dependencies = no validation needed)

### Step 4: Implement per-dependency validation

- [ ] Build `SonameIndex` from state
- [ ] Call `ExtractRpaths()` to get RPATH entries
- [ ] For each dependency in `HeaderInfo.Dependencies`:
  - Expand path variables via `ExpandPathVariables()`
  - Classify via `ClassifyDependency()`
  - Refine TSUKU_MANAGED vs EXTERNALLY_MANAGED via recipe lookup
- [ ] Validate based on category:
  - PURE_SYSTEM: verify file exists (skip on macOS for dyld cache)
  - TSUKU_MANAGED/EXTERNALLY_MANAGED: verify soname in state's Sonames list

### Step 5: Implement recursion logic

- [ ] For TSUKU_MANAGED dependencies (not EXTERNALLY_MANAGED):
  - Resolve library path from state
  - Recursively call `ValidateDependencies()`
  - Attach transitive results to parent DepResult
- [ ] Skip recursion for EXTERNALLY_MANAGED and PURE_SYSTEM

### Step 6: Implement helper functions

- [ ] `validateSystemDep(dep string, targetOS string) error` - file existence check with macOS dyld cache handling
- [ ] `validateTsukuDep(soname, recipe, version string, state *install.State) error` - check soname in state.Libs[recipe][version].Sonames
- [ ] `resolveLibraryPath(recipe, version string, state *install.State, tsukuHome string) string` - build path to library file for recursion
- [ ] `isExternallyManaged(recipe string, loader RecipeLoader, targetOS, targetArch string) bool` - wrapper for recipe.IsExternallyManagedFor()

### Step 7: Write unit tests

- [ ] `TestValidateDependencies_StaticBinary` - static binaries return empty results
- [ ] `TestValidateDependencies_SystemLibsOnly` - binary with only system libs passes
- [ ] `TestValidateDependencies_TsukuManagedFound` - binary with tsuku deps passes when present
- [ ] `TestValidateDependencies_TsukuManagedMissing` - binary fails when tsuku dep missing from state
- [ ] `TestValidateDependencies_UnknownDep` - binary fails when dep is neither system nor tsuku
- [ ] `TestValidateDependencies_CycleDetection` - cycle is detected and handled (returns nil, no error)
- [ ] `TestValidateDependencies_DepthLimit` - exceeding MaxTransitiveDepth returns error
- [ ] `TestValidateDependencies_ExternallyManagedNoRecurse` - externally-managed deps validated but not recursed
- [ ] `TestValidateDependencies_RecursiveValidation` - transitive deps validated when recurse=true

### Step 8: Verify build and tests

- [ ] Run `go vet ./...`
- [ ] Run `go test ./...`
- [ ] Run `golangci-lint run --timeout=5m ./...`

## Testing Strategy

### Unit Tests

Use test fixtures with mock binaries or testdata files:
- Mock `RecipeLoader` interface for recipe lookup
- Create minimal `install.State` with known sonames
- Test each DepCategory path independently

### Edge Cases

- Empty Dependencies slice (static binary)
- Nil state or empty Libs map
- Missing recipe in loader
- Path with unexpanded variables (should fail in ExpandPathVariables)
- macOS dyld cache libraries (pattern-matched, no file check)

### Manual Verification

After implementation, test with real installed libraries:
```bash
go build -o tsuku ./cmd/tsuku
./tsuku install openssl
./tsuku verify openssl
```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Import cycle between verify and actions packages | Use `actions.MaxTransitiveDepth` directly; it's a constant, not a function |
| RecipeLoader interface mismatch | Define minimal interface in verify package; adapter in cmd/tsuku/verify.go |
| macOS dyld cache false negatives | Trust system patterns; don't require file existence on macOS for pattern-matched libs |
| Performance with deep dependency trees | Depth limit (10) prevents excessive recursion; visited set prevents redundant work |

## Success Criteria

- [ ] `ValidateDependencies()` function implemented with signature matching design
- [ ] Integrates with `ValidateABI()` from #981
- [ ] Integrates with `ExpandPathVariables()` from #982
- [ ] Integrates with `BuildSonameIndex()` and `ClassifyDependency()` from #986
- [ ] Integrates with `IsExternallyManagedFor()` from #984 via RecipeLoader interface
- [ ] Cycle detection prevents infinite loops
- [ ] Depth limiting uses `actions.MaxTransitiveDepth = 10`
- [ ] Recursion skips externally-managed and system libraries
- [ ] Static binaries handled correctly (empty results, no error)
- [ ] All unit tests pass
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run --timeout=5m ./...` passes

## Open Questions

None. All integration points are clear from prerequisite issues and IMPLEMENTATION_CONTEXT.md.
