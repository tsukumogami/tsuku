# Issue 1841 Implementation Plan

## Summary

Fix the overly aggressive `checkExistingRecipe` guard in `runCreate` that blocks exact name matches, and add `@critical` tags to key `create.feature` scenarios so they run in every PR CI, not just post-merge.

## Approach

Two-pronged fix: correct the call-site logic in `create.go` (same as PR #1840), then add CI guardrails by tagging representative create scenarios as `@critical`. The existing CI already runs `@critical`-tagged functional tests on every PR that touches code, but `create.feature` had zero `@critical` tags, which is why the regression wasn't caught before merge.

### Alternatives Considered
- **Always run full functional tests on PRs**: Would catch everything but increases CI time for every PR, even non-code changes. The `@critical` subset strategy is already established and works well for other feature files.
- **Add CI path-mapping (create.go changes trigger create.feature)**: Too fragile to maintain. File-to-feature mappings would need constant updating as code moves around.

## Files to Modify
- `cmd/tsuku/create.go` - Change the condition at line 486 to only block when `canonicalName != toolName`
- `test/functional/features/create.feature` - Add `@critical` tags to representative scenarios covering the most common create paths

## Implementation Steps
- [x] Fix `runCreate` condition to only block on satisfies-alias matches (not exact name matches)
- [x] Add `@critical` tags to representative create.feature scenarios (npm, pypi, discovery, deterministic-only)
- [x] Run unit tests to verify the fix
- [x] Run functional tests to verify create scenarios pass

## Testing Strategy
- Unit tests: Existing `TestCheckExistingRecipe_*` tests already cover the helper function. The fix is at the call site.
- Functional tests: The 8 previously-failing scenarios in `create.feature` serve as regression tests.
- CI validation: After PR creation, verify functional tests run in the `test-functional-critical` path.

## Risks and Mitigations
- **Risk**: Tagging too many scenarios as `@critical` slows down PR CI.
  **Mitigation**: Tag only 3-4 representative scenarios, not all 18. Cover the most common paths (npm, pypi, discovery, deterministic-only).
- **Risk**: PR #1840 already open with the code fix; this PR supersedes it.
  **Mitigation**: Close PR #1840 after this PR is created, referencing it.

## Success Criteria
- [ ] `tsuku create prettier --from npm` succeeds (exit 0)
- [ ] `tsuku create openssl@3` is still blocked by satisfies check
- [ ] `@critical`-tagged create scenarios exist and run in `make test-functional-critical`
- [ ] All unit tests pass
- [ ] All functional tests pass
