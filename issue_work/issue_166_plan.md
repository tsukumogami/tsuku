# Issue 166 Implementation Plan

## Summary

Fix the validator to check for `asset_pattern` parameter (matching the installer) instead of `asset` for `github_archive` and `github_file` actions.

## Approach

Simple parameter name fix in the validator. The installer uses `asset_pattern` as the canonical parameter name, so the validator should match.

### Alternatives Considered
- Modify installer to accept both `asset` and `asset_pattern`: Rejected - adds complexity and inconsistency
- Support both in validator: Rejected - recipes should use the canonical name `asset_pattern`

## Files to Modify
- `internal/recipe/validator.go` - Change `asset` to `asset_pattern` in github_archive/github_file validation
- `internal/recipe/validator_test.go` - Update test cases to use `asset_pattern`

## Implementation Steps
- [ ] Update validator.go to check for `asset_pattern` instead of `asset`
- [ ] Update validator_test.go test cases to use `asset_pattern`
- [ ] Run tests to verify fix

## Testing Strategy
- Unit tests: Update existing tests, verify validator accepts `asset_pattern`
- Manual verification: Run `tsuku validate --strict` on a recipe with `asset_pattern`

## Risks and Mitigations
- Breaking existing recipes that use `asset`: Low risk - the installer already requires `asset_pattern`, so any working recipe already uses this name

## Success Criteria
- [ ] Validator accepts recipes with `asset_pattern` for github_archive/github_file
- [ ] Validator rejects recipes missing `asset_pattern` for github_archive/github_file
- [ ] All tests pass
