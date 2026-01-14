# Issue 895 Implementation Plan

## Summary
The Cargo Builder CI workflow uses incorrect PATH (`$HOME/.tsuku/tools/current`) instead of the correct tsuku bin directory (`$HOME/.tsuku/bin`). This prevents the installed hyperfine binary from being found.

## Approach
Change the PATH setup from `$HOME/.tsuku/tools/current` to `$HOME/.tsuku/bin` in both Linux and macOS jobs, and remove the `if: false` conditions to re-enable the jobs.

## Files to Modify
- `.github/workflows/cargo-builder-tests.yml` - Fix PATH setup and re-enable jobs

## Files to Create
None

## Implementation Steps
- [ ] Change line 44 from `$HOME/.tsuku/tools/current` to `$HOME/.tsuku/bin` (Linux job)
- [ ] Change line 89 from `$HOME/.tsuku/tools/current` to `$HOME/.tsuku/bin` (macOS job)
- [ ] Remove `if: false` condition from line 26 (Linux job)
- [ ] Remove `if: false` condition from line 71 (macOS job)
- [ ] Remove skip comments on lines 24 and 69

## Success Criteria
- [ ] Cargo Builder: Linux job passes
- [ ] Cargo Builder: macOS job passes
- [ ] Both jobs verify `hyperfine --version` succeeds

## Open Questions
None - the fix is straightforward based on working examples in other workflows.

## Notes
Other builder test workflows have the same incorrect PATH issue:
- `gem-builder-tests.yml` (lines 42, 87)
- `npm-builder-tests.yml` (lines 42, 85)
- `pypi-builder-tests.yml` (lines 42, 90)
- `homebrew-builder-tests.yml` (line 56)

These may need similar fixes but are out of scope for this issue.
