# Issue #20 Implementation Plan

## Summary

Create `internal/errmsg/` package to format errors with actionable suggestions, utilizing the existing `Suggestion()` methods on `version.ResolverError` and `registry.RegistryError`.

## Approach

The prerequisite issues (#112, #113, #114) added structured error types with `Suggestion()` methods to both version providers and registry client. This issue integrates these suggestions into CLI output by:

1. Creating a formatting package that extracts suggestions from errors
2. Updating CLI commands to use the formatter instead of raw `fmt.Fprintf`

The key insight is that both error types already have `Suggestion()` methods returning targeted suggestions. We just need to detect these errors at the CLI layer and display their suggestions.

### Alternatives Considered

- **String matching on error messages**: Not chosen because it's fragile and duplicates logic already in error types
- **Passing suggestions through all layers**: Not chosen because errors already carry their suggestions via `Suggestion()` method

## Files to Create

- `internal/errmsg/errmsg.go` - Error formatting with suggestion extraction
- `internal/errmsg/errmsg_test.go` - Tests for error formatting

## Files to Modify

- `cmd/tsuku/install.go` - Use errmsg for error output
- `cmd/tsuku/versions.go` - Use errmsg for error output
- `cmd/tsuku/update.go` - Use errmsg for error output
- `cmd/tsuku/outdated.go` - Use errmsg for error output
- `cmd/tsuku/info.go` - Use errmsg for error output
- `cmd/tsuku/helpers.go` - Add shared error printing helper

## Implementation Steps

- [ ] Create `internal/errmsg/errmsg.go` with FormatError function
- [ ] Create `internal/errmsg/errmsg_test.go` with comprehensive tests
- [ ] Add printError helper to `cmd/tsuku/helpers.go`
- [ ] Update install.go to use printError
- [ ] Update other commands (versions, update, outdated, info) to use printError
- [ ] Verify all tests pass

## Testing Strategy

- Unit tests: Test FormatError with various error types (ResolverError, RegistryError, generic errors)
- Manual verification: Run `tsuku install nonexistent` with network disconnected to see suggestions

## Design Notes

### FormatError Function

```go
// FormatError returns a formatted error message with suggestion if available
func FormatError(err error) string

// Suggester interface for errors that provide suggestions
type Suggester interface {
    Suggestion() string
}
```

The function will:
1. Check if error implements `Suggester` interface
2. If yes, append suggestion to error message
3. Format consistently: "Error: {message}\n\nSuggestion: {suggestion}"

### Error Type Detection

Use Go's `errors.As()` to detect `*version.ResolverError` and `*registry.RegistryError`, then call their `Suggestion()` methods. Alternative: define a `Suggester` interface that both types satisfy, and use interface type assertion.

## Risks and Mitigations

- **Risk**: Import cycle between errmsg and version/registry packages
  **Mitigation**: Use interface type assertion (`Suggester` interface) instead of importing concrete types

## Success Criteria

- [ ] Each error type has specific, actionable suggestions displayed
- [ ] Rate limit errors suggest setting GITHUB_TOKEN
- [ ] Timeout errors suggest checking network/retrying
- [ ] DNS errors suggest checking internet connection
- [ ] Recipe not found errors suggest `tsuku recipes`
- [ ] All existing tests pass
- [ ] New unit tests added for errmsg package

## Open Questions

None - design is straightforward given existing Suggestion() methods.
