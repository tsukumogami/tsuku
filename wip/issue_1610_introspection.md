# Issue 1610 Introspection

## Context Reviewed

- **Design doc**: `docs/designs/DESIGN-llm-discovery-implementation.md` (status: Planned)
- **Sibling issues reviewed**: None closed (all 6 issues in milestone are OPEN)
- **Prior patterns identified**:
  - Retry pattern in `internal/actions/download.go` and `download_file.go`
  - `isRetryableStatusCode()` function pattern for HTTP status classification
  - Exponential backoff with `1s, 2s, 4s` base delays
  - Context cancellation check during backoff wait

## Gap Analysis

### Minor Gaps

1. **Established retry pattern**: The codebase has a consistent retry pattern in `internal/actions/download.go` (lines 349-396) that issue #1610 should follow:
   - `const maxRetries = 3`
   - Base delay of `1s` with exponential backoff (`1<<(attempt-1)`)
   - Context cancellation check during wait
   - Error type for HTTP status classification (`httpStatusError` pattern)

2. **Status code classification**: The existing `isRetryableStatusCode()` function in `internal/actions/download_file.go` considers 403, 429, and 5xx as retryable. The issue specifies 202 (Accepted) as the retryable status for DDG rate limiting. This is DDG-specific behavior that should be documented in code comments.

3. **Jitter implementation**: The issue specifies "random variance" for backoff jitter, but the existing pattern in `download.go` does not include jitter. This is an enhancement for DDG retry that goes beyond the established pattern. Implementation should add jitter (e.g., `+/- 25%` variance).

4. **Test fixture location**: The issue specifies `internal/search/testdata/` for HTML fixtures. This directory does not exist yet and needs to be created.

### Moderate Gaps

None identified. The issue specification is complete and actionable.

### Major Gaps

None identified. The issue does not conflict with any prior work.

## Recommendation

**Proceed**

The issue spec is complete. The only gaps are minor implementation details that can be resolved by following existing patterns:
- Use the retry pattern from `internal/actions/download.go`
- Create `internal/search/testdata/` for HTML fixtures
- Add 202 to retryable status codes (DDG-specific)
- Implement jitter as specified (small enhancement over existing pattern)

## Proposed Amendments

No amendments needed. Minor gaps can be incorporated during implementation:

1. Follow retry pattern from `internal/actions/download.go`:
   - 3 max attempts (configurable per AC)
   - Exponential backoff with 1s base
   - Context cancellation check

2. Create test fixtures at `internal/search/testdata/`:
   - `ddg_success.html` - successful search response
   - `ddg_rate_limited.html` - 202 response body (if any)

3. Add jitter implementation per AC (25% variance suggested based on common practice)
