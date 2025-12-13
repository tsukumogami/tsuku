# Issue 489 Summary

## What Was Implemented

Dependency tree discovery for Homebrew formulas that traverses the JSON API before LLM invocation, allowing users to see the full scope and cost estimate before committing to recipe generation.

## Changes Made

- `internal/builders/homebrew.go`:
  - Added `RegistryChecker` interface for dependency injection
  - Added `DependencyNode` struct for tree representation
  - Added `DiscoverDependencyTree()` for recursive API traversal
  - Added `ToGenerationOrder()` for topological sort (leaves first)
  - Added `FormatTree()` for human-readable tree display
  - Added `EstimatedCost()` and `CountNeedingGeneration()` helpers
  - Added `ConfirmationRequest` and `NewConfirmationRequest()` for user flow
  - Added `BuildWithDependencies()` for dependency-aware recipe generation
  - Added `WithRegistryChecker()` option for builder configuration
  - Added `ErrUserCancelled` error for user cancellation

- `internal/builders/homebrew_test.go`:
  - Added `mockRegistryChecker` for testing
  - Added tests for `ToGenerationOrder()` (empty, single, linear, diamond, mixed status)
  - Added tests for `DiscoverDependencyTree()` (no deps, with deps, diamond, existing recipes, errors)
  - Added tests for `FormatTree()` (simple, with children, with recipe, diamond)
  - Added tests for `EstimatedCost()` and `NewConfirmationRequest()`
  - Added tests for `BuildWithDependencies()` (cancelled, all exist, confirm data)

## Key Decisions

- **Visited set for diamond dependencies**: Uses a visited map during tree traversal to ensure shared dependencies are discovered once and shared in the tree structure
- **Topological sort leaves-first**: Dependencies are generated before dependents, ensuring transitive deps are available during validation
- **Optional registry checker**: When registry is nil, all formulas are assumed to need generation
- **Confirmation callback pattern**: `ConfirmFunc` allows the caller to control the confirmation UX without coupling to stdio

## Trade-offs Accepted

- **Sequential API calls**: Each formula is queried sequentially rather than in parallel. This is simpler and avoids rate limiting concerns. Parallel fetching can be added later if needed.
- **No caching of API responses**: Each tree discovery makes fresh API calls. For repeated operations, callers can cache results externally.

## Test Coverage

- New tests added: 24 tests for dependency tree functionality
- No coverage regression (existing tests continue to pass)

## Known Limitations

- API rate limiting: Homebrew API is queried sequentially without rate limiting. Very deep dependency trees could potentially hit rate limits.
- No build dependencies: Only runtime dependencies are traversed (matches design doc specification)

## Future Improvements

- Parallel API calls with rate limiting for faster discovery on large trees
- Cache API responses for repeated operations
- Progress percentage during discovery (currently just formula name)
