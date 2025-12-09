# Issue 280 Implementation Plan

## Summary

Implement the `inspect_archive` tool handler in `internal/llm/client.go` that downloads archives, extracts them to a temp directory, lists files with executable detection, and cleans up.

## Approach

Leverage existing archive extraction patterns from `internal/actions/extract.go` which already handles tar.gz, tar.xz, tar.bz2, and zip formats. Create a new file `internal/llm/archive.go` with archive inspection logic to keep concerns separated and avoid importing the actions package (which has different responsibilities).

### Alternatives Considered

- **Reuse actions/extract.go directly**: Not chosen because that code is designed for installation workflows with different parameters (`ExecutionContext`, `params` map). The LLM tool needs a simpler interface that just returns a file listing.
- **Shell out to tar/unzip commands**: Not chosen because it would introduce platform dependencies and the Go standard library provides all needed functionality.

## Files to Modify

- `internal/llm/client.go` - Replace stub `inspectArchive` with call to new archive inspection logic

## Files to Create

- `internal/llm/archive.go` - Archive download, extraction, and file listing with executable detection
- `internal/llm/archive_test.go` - Unit tests with sample archives

## Implementation Steps

- [ ] Create `internal/llm/archive.go` with `InspectArchiveResult` type and `inspectArchive` implementation
- [ ] Implement archive format detection from URL/Content-Type
- [ ] Implement tar.gz extraction and file listing
- [ ] Implement tar.xz extraction and file listing
- [ ] Implement zip extraction and file listing
- [ ] Add executable bit detection for listed files
- [ ] Update `client.go` to use the new implementation
- [ ] Create `archive_test.go` with unit tests using sample archives
- [ ] Update existing stub test to use real implementation

## Testing Strategy

- **Unit tests**: Create sample tar.gz and zip archives in-memory or as test fixtures, verify:
  - File listing is correct
  - Executable detection works
  - Temp files are cleaned up
  - Error handling for invalid archives
  - Error handling for network failures (mock HTTP server)

- **Integration test**: Skip if no network; test against a real GitHub release asset

## Risks and Mitigations

- **Large archives**: Use size limits (10MB default) and streaming where possible to avoid memory exhaustion
- **Malicious archives**: Apply same security patterns as extract.go (path traversal prevention), but since we only list files (not extract to a target), risk is lower
- **Slow downloads**: Use context cancellation and timeouts from the HTTP client

## Success Criteria

- [ ] `inspectArchive` downloads and extracts archives
- [ ] Returns correct file listing with paths and sizes
- [ ] Correctly detects executable files
- [ ] Supports tar.gz, tar.xz, zip formats
- [ ] Cleans up temp files after inspection
- [ ] Unit tests pass
- [ ] Existing tests continue to pass

## Open Questions

None - requirements are clear from the issue and design doc.
