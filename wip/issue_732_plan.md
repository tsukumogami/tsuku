# Issue 732 Implementation Plan

## Summary
Fix the circular dependency false positive by checking if a tool is already installed before marking it as visited in the dependency resolution chain. This prevents shared dependencies from being incorrectly flagged as circular.

## Approach
The current implementation marks tools as visited before checking if they're already installed, causing false positives when a dependency is shared between multiple tools (e.g., `git-source` and `curl` both depend on `openssl`). The fix reorders the logic to check installation status first, only marking tools as visited when they actually need to be processed.

### Alternatives Considered
- **Fresh visited map for each subtree**: Similar to `ensurePackageManagersForRecipe` (line 180), we could create a fresh `visited` map for each dependency. However, this would prevent detection of actual circular dependencies within a subtree and increase complexity.
  - Why not chosen: Doesn't solve the core issue and could mask real circular dependencies

- **Track visited in two separate maps**: One for "currently in stack" (circular detection) and one for "already processed" (duplicate work prevention).
  - Why not chosen: More complex than necessary; the existing installation check already serves as the "already processed" signal

## Files to Modify
- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/cmd/tsuku/install_deps.go` - Reorder logic in `installWithDependencies` to check installation status before circular dependency detection

## Files to Create
None

## Implementation Steps
- [x] Move the "already installed" check (lines 251-291) to before the circular dependency check (lines 226-230)
- [x] For tools that are already installed and this is a dependency install (`!isExplicit && reqVersion == ""`), update state and return early WITHOUT marking as visited
- [x] Only mark tools as visited when they're about to be processed (after the early return for already-installed dependencies)
- [ ] Add a test case in `dependency_test.go` that reproduces the `git-source` scenario: a tool with multiple dependencies that share a common sub-dependency

## Testing Strategy
- Unit tests: Add `TestDependencyResolution_SharedDependency` in `dependency_test.go` that simulates the exact scenario:
  - Tool A depends on: B, C
  - Tool B depends on: C
  - Verify that C is only installed once and no circular dependency error is raised
- Integration tests: Run `tsuku install git-source` in CI to verify the fix works end-to-end
- Manual verification: Test locally with `./tsuku install git-source` and verify it completes without circular dependency errors

## Risks and Mitigations
- **Risk**: Moving the visited check might break actual circular dependency detection
  - **Mitigation**: The visited check still happens, just after confirming the tool isn't already installed. Actual circular dependencies (A → B → A) will still be caught because the second visit to A won't find it installed yet (it's mid-installation in the call stack)

- **Risk**: Edge case where an explicit install/update bypasses the early return (line 287-290) might not work correctly
  - **Mitigation**: The logic at lines 287-290 already handles this by proceeding with installation when `isExplicit` is true or `reqVersion` is specified. This behavior is preserved.

## Success Criteria
- [ ] `tsuku install git-source` completes successfully without circular dependency errors
- [ ] New unit test `TestDependencyResolution_SharedDependency` passes
- [ ] All existing tests in `dependency_test.go` continue to pass
- [ ] `TestCircularDependencyDetection` still correctly detects actual circular dependencies
- [ ] CI Build Essentials workflow passes with `git-source` test re-enabled

## Open Questions
None - the fix is straightforward and well-scoped.
