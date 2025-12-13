# Issue 472 Implementation Plan

## Summary

Add a public `ResolveVersion` method to the Executor that exposes the existing internal version resolution logic, enabling the two-phase installation model where version resolution runs before cache lookup.

## Approach

Expose the existing `resolveVersionWith` method as a public `ResolveVersion` method. The implementation wraps the internal logic without duplication:

1. Create a version resolver
2. Use the provider factory to get the appropriate version provider from the recipe
3. Delegate to the provider's `ResolveVersion` or `ResolveLatest` method

### Alternatives Considered

- **Alternative 1: Return VersionInfo struct directly** - The design doc specifies returning just the version string (`string, error`). This is simpler and matches the orchestration needs. The full `VersionInfo` can be obtained by calling the method again if needed.

- **Alternative 2: Expose resolveVersionWith directly** - Making the existing method public would expose internal details (the resolver parameter). The new method should encapsulate resolver creation.

## Files to Modify

- `internal/executor/executor.go` - Add `ResolveVersion(ctx context.Context, constraint string) (string, error)` method

## Files to Create

None required - tests will be added to existing test file.

## Implementation Steps

- [x] Add `ResolveVersion` method to Executor that:
  - Creates a version resolver internally
  - Uses the provider factory to get the appropriate provider from the recipe
  - Handles constraint parameter (empty string = resolve latest)
  - Returns resolved version string
- [x] Add unit tests for `ResolveVersion` in `executor_test.go`:
  - Test with empty constraint (resolves to latest)
  - Test with specific version constraint
  - Test error handling for unknown version sources
  - Test with mock version provider

## Testing Strategy

- Unit tests: Test ResolveVersion with various constraints using existing version provider mocking patterns
- Use the existing test fixtures and patterns from `executor_test.go`
- Tests should cover:
  - Empty constraint resolves to latest
  - Specific version constraint
  - Error from provider propagates correctly
  - Unknown source handling

## Risks and Mitigations

- **Risk 1**: Network-dependent tests may be flaky
  - **Mitigation**: Follow existing patterns that log network failures as expected in unit tests

## Success Criteria

- [x] `ResolveVersion(ctx, constraint)` method added to Executor
- [x] Method returns resolved version string (e.g., "14.1.0")
- [x] Unit tests with version provider mocks pass
- [x] `go test ./internal/executor/...` passes
- [x] No changes to existing behavior (backward compatible)
