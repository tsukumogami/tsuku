# Issue 822 Implementation Plan

## Summary

Add `Constraint` and `StepAnalysis` types plus `detectInterpolatedVars` function to `internal/recipe/types.go`, following existing patterns for nil-safe methods and table-driven tests.

## Approach

The implementation adds new types to the existing `types.go` file rather than creating a separate file. This maintains consistency with the codebase structure where related recipe types are grouped together. The types are designed to be nil-safe following Go idioms already used in `WhenClause` (e.g., `IsEmpty()` handles nil receivers).

Key design decisions:
- `Constraint` uses pointer semantics (`*Constraint`) so nil represents "unconstrained"
- `Clone()` returns `&Constraint{}` for nil receivers (idiomatic nil-safe pattern)
- `detectInterpolatedVars` uses regexp for `{{varname}}` detection with a fixed `knownVars` list
- Tests follow the existing table-driven pattern in `types_test.go`

### Alternatives Considered

1. **Separate file for constraint types (`constraint.go`)**: Rejected because the codebase groups related recipe types in `types.go`. Creating a separate file would fragment the type system and require more import management. The existing `types.go` already contains `WhenClause` which is conceptually similar.

2. **Generic interpolation detection (detect all `{{...}}` patterns)**: Rejected in favor of explicit `knownVars` list. The issue specification explicitly states "does not detect unknown variables (only those in `knownVars`)". This design allows future extension by adding to the list without API changes.

3. **Using `Constraint` as value type instead of pointer**: Rejected because nil pointer semantics clearly express "unconstrained" which is the most common case. Value semantics would require a separate boolean or sentinel values to express "no constraint".

## Files to Modify

- `internal/recipe/types.go` - Add `Constraint`, `StepAnalysis` types, `knownVars`, and `detectInterpolatedVars` function

## Files to Create

None - all changes go into existing files.

## Implementation Steps

- [x] Add `Constraint` struct with OS, Arch, LinuxFamily fields to `types.go`
- [x] Add `Clone()` method with nil-safe behavior
- [x] Add `Validate()` method checking LinuxFamily/OS compatibility
- [x] Add `StepAnalysis` struct with Constraint pointer and FamilyVarying bool
- [x] Add `knownVars` package-level variable with platform interpolation variables
- [x] Add `detectInterpolatedVars()` function with recursive scanning
- [x] Add unit tests for `Constraint.Clone()` (nil receiver, field copying)
- [x] Add unit tests for `Constraint.Validate()` (nil receiver, empty, valid combinations, invalid darwin+family)
- [x] Add unit tests for `detectInterpolatedVars()` (nil, no vars, single var, multiple vars, nested map, slice, unknown var)
- [x] Run `go build ./internal/recipe/...` to verify compilation
- [x] Run `go test ./internal/recipe/...` to verify tests pass
- [x] Run `go vet ./internal/recipe/...` to verify no issues

## Testing Strategy

### Unit Tests

All tests will be added to `internal/recipe/types_test.go` following the existing table-driven test pattern:

**Constraint.Clone tests:**
- `TestConstraint_Clone_NilReceiver`: Verify nil receiver returns `&Constraint{}` (not nil)
- `TestConstraint_Clone_CopiesFields`: Verify all three fields are copied to a new instance

**Constraint.Validate tests:**
- `TestConstraint_Validate_NilReceiver`: Returns nil
- `TestConstraint_Validate_EmptyConstraint`: Returns nil for `&Constraint{}`
- `TestConstraint_Validate_LinuxFamilyWithLinuxOS`: Returns nil for `{OS: "linux", LinuxFamily: "debian"}`
- `TestConstraint_Validate_LinuxFamilyWithEmptyOS`: Returns nil for `{LinuxFamily: "debian"}` (family implies linux)
- `TestConstraint_Validate_LinuxFamilyWithDarwinOS`: Returns error for `{OS: "darwin", LinuxFamily: "debian"}`

**detectInterpolatedVars tests:**
- `TestDetectInterpolatedVars_NilInput`: Returns empty map for `nil`
- `TestDetectInterpolatedVars_StringWithoutVars`: Returns empty map for `"no interpolation"`
- `TestDetectInterpolatedVars_StringWithLinuxFamily`: Returns `{"linux_family": true}` for `"pkg-{{linux_family}}"`
- `TestDetectInterpolatedVars_StringWithMultipleVars`: Returns map with all found vars for `"{{os}}-{{arch}}"`
- `TestDetectInterpolatedVars_NestedMap`: Recursively finds vars in `map[string]interface{}{"key": "{{linux_family}}"}`
- `TestDetectInterpolatedVars_Slice`: Recursively finds vars in `[]interface{}{"{{os}}"}`
- `TestDetectInterpolatedVars_UnknownVar`: Does not detect `{{unknown}}` (only known vars)

### Integration Tests

Not applicable for this issue - downstream issues #823 and #824 will provide integration testing when using these types.

### Manual Verification

Run the validation script from the issue:
```bash
go build ./internal/recipe/...
go test -v -run "TestConstraint|TestStepAnalysis|TestDetectInterpolatedVars" ./internal/recipe/...
go test ./...
```

## Risks and Mitigations

- **Risk**: Regex for `{{varname}}` detection may have edge cases with special characters
  - **Mitigation**: Use simple regex `\{\{(linux_family|os|arch)\}\}` that matches exact variable names. Test with edge cases like `{{linux_family}}` embedded in larger strings.

- **Risk**: Recursive `detectInterpolatedVars` could stack overflow on deeply nested structures
  - **Mitigation**: In practice, recipe step parameters are shallow (2-3 levels max). The function follows the same recursive pattern used elsewhere in the codebase for step parameter handling.

- **Risk**: Future `knownVars` additions could break existing behavior
  - **Mitigation**: The `knownVars` slice is append-only. Adding new variables will expand detection, not break existing code.

## Success Criteria

- [ ] `Constraint` type compiles with Clone() and Validate() methods
- [ ] `StepAnalysis` type compiles with Constraint pointer and FamilyVarying bool
- [ ] `detectInterpolatedVars` function compiles and handles all input types
- [ ] All 12 test cases pass (Clone: 2, Validate: 5, detectInterpolatedVars: 7)
- [ ] `go build ./internal/recipe/...` succeeds
- [ ] `go test ./internal/recipe/...` succeeds (all existing + new tests pass)
- [ ] `go vet ./internal/recipe/...` reports no issues

## Open Questions

None - the issue specification is complete and self-contained.
