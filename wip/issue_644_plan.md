# Issue 644 Implementation Plan

## Summary

Implement automatic dependency aggregation from primitive actions when resolving dependencies for composite actions, add validation to detect shadowed dependencies in strict mode, and audit existing recipes to remove redundant declarations.

## Approach

The core fix addresses the problem where composite actions (like `homebrew`) must currently declare dependencies that are already present in their primitive actions (like `homebrew_relocate`). The solution adds dependency aggregation during the resolution phase by decomposing composite actions and collecting dependencies from all primitives in the decomposition tree.

This approach was chosen because:
1. **Minimal invasiveness**: Modifies only the resolution logic, not the action interfaces
2. **Backward compatibility**: Existing explicit declarations continue to work (though shadowed deps will warn in strict mode)
3. **Natural fit**: Decomposition already exists for plan generation; we reuse it for dependency resolution
4. **Performance**: Decomposition happens once during resolution, not repeatedly

### Alternatives Considered

- **Option 1: Modify action interface to auto-aggregate**: Each composite action's `Dependencies()` method would decompose itself and aggregate. Rejected because it couples dependency resolution to decomposition logic, making actions more complex and potentially creating circular dependencies during initialization.

- **Option 2: Post-decomposition aggregation**: Aggregate deps after plan generation completes. Rejected because dependency resolution must happen before plan generation to ensure all required tools are available during eval-time decomposition.

- **Option 3: Static dependency declaration in registry**: Manually maintain a mapping of composite→primitive dependencies. Rejected because it duplicates information and creates maintenance burden when primitive dependencies change.

## Files to Modify

- `internal/actions/resolver.go` - Add `aggregatePrimitiveDeps()` function to collect dependencies from decomposed primitives; modify `ResolveDependencies()` to call it after collecting action dependencies
- `internal/actions/dependencies.go` - Add `DetectShadowedDeps()` function to find declared dependencies that are already inherited from primitives
- `internal/actions/homebrew.go` - Remove `Dependencies()` method (lines 35-39) since patchelf will be automatically inherited from homebrew_relocate
- `internal/recipe/validator.go` - Add `validateShadowedDependencies()` function; integrate it into `ValidateBytes()` to check for shadowed deps and add warnings (errors in strict mode)
- `cmd/tsuku/validate.go` - Update to support strict mode for shadowed dependency validation (already has `--strict` flag, just need to ensure it propagates to new validation)

## Files to Create

- `internal/actions/aggregate_test.go` - Unit tests for primitive dependency aggregation, covering composite actions, nested composites, and cycle detection
- `internal/actions/shadowed_deps_test.go` - Unit tests for shadowed dependency detection, covering various scenarios (exact match, transitive inheritance, no shadowing)
- `internal/recipe/validator_shadowed_test.go` - Tests for shadowed dependency validation in recipes

## Implementation Steps

- [x] Implement core aggregation logic in `resolver.go`
  - Add `aggregatePrimitiveDeps()` that decomposes an action and recursively collects all primitive dependencies
  - Handle actions that aren't decomposable (return empty deps)
  - Detect and prevent infinite recursion/cycles during aggregation
  - Merge aggregated deps with explicitly declared deps (explicit takes precedence for version constraints)

- [x] Integrate aggregation into `ResolveDependencies()`
  - After calling `GetActionDeps(step.Action)`, check if action is decomposable
  - If decomposable, call `aggregatePrimitiveDeps()` and merge results
  - Ensure self-dependency filtering still works (skip if dep == recipe name)
  - Preserve existing step-level override semantics (dependencies param replaces all)

- [x] Implement shadowed dependency detection in `dependencies.go`
  - Add `DetectShadowedDeps(recipe, aggregatedDeps)` that compares declared deps with inherited deps
  - Return list of shadowed dependency names with source information (which primitive provides it)
  - Handle both install-time and runtime dependencies

- [x] Add shadowed dependency validation to recipe validator
  - Integrated shadowed dependency checking into `cmd/tsuku/validate.go` (CLI layer)
  - Avoids circular dependency between recipe and actions packages
  - Calls `actions.DetectShadowedDeps()` after recipe validation
  - Adds warnings for shadowed dependencies (become errors in strict mode via existing `--strict` flag)
  - Helpful message: "dependency 'X' is already inherited from action 'Y' (remove this redundant declaration)"

