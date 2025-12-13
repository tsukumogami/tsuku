# Issue 470 Implementation Plan

## Summary

Create a new file `internal/executor/plan_cache.go` containing foundational types and functions for plan caching: `PlanCacheKey` struct, `CacheKeyFor()` helper, `ValidateCachedPlan()` validation function, and `ChecksumMismatchError` type.

## Approach

Follow the design document specification closely since it provides concrete code examples. Create a standalone file with no external dependencies beyond existing package types, enabling parallel development with other foundation issues.

### Alternatives Considered
- **Embed in existing plan.go**: Rejected because plan.go focuses on plan structure/validation, while cache infrastructure is a distinct concern. Separate file improves maintainability.
- **Put in install package**: Rejected because these types are closely tied to executor.InstallationPlan and will be used by executor methods.

## Files to Create
- `internal/executor/plan_cache.go` - PlanCacheKey, CacheKeyFor, ValidateCachedPlan, ChecksumMismatchError
- `internal/executor/plan_cache_test.go` - Unit tests for all functions

## Files to Modify
None - this is a new standalone file.

## Implementation Steps
- [ ] Create plan_cache.go with PlanCacheKey struct
- [ ] Add CacheKeyFor() function that generates cache key from resolution output
- [ ] Add ValidateCachedPlan() function that validates format version, platform, and recipe hash
- [ ] Add ChecksumMismatchError type with helpful error message (include tool and version)
- [ ] Add unit tests for PlanCacheKey and CacheKeyFor
- [ ] Add unit tests for ValidateCachedPlan
- [ ] Add unit tests for ChecksumMismatchError
- [ ] Run full test suite and lint checks

## Testing Strategy
- **Unit tests**:
  - CacheKeyFor: verify correct key generation from inputs
  - ValidateCachedPlan: test valid plan, format version mismatch, platform mismatch, recipe hash mismatch
  - ChecksumMismatchError: verify Error() returns expected message format with tool/version
- **Manual verification**: `go build ./...` and `go test ./internal/executor/...`

## Risks and Mitigations
- **Risk**: ValidateCachedPlan signature might need install.Plan instead of InstallationPlan
  - **Mitigation**: Design shows it validates against InstallationPlan. The install.Plan → InstallationPlan conversion happens in issue #475.
- **Risk**: Platform string parsing in ValidateCachedPlan could fail for unusual formats
  - **Mitigation**: Use strings.Cut() for safe parsing, document expected format "os-arch"

## Success Criteria
- [ ] `go test ./internal/executor/...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run ./internal/executor/...` passes
- [ ] All functions have corresponding unit tests
- [ ] ChecksumMismatchError message includes tool name and version for actionable recovery

## Open Questions
None - design document provides clear specifications.
