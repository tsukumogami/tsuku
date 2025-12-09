# Issue 303 Implementation Plan

## Summary

Implement a `PreDownloader` in the validate package that downloads release assets to a temporary directory and computes SHA256 checksums during download for embedding in recipes.

## Approach

Follow the design document and reuse patterns from the existing `DownloadAction` for secure HTTP downloads. The PreDownloader will be self-contained within the validate package and compute checksums during the download process (streaming) rather than re-reading the file.

### Alternatives Considered

- **Reuse DownloadAction directly**: Not chosen because DownloadAction is tightly coupled to recipe execution context and outputs to stdout
- **Download then checksum**: Not chosen because reading the file twice is inefficient; streaming hash computation is better

## Files to Create

- `internal/validate/predownload.go` - PreDownloader implementation
- `internal/validate/predownload_test.go` - Unit tests

## Files to Modify

None - this is a new component

## Implementation Steps

- [ ] Create `PreDownloader` struct with configurable HTTP client and temp directory
- [ ] Implement `Download` method with streaming SHA256 computation using `io.TeeReader`
- [ ] Add HTTPS-only enforcement (security)
- [ ] Add SSRF protection via redirect validation (reuse logic from download.go)
- [ ] Add cleanup on failure (remove partial downloads)
- [ ] Create `DownloadResult` type with path, checksum, and size
- [ ] Add constructor `NewPreDownloader()` with sensible defaults
- [ ] Write unit tests with httptest mock server

## Testing Strategy

- Unit tests: Mock HTTP server to test:
  - Successful download with correct checksum
  - HTTP error handling (404, 500)
  - Non-HTTPS URL rejection
  - SSRF protection (redirect to private IPs)
  - Cleanup on failure
  - Large file handling with streaming

## Risks and Mitigations

- **Risk**: Temp directory cleanup on interrupted process
  - **Mitigation**: Use `$TMPDIR/tsuku-validate-*` prefix for easy identification; startup cleanup (#307) will handle orphans

- **Risk**: Network timeouts during download
  - **Mitigation**: Use configurable timeout via context, respect context cancellation

## Success Criteria

- [ ] `PreDownloader.Download()` successfully downloads files to temp directory
- [ ] SHA256 checksum is computed during download (not after)
- [ ] Download errors result in cleanup of partial files
- [ ] HTTPS-only enforcement works
- [ ] SSRF protection prevents redirects to private IPs
- [ ] All unit tests pass
- [ ] `go vet` and `golangci-lint` pass

## Open Questions

None - design document provides clear guidance
