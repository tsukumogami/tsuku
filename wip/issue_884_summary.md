# Issue 884 Summary

## What Was Implemented

Added retry logic with exponential backoff for transient HTTP errors (403, 429, 5xx) in download operations, combined with a proper User-Agent header. This addresses intermittent 403 errors from GNU FTP mirrors that were causing gdbm-source sandbox tests to fail in CI.

## Changes Made

- `internal/httputil/client.go`: Added `DefaultUserAgent` constant for consistent User-Agent header across downloads
- `internal/actions/download_file.go`:
  - Added `isRetryableStatusCode()` function to classify transient errors
  - Added `httpStatusError` type to track HTTP status codes through error handling
  - Refactored `downloadFileHTTP()` to include retry loop with exponential backoff
  - Added `doDownloadFileHTTP()` as the single-attempt download helper
  - Set User-Agent header on all requests
- `internal/actions/download.go`:
  - Refactored `downloadFile()` method with same retry logic pattern
  - Added `doDownloadFile()` as the single-attempt download helper
  - Set User-Agent header on all requests
- `internal/actions/download_test.go`: Added unit tests for retry behavior

## Key Decisions

- **Retry on 403**: GNU FTP mirrors return 403 intermittently due to rate limiting or bot detection. Retrying with backoff handles these transient cases.
- **3 retries max**: Reasonable limit that handles transient failures without excessive delays for genuine errors.
- **Exponential backoff (1s, 2s, 4s)**: Standard pattern that reduces load on servers while allowing time for rate limits to clear.
- **User-Agent pattern**: Reused existing pattern from other tsuku components (`tsuku/1.0 (https://github.com/tsukumogami/tsuku)`).

## Trade-offs Accepted

- **Retries slow down legitimate 403/5xx failures**: Acceptable because the retry delays (max 7s total) are short enough not to cause frustration, and the primary use case (transient mirror issues) benefits significantly.
- **No jitter in backoff**: Could add random jitter to prevent thundering herd, but given single-user CLI context this is unnecessary complexity.

## Test Coverage

- New tests added: 4 (TestIsRetryableStatusCode, TestHttpStatusError, TestDownloadAction_UserAgent, TestDownloadAction_NonRetryableErrorFailsImmediately)
- All existing tests pass

## Known Limitations

- Cannot fully test retry behavior in unit tests due to TLS certificate requirements with httptest.NewTLSServer. Tests verify the retry logic components individually.
- Some 403s are permanent (e.g., geo-blocking) and will still fail after retries.

## Future Improvements

- Consider adding jitter to exponential backoff if this is ever used in parallel download scenarios.
- Could expose retry configuration via environment variables if users need to adjust behavior.
