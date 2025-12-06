# Issue 184 Implementation Plan

## Summary

Replace the existing Python-based recipe validation in CI with `tsuku validate --strict`, and add a nightly scheduled run to catch issues early.

## Approach

Modify the existing `validate-recipes` job in `.github/workflows/test.yml` to use `tsuku validate --strict` instead of the Python script, and add a separate nightly schedule trigger. This approach reuses existing infrastructure and follows established patterns.

### Alternatives Considered

- **Create new workflow file**: Rejected because test.yml already has a `validate-recipes` job and the matrix job detects recipe changes. Adding would duplicate logic.
- **Add to scheduled-tests.yml**: Rejected because that file focuses on installation tests, not validation. Keeping validation in test.yml maintains separation of concerns.

## Files to Modify

- `internal/recipe/validator.go` - Add `go_install` to known actions, add `goproxy`/`nixpkgs` to known version sources
- `internal/recipe/validator_test.go` - Add tests for new action/version sources
- `.github/workflows/test.yml` - Replace Python validation with `tsuku validate --strict`, add nightly schedule trigger for validation

## Files to Create

None

## Implementation Steps

- [x] Fix validator to recognize `go_install` action
- [x] Fix validator to recognize `nixpkgs` version source (goproxy was already recognized)
- [ ] Update test.yml to add schedule trigger for recipe validation (nightly at 00:00 UTC)
- [ ] Replace Python-based validation with Go-based tsuku validate --strict
- [ ] Verify workflow syntax is valid
- [ ] Test locally that tsuku validate --strict works on all recipes

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy

- Manual verification: Run `tsuku validate --strict` on all recipes locally to confirm no failures
- CI verification: Push branch to verify workflow runs correctly

## Risks and Mitigations

- **Risk**: Some existing recipes fail `--strict` validation (50 recipes currently)
  - **Mitigation**: Root causes identified:
    1. `go_install` action not in validator's known actions list (bug)
    2. Missing `github_repo` when using github version source (warning)
    3. Unknown version source 'nixpkgs' (not yet supported)
  - These are pre-existing issues that should be addressed in separate PRs
- **Risk**: Build step adds time to CI
  - **Mitigation**: Go build is already cached in workflow; incremental overhead is minimal

## Success Criteria

- [ ] Workflow runs `tsuku validate --strict` on all recipe files in `internal/recipe/recipes/`
- [ ] Validation runs on pull_request events that modify recipes
- [ ] Validation runs on scheduled nightly cron (00:00 UTC)
- [ ] CI fails if any recipe has validation errors or warnings
- [ ] Actions pinned to commit SHAs (note: existing workflow uses @v4/v5 tags, will match existing pattern)

## Open Questions

None
