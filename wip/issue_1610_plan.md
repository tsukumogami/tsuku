# Issue 1610 Implementation Plan

## Summary

Add exponential backoff retry logic to DDGProvider.Search() for HTTP 202 rate limiting responses, following the established retry pattern in `internal/actions/download.go`, with jitter added to prevent thundering herd. Tests will use recorded HTML fixtures.

## Approach

Adapt the existing retry pattern from `internal/actions/download.go` (lines 349-396) to the DDG search context. The established pattern uses a retry loop with exponential backoff (1s, 2s, 4s base delays), context cancellation checks during waits, and an `httpStatusError` type for status code classification. This implementation will extend that pattern with:

1. DDG-specific 202 status code handling (rate limiting)
2. Jitter added to backoff delays (25% variance) to prevent thundering herd
3. Configurable max retries (default: 3 attempts)
4. Debug-level logging via the `internal/log` package

### Alternatives Considered

- **Inline retry in Search()**: Directly embed retry loop in the Search method. Chosen because it keeps the logic self-contained and follows the existing download.go pattern.

- **Separate retry package**: Extract a generic retry utility for reuse. Rejected because:
  1. The actions package already has its own pattern - adding a third location would fragment the codebase
  2. DDG-specific 202 handling and the download 403/429/5xx handling have different semantics
  3. YAGNI - wait until a third use case emerges

- **Retry middleware/decorator**: Wrap the HTTP client with retry middleware. Rejected because:
  1. More complex than needed for a single provider
  2. Would obscure the DDG-specific 202 handling
  3. Harder to test specific retry scenarios

## Files to Modify

- `internal/search/ddg.go` - Add retry loop, backoff with jitter, logging, and configurable max retries

## Files to Create

- `internal/search/testdata/ddg_success.html` - Recorded successful DDG HTML response
- `internal/search/testdata/ddg_empty.html` - DDG response with no results (for edge case testing)
- `internal/search/ddg_retry_test.go` - Retry-specific tests using mock HTTP server (or extend ddg_test.go)

## Implementation Steps

- [x] Create testdata directory and add recorded HTML fixtures
- [x] Add DDGProviderOptions struct with MaxRetries, Logger, and HTTPClient fields
- [x] Add NewDDGProviderWithOptions constructor that accepts options
- [x] Extract HTTP request logic to a helper method (doSearch) for retry isolation
- [x] Implement retry loop in Search() with exponential backoff and jitter
- [x] Add 202 status code detection as retryable condition
- [x] Add debug-level logging for retry attempts
- [x] Update existing tests to use fixture files
- [x] Add test: successful parse from fixture
- [x] Add test: 202 retry then success
- [x] Add test: max retries exceeded
- [x] Add test: context cancellation during backoff
- [x] Run `go test ./internal/search/...` to verify all tests pass

## Testing Strategy

### Unit Tests

All tests in `internal/search/ddg_test.go` (or `ddg_retry_test.go`) will use either:
1. HTML fixtures from `testdata/` for parsing tests
2. `httptest.NewServer` for retry behavior tests

Test cases:
1. **Successful parse**: Load `testdata/ddg_success.html`, verify results extracted correctly
2. **202 retry success**: Mock server returns 202 twice, then 200 - verify 3 attempts made, results returned
3. **Max retries exceeded**: Mock server always returns 202 - verify error after configured attempts
4. **Context cancellation**: Cancel context during backoff wait - verify ctx.Err() returned promptly
5. **Non-retryable status**: Mock server returns 404 - verify immediate failure (no retry)

### No Live HTTP Calls

The existing integration test (`TestDDGProvider_Integration`) is gated behind `INTEGRATION_TESTS=1`. All new tests will be pure unit tests with no network dependency.

## Risks and Mitigations

- **DDG HTML structure changes**: Mitigated by using recorded fixtures. Fixtures document the expected structure and tests will fail fast if parsing breaks.

- **Jitter implementation correctness**: Use `math/rand` with 25% variance (base * (0.75 + rand * 0.5)). Add a test that verifies delays fall within expected range by running multiple iterations.

- **Backoff delays slow down tests**: Use a mock clock or injectable delay function. The retry loop will accept a `sleep` function for testing that can return immediately.

- **Thundering herd despite jitter**: The 25% variance is standard practice. For truly independent requests, this provides sufficient spread.

## Success Criteria

- [x] DDGProvider retries on HTTP 202 with exponential backoff
- [x] Max retries is configurable (default: 3)
- [x] Backoff includes jitter (25% variance)
- [x] Context cancellation stops retry loop promptly
- [x] Debug-level logging shows retry attempts
- [x] All tests use fixtures or mocks (no live HTTP)
- [x] Tests cover: success, 202 retry, max retries, context cancellation
- [x] `go test ./internal/search/...` passes
- [x] `go vet ./internal/search/...` passes

## Open Questions

None - all implementation details are clear from the issue spec and existing patterns.
