# Issue 1612 Implementation Plan

## Summary

Extend GitHub verification to detect forks, fetch parent metadata, and populate new fork-related fields in the Metadata struct. Update quality threshold to exclude forks from auto-selection.

## Approach

The implementation adds fork detection inline within the existing `verifyGitHubRepo` function rather than extracting it to a separate function. This keeps the verification logic cohesive and avoids adding a second HTTP call path that would complicate the httpGet abstraction.

### Alternatives Considered

- **Separate fork verification function**: Would require passing the httpGet function through or making a second API call independently. Rejected because it fragments the verification logic and the parent data is already included in the initial API response.
- **Lazy parent fetch on-demand**: Only fetch parent metadata when displaying confirmation. Rejected because downstream issues (#1614 ranking, #1615 UX) need fork data at verification time for ranking and filtering decisions.

## Files to Modify

- `internal/discover/resolver.go` - Add IsFork, ParentRepo, ParentStars fields to Metadata struct
- `internal/discover/llm_discovery.go` - Extend ghRepo struct with Parent, populate fork metadata in verifyGitHubRepo, update passesQualityThreshold to exclude forks
- `internal/discover/llm_discovery_test.go` - Add tests for fork detection, parent metadata population, and quality threshold fork exclusion

## Files to Create

None.

## Implementation Steps

- [ ] Add fork fields to Metadata struct in resolver.go (IsFork bool, ParentRepo string, ParentStars int)
- [ ] Extend ghRepo struct in verifyGitHubRepo to parse Parent with FullName and StargazersCount
- [ ] Populate fork metadata in verifyGitHubRepo when Fork is true
- [ ] Handle graceful degradation when parent metadata is missing (set IsFork=true, leave ParentStars=0)
- [ ] Update passesQualityThreshold to return false when IsFork is true
- [ ] Add TestVerifyGitHubRepo_Fork test for fork detection
- [ ] Add TestVerifyGitHubRepo_ForkWithMissingParent test for graceful degradation
- [ ] Add TestPassesQualityThreshold_RejectsForks test for quality threshold
- [ ] Run go test ./internal/discover/... to verify all tests pass
- [ ] Run go vet ./... and go build ./cmd/tsuku to ensure clean build

## Testing Strategy

### Unit Tests

1. **TestVerifyGitHubRepo_Fork**: Mock HTTP response with `fork: true` and parent metadata. Verify:
   - Metadata.IsFork is true
   - Metadata.ParentRepo matches parent.full_name
   - Metadata.ParentStars matches parent.stargazers_count

2. **TestVerifyGitHubRepo_ForkWithMissingParent**: Mock HTTP response with `fork: true` but null parent. Verify:
   - Metadata.IsFork is true
   - Metadata.ParentRepo is empty string
   - Metadata.ParentStars is 0

3. **TestPassesQualityThreshold_RejectsForks**: Verify passesQualityThreshold returns false when IsFork is true, even if Stars exceeds MinStarsThreshold.

### Integration Verification

Run the validation script from the issue to confirm:
- Fork fields exist in resolver.go
- Fork detection code exists in llm_discovery.go
- IsFork is checked in quality threshold

## Risks and Mitigations

- **GitHub API response structure changes**: The parent field structure is well-documented and stable. Use pointer type for Parent struct to handle null gracefully.
- **Rate limiting on real API calls in tests**: Use mock httpGet function for all new tests. The existing TestVerifyGitHubRepo test already makes real API calls; new tests won't add to that burden.
- **Edge case: parent repo archived or deleted**: Not in scope for this issue; the parent data from the API response is used as-is. #1613 will handle rate limit edge cases.

## Success Criteria

- [ ] Metadata struct has IsFork, ParentRepo, ParentStars fields
- [ ] verifyGitHubRepo populates fork metadata when fork: true
- [ ] passesQualityThreshold returns false when IsFork is true
- [ ] All new tests pass: TestVerifyGitHubRepo_Fork, TestVerifyGitHubRepo_ForkWithMissingParent, TestPassesQualityThreshold_RejectsForks
- [ ] Validation script from issue passes
- [ ] go test ./internal/discover/... passes
- [ ] go vet ./... passes
- [ ] go build ./cmd/tsuku succeeds

## Open Questions

None. The issue spec is complete and the introspection confirmed no gaps require resolution.
