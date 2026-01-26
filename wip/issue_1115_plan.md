# Issue 1115 Implementation Plan

## Summary

Create a coverage validation system in `internal/recipe/coverage.go` that statically analyzes recipes for glibc/musl/darwin support, generating errors for libraries missing musl coverage and warnings for tools with library dependencies missing musl coverage. The validation will be exposed via a `--check-libc-coverage` flag on the existing `validate` command.

## Approach

The implementation follows the design document's Layer 1 (Recipe Validation) approach: static analysis of recipe step `when` clauses to determine platform coverage without executing any installation. This provides fast CI feedback and catches musl compatibility issues at PR time.

The coverage analyzer examines each step's `when` clause to determine which platforms it supports (glibc, musl, darwin). Unconditional steps (no `when` clause) count for all platforms. The analyzer then generates errors/warnings based on recipe type and whether explicit `supported_libc` constraints exist.

### Alternatives Considered

1. **Runtime-based validation (simulate plan generation for each target)**: Would require loading all recipes and generating plans for multiple targets. More accurate but slower and more complex. Rejected because static analysis is sufficient for catching missing coverage and is faster for CI.

2. **Validation at loader level**: Adding coverage validation during recipe loading. Rejected because it would slow down all recipe loads, and coverage validation is only needed during explicit validation commands or CI.

## Files to Modify

- `cmd/tsuku/validate.go` - Add `--check-libc-coverage` flag and call coverage validation
- `internal/recipe/validator.go` - Add coverage validation integration point

## Files to Create

- `internal/recipe/coverage.go` - CoverageReport struct and AnalyzeRecipeCoverage function
- `internal/recipe/coverage_test.go` - Unit tests for all validation scenarios

## Implementation Steps

- [ ] Create `internal/recipe/coverage.go` with CoverageReport struct containing fields: Recipe, HasGlibc, HasMusl, HasDarwin, SupportedLibc, Warnings, Errors
- [ ] Implement helper functions to check if a WhenClause matches glibc/musl/darwin targets
- [ ] Implement AnalyzeRecipeCoverage() function that analyzes all steps for platform coverage
- [ ] Add detection of explicit supported_libc constraints in recipe metadata
- [ ] Generate errors for library recipes missing musl (no path and no explicit constraint)
- [ ] Generate warnings for tool recipes with library dependencies missing musl
- [ ] Add helper function to check if recipe has library dependencies
- [ ] Add `--check-libc-coverage` flag to validate command in cmd/tsuku/validate.go
- [ ] Implement ValidateCoverageForRecipe function that integrates with ValidationResult
- [ ] Add TestTransitiveDepsHaveMuslSupport test that walks dependency tree
- [ ] Create comprehensive unit tests for all coverage validation scenarios
- [ ] Verify existing tests pass with `go test ./...`
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` before finalizing

## Testing Strategy

### Unit Tests

Test coverage for `internal/recipe/coverage_test.go`:

1. **Unconditional steps** - Steps without `when` clause count for all platforms
2. **glibc-only steps** - `when = { os = ["linux"], libc = ["glibc"] }` only counts for glibc
3. **musl-only steps** - `when = { os = ["linux"], libc = ["musl"] }` only counts for musl
4. **darwin-only steps** - `when = { os = ["darwin"] }` only counts for darwin
5. **Library recipe missing musl** - Type "library" without musl path generates ERROR
6. **Library recipe with explicit constraint** - `supported_libc = ["glibc"]` generates PASS
7. **Tool recipe with library deps missing musl** - Generates WARNING
8. **Tool recipe without deps** - No warning even if missing musl
9. **Recipe with all three paths** - No warnings or errors
10. **Transitive dependency check** - Validates full dependency tree has musl support

### Integration Testing

- Run validation against existing embedded recipes to ensure they pass
- Specifically test zlib.toml, openssl.toml, libyaml.toml, gcc-libs.toml (library recipes with musl support)

### Manual Verification

- Build tsuku and run `./tsuku validate --check-libc-coverage internal/recipe/recipes/zlib.toml`
- Verify output shows appropriate coverage information

## Risks and Mitigations

1. **Risk: False positives for tools that genuinely cannot support musl**
   - Mitigation: Explicit `supported_libc` constraint allows documented exceptions; warning (not error) for tools

2. **Risk: Performance impact when validating many recipes**
   - Mitigation: Static analysis is O(n) in number of steps; no network or execution required

3. **Risk: Complex when clause logic may miss edge cases**
   - Mitigation: Comprehensive unit tests for all when clause combinations; helper functions isolate matching logic

4. **Risk: Breaking existing CI workflows**
   - Mitigation: `--check-libc-coverage` is opt-in; default validation behavior unchanged

## Success Criteria

- [ ] CoverageReport struct exists with documented fields
- [ ] AnalyzeRecipeCoverage correctly identifies glibc/musl/darwin coverage from step when clauses
- [ ] Unconditional steps count for all platforms
- [ ] Library recipes without musl path and without explicit constraint generate errors
- [ ] Tool recipes with library deps but no musl path generate warnings
- [ ] Explicit supported_libc constraints allow opt-out (no error/warning)
- [ ] `--check-libc-coverage` flag added to validate command
- [ ] Clear error messages include fix suggestions
- [ ] All unit tests pass
- [ ] All existing tests pass
- [ ] Code passes go vet and golangci-lint

## Open Questions

None - the design document provides clear specifications for all validation behavior.
