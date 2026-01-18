# Issue 1025 Implementation Plan

## Summary
Add create-draft-release and finalize-release jobs to release.yml, wrapping the existing goreleaser job with draft-then-publish semantics.

## Approach
Modify the existing release workflow to:
1. Add a `create-draft-release` job that runs first and outputs release_id/upload_url
2. Make the existing `release` job depend on create-draft-release
3. Add a `finalize-release` job that publishes the draft after all builds complete

This is the minimal skeleton that downstream issues will build on.

## Files to Modify
- `.github/workflows/release.yml` - Add two new jobs (create-draft-release, finalize-release), add needs dependency to release job

## Files to Create
None

## Implementation Steps
- [ ] Add `create-draft-release` job with outputs (release_id, upload_url)
- [ ] Add `needs: [create-draft-release]` to existing release job
- [ ] Pass release_id to release job environment (for downstream #1026)
- [ ] Add `finalize-release` job depending on release job
- [ ] Test workflow YAML syntax with actionlint (if available)
- [ ] Run validation script from issue

## Success Criteria
- [ ] `create-draft-release` job exists and outputs release_id
- [ ] `release` job has needs dependency on create-draft-release
- [ ] `finalize-release` job exists with needs dependency
- [ ] Workflow YAML syntax is valid
- [ ] Validation script passes

## Open Questions
None - the design is clear and this is a straightforward structural change.
