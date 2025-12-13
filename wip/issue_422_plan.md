# Issue 422 Implementation Plan

## Summary

Add a `Logger log.Logger` field to `ExecutionContext` and integrate debug logging into the download, extract, and install_binaries actions using the existing `log.Logger` interface and `log.SanitizeURL` function.

## Approach

Add the Logger field to ExecutionContext with a getter method that falls back to `log.Default()` if nil. This allows all actions to use logging without requiring changes to every call site that creates an ExecutionContext.

### Alternatives Considered
- **Pass Logger as parameter to each action**: Rejected because it would require changing the Action interface and updating all ~80 action implementations.
- **Global logger only**: Rejected because it makes testing harder and doesn't follow the context pattern already established.

## Files to Modify
- `internal/actions/action.go` - Add Logger field and Log() helper method to ExecutionContext
- `internal/actions/download.go` - Add debug logging for URL, cache status, checksum
- `internal/actions/extract.go` - Add debug logging for archive type, destination
- `internal/actions/install_binaries.go` - Add debug logging for binary paths, symlinks
- `internal/executor/executor.go` - Set Logger on ExecutionContext using log.Default()

## Files to Create
- None

## Implementation Steps
- [x] Add Logger field and Log() method to ExecutionContext in action.go
- [x] Update executor.go to set Logger from log.Default()
- [x] Add debug logging to download.go (URL sanitized, cache status, checksum)
- [x] Add debug logging to extract.go (archive type, destination path)
- [x] Add debug logging to install_binaries.go (binary paths, symlink creation)
- [ ] Run tests and validate with --debug flag

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: Existing tests continue to pass (Logger is optional/nil-safe)
- Manual verification: Build tsuku, run `tsuku install --debug <tool>` and verify debug output appears

## Risks and Mitigations
- **Risk**: Breaking existing tests that create ExecutionContext without Logger
- **Mitigation**: Use nil-safe Log() method that falls back to log.Default()

## Success Criteria
- [ ] ExecutionContext has Logger field with nil-safe access
- [ ] download.go logs sanitized URL, cache hit/miss, checksum verification
- [ ] extract.go logs archive type and destination path
- [ ] install_binaries.go logs binary paths and symlink creation
- [ ] All existing tests pass
- [ ] Debug output visible with `tsuku install --debug <tool>`

## Open Questions
None - design is straightforward based on existing log.Logger interface and ExecutionContext pattern.
