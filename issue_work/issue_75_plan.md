# Issue 75 Implementation Plan

## Summary

Add go mod tidy verification check to release.yml as a fail-fast guard before GoReleaser runs.

## Approach

Copy the existing go mod tidy check from test.yml (lines 30-36) and add it to release.yml between "Set up Go" and "Run GoReleaser" steps. The error message is adjusted to be release-specific.

## Files to Modify
- `.github/workflows/release.yml` - Add go mod tidy verification step

## Implementation Steps
- [ ] Add "Verify go.mod is tidy" step after "Set up Go" in release.yml

## Testing Strategy
- CI will validate the workflow syntax
- No functional tests needed - this is a CI workflow change

## Success Criteria
- [ ] Workflow YAML is valid
- [ ] Check runs before GoReleaser
- [ ] Error message is clear for release context
