# Issue 20 Implementation Plan

## Summary

Create an `errmsg` package with helper functions that detect error types and return formatted error messages with actionable suggestions.

## Approach

Create a centralized error formatting package that:
1. Analyzes error types (ResolverError, network errors, not found, etc.)
2. Returns multi-line formatted messages with context and suggestions
3. Is called from CLI commands to enhance error output

This approach keeps error message logic in one place, making it easy to maintain and extend. CLI commands call `errmsg.Format(err)` to get enhanced messages.

### Alternatives Considered

- **Modify internal packages to return enhanced errors**: Rejected because it couples display logic to library code, making errors harder to parse programmatically
- **Add suggestions inline at each error site**: Rejected because it leads to duplication and inconsistency

## Files to Create

- `internal/errmsg/errmsg.go` - Error message formatting with suggestions

## Files to Modify

- `cmd/tsuku/install.go` - Use errmsg.Format for enhanced error output
- `cmd/tsuku/versions.go` - Use errmsg.Format for enhanced error output
- `cmd/tsuku/verify.go` - Use errmsg.Format for enhanced error output
- `cmd/tsuku/remove.go` - Use errmsg.Format for enhanced error output
- `cmd/tsuku/update.go` - Use errmsg.Format for enhanced error output

## Implementation Steps

- [ ] Create internal/errmsg/errmsg.go with Format function and error detection
- [ ] Add unit tests for errmsg package
- [ ] Update install.go to use errmsg.Format
- [ ] Update versions.go to use errmsg.Format
- [ ] Update verify.go to use errmsg.Format
- [ ] Update remove.go to use errmsg.Format
- [ ] Update update.go to use errmsg.Format
- [ ] Run tests and verify build

## Testing Strategy

- Unit tests: Test Format() with various error types (ResolverError, net.Error, generic errors)
- Manual verification: Simulate network errors and check output format

## Error Categories

| Error Type | Possible Causes | Suggestions |
|------------|-----------------|-------------|
| Network error | Connection failed, timeout | Check internet, retry later |
| Rate limit | Too many requests | Set GITHUB_TOKEN, wait, retry |
| Recipe not found | Typo, not in registry | Check spelling, run `tsuku recipes`, use `tsuku create` |
| Version not found | Invalid version | Run `tsuku versions <tool>` |
| Permission error | File system issue | Check permissions on $TSUKU_HOME |
| Dependency error | Missing dependency | Install required dependency first |

## Success Criteria

- [ ] Error messages include "Possible causes" section
- [ ] Error messages include "Suggestions" section with actionable steps
- [ ] Rate limit errors suggest setting GITHUB_TOKEN
- [ ] Recipe not found errors suggest `tsuku recipes` and `tsuku create`
- [ ] All existing tests pass
- [ ] Build succeeds

## Open Questions

None - approach is straightforward.