- [ ] Update `homebrew.go` to remove redundant dependency declaration
  - Remove `Dependencies()` method from `HomebrewAction`
  - Remove TODO comments that reference this issue (#644)
  - Verify patchelf is still resolved via aggregation from `homebrew_relocate`

- [ ] Update `homebrew_relocate.go` to remove TODO comments
  - Remove TODO(#644) comment (line 21) since aggregation now works
  - Keep the `Dependencies()` method as it's the source of truth for patchelf

- [ ] Write comprehensive unit tests for aggregation
  - Test simple composite (homebrew → homebrew_relocate → patchelf)
  - Test composite with no primitive deps (e.g., github_archive → download_file)
  - Test nested composites (if any exist)
  - Test action that's not decomposable (returns empty)
  - Test self-dependency filtering during aggregation

- [ ] Write unit tests for shadowed dependency detection
  - Test exact match: declared dep equals inherited dep
  - Test no shadowing: declared dep not in inherited set
  - Test version constraint shadowing: declared "foo@1.2.3" vs inherited "foo@latest"
  - Test runtime vs install-time separation

- [ ] Write integration tests for validation
  - Test recipe with shadowed dependency produces warning
  - Test strict mode converts warning to error
  - Test recipe with no shadowing passes validation
  - Test recipe with valid extra dependency (not shadowed) passes

- [ ] Audit existing recipes and remove redundant dependencies
  - Search testdata/recipes for patterns like `dependencies = ["patchelf"]` in recipes using homebrew action
  - Remove redundant declarations
  - Verify tests still pass with aggregated dependencies
  - Document changes in commit message

## Testing Strategy

### Unit Tests

- **Aggregation logic** (`aggregate_test.go`):
  - Test `aggregatePrimitiveDeps()` with various action types (composite, primitive, non-existent)
  - Test cycle detection prevents infinite recursion
  - Test dependency merging (aggregated + explicit)
  - Test self-dependency filtering works during aggregation

- **Shadowed detection** (`shadowed_deps_test.go`):
  - Test `DetectShadowedDeps()` identifies exact matches
  - Test version constraint differences (shadowed if name matches, regardless of version)
  - Test empty/nil inputs return empty results
  - Test install-time vs runtime dependency separation

- **Validator integration** (`validator_shadowed_test.go`):
  - Test validator adds warnings for shadowed deps
  - Test strict mode treats shadowed deps as errors
  - Test helpful error messages include action source

### Integration Tests

- **Resolver integration**:
  - Test `ResolveDependencies()` with homebrew action returns patchelf without explicit declaration
  - Test existing recipes with explicit deps still work (backward compatibility)
  - Test step-level overrides still work with aggregation

- **End-to-end validation**:
  - Run `tsuku validate` on test recipes with shadowed deps
  - Verify warnings appear in output
  - Verify `--strict` flag fails validation
  - Create new test recipe without shadowed deps, verify clean validation

### Manual Verification

- Build tsuku with changes
- Run resolver on homebrew recipe, verify patchelf is in dependencies
- Run validator on old homebrew recipe (with explicit patchelf), verify shadowed warning
- Run validator with `--strict`, verify it fails
- Remove explicit patchelf declaration, verify clean validation

## Risks and Mitigations

- **Risk**: Decomposition during resolution might be expensive for complex recipes
  - **Mitigation**: Decomposition is already used in plan generation; if performance is an issue there, it would be visible already. Aggregation is a one-time operation per recipe. Add caching if needed.

- **Risk**: Circular dependencies in decomposition could cause infinite loops
  - **Mitigation**: Reuse existing cycle detection from `DecomposeToPrimitives()`. Add tests specifically for this scenario.

- **Risk**: Breaking changes if recipes rely on NOT having aggregated dependencies
  - **Mitigation**: Aggregation only adds dependencies; it doesn't remove them. Worst case: an extra dependency is installed. Validate against test suite before merging.

- **Risk**: Version constraint conflicts (explicit says "foo@1.0", aggregate says "foo@2.0")
  - **Mitigation**: Give precedence to explicit declarations. If recipe explicitly declares a version, use it; otherwise use aggregated version.

- **Risk**: Eval-time dependencies needed for decomposition might not be available
  - **Mitigation**: Eval-time deps are already handled separately (see `GetEvalDeps()` in `eval_deps.go`). Aggregation only affects install-time and runtime deps, which are resolved after eval completes.

- **Risk**: Shadowed dependency validation might produce false positives
  - **Mitigation**: Only flag exact name matches. Different versions of same dependency are still shadowed (same name). Make warnings descriptive so users understand why something is flagged.

## Success Criteria

- [ ] `ResolveDependencies()` returns patchelf for recipes using homebrew action, even without explicit declaration
- [ ] `HomebrewAction.Dependencies()` can be removed without breaking existing tests
- [ ] Validator detects shadowed dependencies and produces warnings
- [ ] Validator with `--strict` flag treats shadowed dependencies as errors
- [ ] All existing tests pass with aggregation enabled
- [ ] New unit tests cover aggregation logic, shadowed detection, and validation
- [ ] Test recipes with redundant dependency declarations cleaned up
- [ ] No performance regression in dependency resolution (measure with benchmarks if needed)

## Open Questions

None - the approach is clear and all requirements are specified. The implementation can proceed directly.
