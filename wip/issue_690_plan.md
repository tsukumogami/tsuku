# Issue 690 Implementation Plan

## Summary

Implement platform tuple support for step-level `when` clauses by replacing the `map[string]string` representation with a structured `WhenClause` type containing `Platform []string` and `OS []string` fields, following the design in `docs/DESIGN-when-clause-platform-tuples.md`.

## Approach

Following **Option 2: Structured WhenClause Type** from the design document because:
- No backwards compatibility needed (only 2 recipes to migrate in same PR)
- Clean type-safe implementation without CSV storage hacks
- Consistent with install_guide's platform tuple support
- Future-extensible for additional when clause features

The implementation will support:
- Platform tuples: `when = { platform = ["darwin/arm64", "linux/amd64"] }`
- OS arrays: `when = { os = ["darwin", "linux"] }`
- Package manager check: `when = { package_manager = "brew" }`
- Additive semantics: All matching steps execute (not exclusive)
- Mutual exclusivity: `platform` and `os` cannot coexist in same clause

### Alternatives Considered

- **Option 1: CSV Storage in map[string]string**: Rejected - inelegant hack, parsing complexity, backwards compat not needed
- **Option 3: Separate when_platform Field**: Rejected - inconsistent pattern, confusing interaction rules

## Files to Modify

- `internal/recipe/types.go` - Define `WhenClause` struct, update `Step.When` field, modify `UnmarshalTOML()`
- `internal/executor/plan_generator.go` - Update `shouldExecuteForPlatform()` to use `WhenClause.Matches()`
- `internal/recipe/platform.go` - Extend `ValidateStepsAgainstPlatforms()` to validate when clause platform tuples
- `internal/recipe/recipes/g/gcc-libs.toml` - Migrate from `when.os` to `when = { os = ["linux"] }`
- `internal/recipe/recipes/n/nodejs.toml` - Migrate from `when.os` to `when = { os = ["linux"] }`

## Files to Create

- `internal/recipe/when_test.go` - Unit tests for WhenClause.Matches() and validation
- `docs/when-clause-usage.md` - User documentation for when clause syntax

## Implementation Steps

### Phase 1: Define WhenClause Struct

- [ ] Add `WhenClause` struct in `internal/recipe/types.go` with fields:
  - `Platform []string` - Platform tuples like ["darwin/arm64"]
  - `OS []string` - OS values like ["darwin", "linux"]
  - `PackageManager string` - Runtime package manager check
- [ ] Add `IsEmpty() bool` method - returns true if all fields are zero values
- [ ] Add `Matches(os, arch string) bool` method implementing:
  - Return true if IsEmpty() (no conditions = match all)
  - Check Platform array first (exact tuple match)
  - Fall back to OS array (any arch)
  - Return true if no platform/OS conditions

### Phase 2: Update TOML Unmarshaling

- [ ] Change `Step.When` field from `map[string]string` to `*WhenClause` in types.go
- [ ] Update `Step.UnmarshalTOML()` to parse when clause:
  - Extract `when` table from stepMap
  - Parse `platform` as []string
  - Parse `os` as []string
  - Parse `package_manager` as string
  - Validate mutual exclusivity (platform XOR os, not both)
  - Create WhenClause instance
- [ ] Update `Step.ToMap()` method to serialize WhenClause back to map format

### Phase 3: Update Runtime Filtering

- [ ] Modify `shouldExecuteForPlatform()` in plan_generator.go:
  - Change signature to accept `*WhenClause` instead of `map[string]string`
  - Call `when.Matches(targetOS, targetArch)` for filtering
  - Handle nil when clause (execute on all platforms)
- [ ] Update all call sites of `shouldExecuteForPlatform()`
- [ ] Verify plan generation tests still pass

### Phase 4: Add Validation

- [ ] Extend `ValidateStepsAgainstPlatforms()` in platform.go:
  - For each step's WhenClause:
    - Validate platform tuple format (`os/arch` with slash)
    - Check platform tuples exist in `Recipe.GetSupportedPlatforms()`
    - Check OS values exist in supported OS set
    - Return StepValidationError for invalid tuples
- [ ] Add test cases for validation errors

### Phase 5: Migrate Existing Recipes

- [ ] Update `gcc-libs.toml`:
  - Change `[steps.when]\nos = "linux"` to `when = { os = ["linux"] }`
  - Apply to all 3 steps with when clauses
- [ ] Update `nodejs.toml`:
  - Change `[steps.when]\nos = "linux"` to `when = { os = ["linux"] }`
  - Apply to 2 steps with when clauses
- [ ] Verify both recipes load without errors
- [ ] Test installation on linux and darwin platforms

### Phase 6: Add Tests

- [ ] Create `when_test.go` with tests for:
  - `WhenClause.IsEmpty()` - nil, zero values, populated
  - `WhenClause.Matches()` - platform tuples, OS arrays, empty clause
  - TOML unmarshaling - platform arrays, OS arrays, mutual exclusivity
  - Validation - invalid tuples, unsupported platforms
- [ ] Add integration tests in `plan_generator_test.go`:
  - Step filtering with platform tuples
  - Step filtering with OS arrays
  - Additive semantics (multiple matching steps execute)

### Phase 7: Add Documentation

- [ ] Create `docs/when-clause-usage.md` with:
  - Platform tuple syntax and examples
  - OS array syntax and examples
  - Package manager filtering
  - Mutual exclusivity rules
  - Additive matching semantics
  - Migration examples from old syntax
- [ ] Update `docs/DESIGN-when-clause-platform-tuples.md` status to "Accepted"
- [ ] Reference when clause doc in main recipe documentation

## Testing Strategy

### Unit Tests
- `WhenClause.IsEmpty()` - all combinations of empty/non-empty fields
- `WhenClause.Matches()`:
  - Platform exact match (darwin/arm64 matches ["darwin/arm64"])
  - Platform no match (linux/amd64 doesn't match ["darwin/arm64"])
  - OS match any arch (darwin/arm64 matches os=["darwin"])
  - Empty clause matches all platforms
  - Mutual exclusivity validation

### Integration Tests
- Load gcc-libs.toml and nodejs.toml successfully
- Generate plan for each recipe on different platforms
- Verify correct steps are included based on when clauses
- Test additive semantics with multiple matching steps

### Manual Verification
- Install gcc-libs on linux (should succeed)
- Install gcc-libs on darwin (should succeed with steps skipped)
- Install nodejs on both platforms
- Verify nodejs wrapper script only created on linux

## Risks and Mitigations

- **Risk**: Breaking change to Step.When field type
  - **Mitigation**: Only 2 recipes use when clauses, migrating them in same PR

- **Risk**: TOML unmarshaling errors with new array syntax
  - **Mitigation**: Comprehensive unit tests for unmarshaling, test both recipes

- **Risk**: Validation too strict, blocks valid use cases
  - **Mitigation**: Follow validation patterns from install_guide (proved in #689)

- **Risk**: Additive semantics confusing to users
  - **Mitigation**: Clear documentation with examples, matches ecosystem precedents (Cargo, Homebrew)

## Success Criteria

- [ ] All existing tests pass
- [ ] gcc-libs.toml and nodejs.toml load without errors
- [ ] New unit tests for WhenClause have 100% coverage
- [ ] Documentation clearly explains syntax and semantics
- [ ] No breaking changes to recipes not using when clauses
- [ ] Can express "darwin/arm64 OR linux/amd64" without step duplication
- [ ] Build succeeds with no warnings
- [ ] CI passes all checks

## Open Questions

None - all design decisions were resolved during exploration phase with user feedback.
