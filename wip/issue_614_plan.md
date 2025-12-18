# Issue 614 Implementation Plan

## Summary

Upon investigation, the strip_dirs issue appears to already be resolved in the current codebase. Testing shows that `strip_dirs=1` correctly strips the leading directory component during extraction in sandbox mode.

## Investigation Findings

1. **Code review confirms correct implementation**:
   - `download_archive` Decompose() correctly passes `strip_dirs` to extract step (composites.go:103)
   - `extract` action correctly reads `strip_dirs` parameter (extract.go:123)
   - Strip logic correctly implemented in tar (extract.go:297-303) and zip (extract.go:403-409)

2. **Manual testing confirms it works**:
   - Tested `golang` recipe with `strip_dirs=1`
   - Plan correctly generated with `"strip_dirs": 1` in extract step
   - Sandbox execution **PASSED**
   - Files correctly placed at `bin/go` not `go/bin/go`

3. **CI has exclusions but they may be outdated**:
   - sandbox-tests.yml excludes golang, nodejs, perl with comment "see #614"
   - However, manual test shows the functionality works correctly

## Approach

Since manual testing shows the feature works correctly, the issue may have been:
1. Fixed inadvertently in a recent commit
2. A misunderstanding of the expected behavior
3. Only reproducible under specific CI conditions

**Proposed resolution**: Remove the exclusions from sandbox-tests.yml and verify tests pass in CI.

### Alternatives Considered

- **Add explicit strip_dirs handling**: Not needed - code review shows it's already implemented correctly
- **Fix extract logic**: Not needed - manual testing shows correct behavior

## Files to Modify

- `.github/workflows/sandbox-tests.yml` - Remove exclusions for archive_golang_directory, archive_nodejs_checksum, archive_perl_relocatable

## Implementation Steps

- [ ] Remove test exclusions from sandbox-tests.yml
- [ ] Verify tests pass locally
- [ ] Run full CI to confirm tests pass
- [ ] Update issue with findings

## Testing Strategy

- Run sandbox tests for all three previously excluded recipes
- Verify they pass without modification to application code
- Monitor CI results

## Risks and Mitigations

- **Risk**: Tests might still fail in CI due to environmental differences
  - **Mitigation**: Check CI logs carefully and investigate any failures

- **Risk**: Issue creator had a legitimate bug that we're not reproducing
  - **Mitigation**: Ask for additional context or reproduction steps

## Success Criteria

- [ ] archive_golang_directory sandbox test passes
- [ ] archive_nodejs_checksum sandbox test passes
- [ ] archive_perl_relocatable sandbox test passes
- [ ] All CI checks pass
- [ ] Exclusions removed from sandbox-tests.yml

## Open Questions

- Why were the tests failing originally when the code appears correct?
- Was there a recent change that inadvertently fixed the issue?
- Are there specific conditions needed to reproduce the original bug?
