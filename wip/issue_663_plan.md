# Issue 663 Implementation Plan

## Summary
Add an `Env` field to `ExecutionContext` and modify `setup_build_env` to populate it, then update `configure_make` and `cmake_build` to use the shared environment when available.

## Approach
This implementation follows Option 1 from the issue description: implement full environment modification in `setup_build_env` and have build actions use `ctx.Env` if already set. This aligns with the design document's intent (DESIGN-dependency-provisioning.md lines 488-509) where `setup_build_env` directly modifies `ctx.Env`, creating a shared environment that subsequent steps inherit.

**Why this approach:**
- Matches the design document's vision for reusable environment setup
- Maintains backward compatibility (existing recipes without `setup_build_env` continue to work)
- Enables future recipes to use `setup_build_env` for cleaner, more explicit dependency environment configuration
- Allows `cmake_build` to benefit from autotools-style dependency path configuration when used with `setup_build_env`

### Alternatives Considered
- **Update design document to reflect current no-op behavior**: This would formalize the status quo but misses the opportunity to provide cleaner environment setup patterns for future recipes. Not chosen because the design's vision is sound and provides value.
- **Remove setup_build_env entirely**: Would simplify the codebase but eliminate the explicit environment setup step that makes dependency configuration more transparent. Not chosen because explicit setup improves recipe clarity.

## Files to Modify
- `internal/actions/action.go` - Add `Env []string` field to `ExecutionContext`
- `internal/actions/setup_build_env.go` - Modify `Execute()` to set `ctx.Env` instead of just displaying it
- `internal/actions/configure_make.go` - Update `Execute()` to use `ctx.Env` if set, otherwise call `buildAutotoolsEnv()`
- `internal/actions/cmake_build.go` - Update `Execute()` to use `ctx.Env` if set, otherwise call `buildCMakeEnv()`
- `internal/actions/setup_build_env_test.go` - Add tests verifying `ctx.Env` is populated correctly

## Files to Create
None - all changes are modifications to existing files.

## Implementation Steps
- [x] Add `Env []string` field to `ExecutionContext` struct in `internal/actions/action.go`
- [x] Modify `SetupBuildEnvAction.Execute()` to populate `ctx.Env` using `buildAutotoolsEnv(ctx)`
- [ ] Update `ConfigureMakeAction.Execute()` to check for `ctx.Env` and use it if available, otherwise call `buildAutotoolsEnv(ctx)` directly
- [ ] Update `CMakeBuildAction.Execute()` to check for `ctx.Env` and use it if available, otherwise call `buildCMakeEnv()` directly
- [ ] Add unit tests in `setup_build_env_test.go` verifying that `ctx.Env` is populated with expected environment variables
- [ ] Add integration tests verifying that build actions can use environment from `setup_build_env`
- [ ] Run full test suite to ensure no regressions

## Testing Strategy
- **Unit tests**:
  - Verify `ctx.Env` is populated by `setup_build_env` with correct PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
  - Test with zero dependencies (should still set SOURCE_DATE_EPOCH and other base vars)
  - Test with multiple dependencies (should include all dependency paths)
  - Verify `configure_make` uses `ctx.Env` when set
  - Verify `cmake_build` uses `ctx.Env` when set

- **Integration tests**:
  - Create a test recipe that uses `setup_build_env` followed by `configure_make`
  - Verify the environment is shared between actions in the same execution context

- **Manual verification**:
  - Test with existing recipes (ncurses, curl) that don't use `setup_build_env` to ensure backward compatibility
  - Create a simple test recipe that explicitly uses `setup_build_env` to verify the new behavior works

## Risks and Mitigations
- **Risk**: Breaking existing recipes that don't use `setup_build_env`
  - **Mitigation**: Make environment checking optional - only use `ctx.Env` if it's non-empty. Build actions continue to call their environment builders if `ctx.Env` is not set.

- **Risk**: Environment pollution if `setup_build_env` is called multiple times
  - **Mitigation**: Document that `setup_build_env` replaces the environment (doesn't append). Consider adding a check to warn if `ctx.Env` is already set.

- **Risk**: `cmake_build` may need different environment than autotools builds
  - **Mitigation**: Initially, `cmake_build` will continue using `buildCMakeEnv()` by default. Only use `ctx.Env` if it's explicitly set by `setup_build_env`. This preserves current behavior while allowing opt-in to shared environment.

- **Risk**: Tests may need adjustment for new environment handling
  - **Mitigation**: Carefully review all configure_make and cmake_build tests. Add new tests for the environment sharing behavior.

## Success Criteria
- [ ] `ExecutionContext` has an `Env []string` field
- [ ] `setup_build_env` populates `ctx.Env` with environment variables from dependencies
- [ ] `configure_make` uses `ctx.Env` when set, falls back to `buildAutotoolsEnv()` otherwise
- [ ] `cmake_build` uses `ctx.Env` when set, falls back to `buildCMakeEnv()` otherwise
- [ ] All existing tests pass without modification
- [ ] New tests verify environment sharing works correctly
- [ ] Existing recipes (ncurses, curl) continue to work without changes
- [ ] Implementation matches design document expectations (DESIGN-dependency-provisioning.md lines 488-509)

## Open Questions
None - the implementation path is clear. The design document provides explicit guidance on the expected behavior.
