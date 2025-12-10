# Issue 380 Implementation Plan

## Summary

Create structured error message templates for common LLM builder failures that provide users with clear next steps.

## Approach

Follow the existing error handling patterns established in `internal/version/errors.go` and `internal/registry/errors.go`:
- Errors implement the `errmsg.Suggester` interface (with `Suggestion()` method)
- Error types include structured fields for context (times, limits, etc.)
- Consistent formatting with what/why/how-to-fix structure

### Error Categories to Implement

1. **Rate Limit Error** - When hourly rate limit is exceeded
2. **Budget Error** - When daily LLM budget is exhausted
3. **GitHub API Errors** - Rate limit and repo not found
4. **LLM API Errors** - Authentication failure
5. **Validation Error** - Recipe validation failed after repair attempts

### Alternatives Considered

- **Extending existing error packages**: Rejected because these errors are specific to the LLM builder and don't belong in version or registry packages.
- **Adding to errmsg package**: Rejected because errmsg is for error formatting, not error definition.

## Files to Create

- `internal/builders/errors.go` - Error type definitions
- `internal/builders/errors_test.go` - Unit tests for error formatting

## Files to Modify

- `internal/builders/github_release.go` - Use new error types instead of `fmt.Errorf`

## Implementation Steps

- [x] 1. Create error types in `internal/builders/errors.go`:
  - RateLimitError (hourly generation limit)
  - BudgetError (daily cost budget)
  - GitHubRateLimitError (GitHub API rate limit)
  - GitHubRepoNotFoundError (repository not found)
  - LLMAuthError (authentication failure)
  - ValidationError (repair loop exhausted)

- [x] 2. Add Suggestion() method to each error type implementing errmsg.Suggester

- [x] 3. Add unit tests for error formatting in `internal/builders/errors_test.go`

- [x] 4. Update github_release.go to use new error types where appropriate

- [x] 5. Run tests and verify implementation

## Testing Strategy

- Unit tests for each error type verifying:
  - Error() output format
  - Suggestion() output includes actionable steps
  - Proper wrapping of underlying errors

## Success Criteria

- [ ] Error templates for rate limiting, budget, GitHub API, LLM API failures
- [ ] Each error includes actionable recovery steps
- [ ] Consistent formatting across all error types
- [ ] Errors include relevant context (wait times, limits, URLs)
- [ ] Unit tests for error formatting
