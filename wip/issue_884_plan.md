# Issue 884 Implementation Plan

## Summary

Add retry logic with exponential backoff for transient HTTP errors (403, 429, 5xx) in the download_file action, combined with adding a proper User-Agent header to downloads from GNU FTP mirrors and other sources.

## Approach

The GNU FTP mirror (ftpmirror.gnu.org) returns 403 errors intermittently, likely due to rate limiting or bot detection. The fix involves two complementary changes:

1. **Add User-Agent header**: Many servers, including GNU mirrors, block requests without a User-Agent or with generic Go HTTP client User-Agents. Adding a proper User-Agent will prevent many 403s from occurring in the first place.

2. **Add retry logic**: For transient errors that still occur (403, 429, 5xx), implement exponential backoff retry. This handles genuine rate limiting and temporary server issues.

This approach mirrors how tsuku already handles similar issues in other parts of the codebase (see `internal/builders/cargo.go`, `internal/builders/homebrew.go` for User-Agent patterns, and `internal/builders/github_release.go` for retry patterns).

### Alternatives Considered

- **Alternative mirrors only**: Use alternative GNU mirrors (e.g., ftp.gnu.org, mirrors.kernel.org). **Why not chosen**: Relies on external mirror availability; doesn't fix the root cause for other similar sources; User-Agent fix is simpler and more universally applicable.

- **Download caching in CI only**: Pre-cache downloads in CI workflow. **Why not chosen**: Adds complexity to CI; doesn't help local users experiencing the same issue; retry logic is more robust.

- **Hardcode mirror list with fallback**: Try multiple mirrors in sequence. **Why not chosen**: Over-engineered for this issue; User-Agent + retry addresses the root cause without maintaining mirror lists.

## Files to Modify

- `internal/actions/download_file.go` - Add User-Agent header and retry logic to `downloadFileHTTP` function
- `internal/actions/download.go` - Add User-Agent header and retry logic to `downloadFile` method
- `internal/httputil/client.go` - Add User-Agent constant for reuse across the codebase

## Files to Create

None required - the changes fit naturally into existing files.

## Implementation Steps

- [ ] Add `DefaultUserAgent` constant to `internal/httputil/client.go` (value: `tsuku/1.0 (https://github.com/tsukumogami/tsuku)`)
- [ ] Update `downloadFileHTTP` in `internal/actions/download_file.go`:
  - Add `User-Agent` header using the new constant
  - Implement retry loop with exponential backoff for 403, 429, 5xx status codes
  - Add max retries (3) and base delay (1 second) with exponential growth
- [ ] Update `downloadFile` in `internal/actions/download.go`:
  - Add `User-Agent` header using the new constant
  - Implement the same retry logic for consistency
- [ ] Add unit tests for retry behavior in `internal/actions/download_test.go`
- [ ] Run existing tests to verify no regressions
- [ ] Test manually against GNU FTP mirror to verify the fix works

## Testing Strategy

- **Unit tests**: Add tests for retry behavior in `internal/actions/download_test.go`:
  - Test that retries occur on 403, 429, and 5xx responses
  - Test that retries stop after max attempts
  - Test that successful responses on retry are handled correctly
  - Test that non-retryable errors (400, 404) fail immediately

- **Integration tests**: The existing CI workflows test gdbm-source installation, which will validate the fix works end-to-end

- **Manual verification**: Test locally by installing gdbm-source and observing download behavior

## Risks and Mitigations

- **Risk**: Retries could slow down legitimate failures.
  **Mitigation**: Only retry on transient error codes (403, 429, 5xx); non-retryable errors (400, 404) fail immediately. Max 3 retries with reasonable delays (1s, 2s, 4s).

- **Risk**: Some 403s are permanent (e.g., geo-blocking).
  **Mitigation**: 3 retries is a reasonable limit; if all fail, the user gets a clear error. The retry delay is short enough to not cause excessive wait times.

- **Risk**: User-Agent change could be rejected by some servers.
  **Mitigation**: The User-Agent follows standard conventions and is already used successfully in other parts of tsuku (builders, version providers).

## Success Criteria

- [ ] gdbm-source sandbox test passes reliably in CI (no more 403 failures)
- [ ] User-Agent header is set for all downloads
- [ ] Retry logic handles 403, 429, and 5xx with exponential backoff
- [ ] Non-retryable errors (400, 404) fail immediately without retry
- [ ] Unit tests cover the retry behavior
- [ ] No regressions in existing download functionality

## Open Questions

None - all acceptance criteria are addressable with the proposed approach.
