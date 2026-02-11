# Issue 1610 Summary

## What Was Implemented

Added exponential backoff retry logic to DDGProvider.Search() to handle DuckDuckGo's 202 rate limiting responses. The implementation follows the established retry pattern from `internal/actions/download.go` with added jitter to prevent thundering herd.

## Changes Made

- `internal/search/ddg.go`:
  - Added `DDGOptions` struct with configurable MaxRetries, Logger, and HTTPClient
  - Added `NewDDGProviderWithOptions` constructor for configurable behavior
  - Refactored Search() to include retry loop with exponential backoff and 25% jitter
  - Extracted HTTP request logic to `doSearch()` helper for retry isolation
  - Added debug-level logging for retry attempts

- `internal/search/ddg_test.go`:
  - Added fixture-based tests using `testdata/` HTML files
  - Added 202 retry success test (mock returns 202 twice, then 200)
  - Added max retries exceeded test
  - Added context cancellation test
  - Added non-retryable status test (404 fails immediately)
  - Added options constructor tests

- `internal/search/testdata/`:
  - Added `ddg_success.html` fixture with sample search results
  - Added `ddg_empty.html` fixture for empty results edge case

## Key Decisions

- **Inline retry in Search()**: Kept retry logic self-contained in the Search method rather than extracting to a separate package, matching the existing pattern in download.go
- **25% jitter variance**: Used `0.75 + rand*0.5` multiplier for jitter (75%-125% of base delay) to spread retries without excessive variance
- **HTTP 202 only retryable**: Unlike download.go which retries on 403/429/5xx, DDG provider only retries on 202 which is DDG's specific rate limiting signal

## Trade-offs Accepted

- **Test execution time**: Retry tests take ~6 seconds due to actual backoff delays. Accepted because tests use real time.After for accurate context cancellation testing
- **sleepFn unused**: DDGOptions has a sleepFn field for testing but we use time.After in the select statement for context cancellation support; the field remains for potential future use

## Test Coverage

- New tests added: 8 (3 fixture tests, 5 retry behavior tests)
- All acceptance criteria covered:
  - Success parse from fixture
  - 202 retry then success
  - Max retries exceeded
  - Context cancellation during backoff
  - Non-retryable status (404)
  - Options defaults and custom values

## Known Limitations

- Tests use actual time.After delays (not mocked), so retry tests take several seconds
- Jitter is non-deterministic (uses math/rand), so exact delay values can't be asserted in tests

## Future Improvements

- Consider seeding rand for reproducible jitter in tests
- Backoff delays could be made configurable via DDGOptions if different timing is needed
