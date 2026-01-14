# Issue 880 Implementation Plan

## Summary

Re-enable libsixel-source in macOS CI tests. Investigation shows the meson build now succeeds on both Apple Silicon and Intel - the issue appears to have been resolved by prior changes to meson_build.go or was a transient CI environment issue.

## Approach

The original failure was reported during PR #878 (CI batching work) and libsixel-source was preemptively excluded on January 13, 2026. However, CI run 20978439894 from January 14, 2026 shows libsixel-source successfully building on both macOS Apple Silicon and Intel. Local testing also confirms the build works on Apple Silicon.

The fix is to simply re-enable the test and verify CI passes. If failures recur, we will capture the actual error message and investigate further.

### Investigation Findings

1. **Local test (2026-01-14)**: libsixel-source builds and installs successfully on Apple Silicon (local machine)
2. **CI run 20978439894**: libsixel-source passed on both macOS Apple Silicon and Intel
3. **meson_build.go changes**: Recent commits (ddb7385, 02dcd59) fixed python/ninja resolution and dependency binary lookup which may have resolved the issue
4. **Upstream libsixel**: Issue #31 (macOS build failure for sys/io.h) was fixed in September 2021 - the current recipe uses v1.10.3 which includes this fix

### Alternatives Considered

- **Add macOS-specific meson args**: Not needed - the build works without modifications
- **Pin to a different libsixel version**: Not needed - v1.10.3 works correctly
- **Add platform constraints to recipe**: Not needed - the recipe is cross-platform compatible

## Files to Modify

- `.github/workflows/build-essentials.yml` - Re-enable libsixel-source in macOS test functions

## Files to Create

None

## Implementation Steps

- [x] Remove the comment excluding libsixel-source from macOS Apple Silicon test
- [x] Add the run_test call for libsixel-source in the Apple Silicon job
- [ ] Push changes and verify CI passes on macOS Apple Silicon

Note: macOS Intel tests are currently skipped due to runner deprecation (#896), so no changes needed there.

## Testing Strategy

- **CI verification**: The primary test is the GitHub Actions CI run itself
- **Local verification**: Already confirmed working on local Apple Silicon machine
- **No unit tests needed**: This is a CI workflow change, not code change

## Risks and Mitigations

- **Risk: CI failure recurs**: If the failure was intermittent, it may recur
  - **Mitigation**: The CI run will capture the actual error message, enabling proper diagnosis
  - **Fallback**: Re-exclude and open a new issue with specific error details

- **Risk: Apple Silicon vs Intel differences**: The recipe may work on ARM but fail on x86_64
  - **Mitigation**: macOS Intel tests are currently disabled (#896), so this is not blocking
  - **Note**: When Intel tests are re-enabled, verify libsixel-source works there too

## Success Criteria

- [ ] libsixel-source test passes in macOS Apple Silicon CI job
- [ ] No test regressions in other build-essentials tests
- [ ] CI run completes successfully on main branch

## Open Questions

None - the path forward is clear: re-enable and verify.
