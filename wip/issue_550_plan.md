# Issue 550 Implementation Plan

## Summary
Enhance `buildAutotoolsEnv()` to accept `ResolvedDeps` parameter and construct PKG_CONFIG_PATH, CPPFLAGS, and LDFLAGS from dependency installation paths.

## Approach
Add ResolvedDeps to ExecutionContext, then update buildAutotoolsEnv() to iterate over dependencies and build environment variables pointing to their lib/pkgconfig, include/, and lib/ directories.

### Why This Approach
- **Minimal API changes**: Only adds one field to ExecutionContext
- **Consistent with existing patterns**: Follows the pattern of ToolsDir already in ExecutionContext
- **Centralized dependency resolution**: Uses existing ResolveDependencies() result
- **Simple implementation**: Straightforward path construction from ToolsDir + dep name-version

### Alternatives Considered
- **Pass StateManager to buildAutotoolsEnv()**: Would require StateManager dependency, more complex, breaks single responsibility
- **Global dependency registry**: Would introduce global state, harder to test
- **Action-level dependency resolution**: Would duplicate resolution logic already done at install time

## Files to Modify
- `internal/actions/action.go` - Add Dependencies field to ExecutionContext
- `internal/actions/configure_make.go` - Update buildAutotoolsEnv() signature and implementation
- `internal/executor/executor.go` - Pass resolved dependencies when creating ExecutionContext
- `internal/actions/configure_make_test.go` - Add unit tests for path construction

## Implementation Steps
- [ ] Add `Dependencies ResolvedDeps` field to ExecutionContext struct
- [ ] Update buildAutotoolsEnv() signature to use ctx parameter
- [ ] Implement PKG_CONFIG_PATH construction from dependencies
- [ ] Implement CPPFLAGS construction from dependencies
- [ ] Implement LDFLAGS construction from dependencies
- [ ] Update executor.go to resolve and pass dependencies
- [ ] Add unit tests for buildAutotoolsEnv() path construction
- [ ] Verify existing configure_make tests still pass

## Testing Strategy
- Unit tests: Test buildAutotoolsEnv() with mock dependencies to verify correct path construction
- Integration tests: Existing build-essentials CI tests will validate with real dependencies
- Manual verification: Not needed - CI covers all platforms

## Risks and Mitigations
- **Risk**: Dependencies not yet installed when building environment
  - **Mitigation**: Dependencies are already installed by ensurePackageManagersForRecipe() before action execution
- **Risk**: Path construction differs across platforms
  - **Mitigation**: Use filepath.Join() for cross-platform path handling
- **Risk**: Missing dependency directories (e.g., no lib/pkgconfig)
  - **Mitigation**: Check directory existence before adding to paths, skip gracefully

## Success Criteria
- [ ] buildAutotoolsEnv() constructs PKG_CONFIG_PATH from all install-time dependencies
- [ ] buildAutotoolsEnv() constructs CPPFLAGS with -I flags for all dependency include/ dirs
- [ ] buildAutotoolsEnv() constructs LDFLAGS with -L flags for all dependency lib/ dirs
- [ ] Existing CC/CXX zig fallback logic preserved
- [ ] Unit tests verify correct path construction with multiple dependencies
- [ ] All existing tests continue to pass

## Open Questions
None - approach is clear based on existing patterns in the codebase.
