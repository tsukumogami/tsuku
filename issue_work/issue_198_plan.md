# Issue 198 Implementation Plan

## Summary

Enhance the recipe validator to enforce verification mode rules and expand dangerous pattern detection. Update `validateVerify` function in `internal/recipe/validator.go`.

## Approach

Add mode-specific validation rules to the existing `validateVerify` function. The validator already has a pattern for warnings vs errors (e.g., `addWarning` for recommendations, `addError` for requirements). We'll follow this pattern for the new rules.

### Alternatives Considered

- Separate validation functions per mode: Not chosen - would duplicate code and complicate the validation flow
- Validation in types.go: Not chosen - types.go is for data structures, validation logic belongs in validator.go

## Files to Modify

- `internal/recipe/validator.go` - Add mode validation rules and expand dangerous patterns

## Files to Create

None (tests added to existing `validator_test.go`)

## Implementation Steps

- [ ] Add dangerous pattern detection for `||`, `&&`, `eval`, `exec`, `$()`, and backticks
- [ ] Add warning when `mode = "version"` pattern lacks `{version}`
- [ ] Add error when `mode = "output"` lacks `reason` field
- [ ] Add error when `mode = "functional"` is used (reserved for v2)
- [ ] Add unit tests for each validation rule

## Testing Strategy

- Unit tests: Each new validation rule with positive and negative cases
- Existing tests: Ensure no regressions in current dangerous pattern detection

## Risks and Mitigations

- Risk: False positives for dangerous patterns (e.g., `eval` in tool name)
  - Mitigation: Use word boundary patterns (prefix with space or start of string)
- Risk: Breaking existing recipes with warnings
  - Mitigation: Use warnings for `{version}` check, only errors for security issues

## Success Criteria

- [ ] Warn if `mode = "version"` pattern lacks `{version}`
- [ ] Error if `mode = "output"` lacks `reason` field
- [ ] Error if `mode = "functional"` used
- [ ] Detect dangerous patterns: `||`, `&&`, `eval`, `exec`, `$()`, backticks
- [ ] All unit tests pass
- [ ] No regressions in existing tests

## Open Questions

None
