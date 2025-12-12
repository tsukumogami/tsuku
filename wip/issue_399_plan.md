# Issue 399 Implementation Plan

## Summary

Split `internal/install/state.go` (613 lines) into four focused files by extracting domain-specific operations while keeping core types and StateManager in the original file.

## Approach

The file has clear domain boundaries - tool operations, library operations, and LLM usage tracking. Each domain will be extracted to its own file while keeping the core State type and StateManager in state.go. This follows the same pattern used for previous splits (resolver.go, install.go).

### Alternatives Considered
- **Single file refactoring**: Keep everything in one file but reorganize with better comments - rejected because the issue specifically requires file splitting for merge conflict reduction
- **Interface-based separation**: Extract interfaces for each domain - rejected as over-engineering for internal package

## Files to Modify
- `internal/install/state.go` - Remove extracted code, keep core types and StateManager

## Files to Create
- `internal/install/state_tool.go` - Tool state operations
- `internal/install/state_lib.go` - Library state operations
- `internal/install/state_llm.go` - LLM usage tracking

## Implementation Steps
- [x] Create `state_tool.go` with tool operations
- [x] Create `state_lib.go` with library operations
- [x] Create `state_llm.go` with LLM usage operations
- [x] Update `state.go` to remove extracted code
- [x] Verify build and tests pass
- [ ] Run golangci-lint (CI will run this)

## Testing Strategy
- Unit tests: All existing tests in `internal/install/state_test.go` should pass unchanged
- Build verification: `go build ./...` must succeed
- Lint verification: `golangci-lint run --timeout=5m`

## Risks and Mitigations
- **Circular dependencies**: Keep all types in state.go to avoid import cycles between split files
- **Test coverage**: No new code is being added, just reorganized - existing tests cover all functionality

## Success Criteria
- [ ] `state.go` contains only core types and StateManager (~240 lines)
- [ ] `state_tool.go` contains tool operations
- [ ] `state_lib.go` contains library operations
- [ ] `state_llm.go` contains LLM usage operations
- [ ] All existing tests pass
- [ ] Build succeeds
- [ ] Lint passes

## File Organization

### state.go (core, ~240 lines)
- Types: `State`, `StateManager`
- Types: `VersionState`, `ToolState`, `LibraryVersionState`, `LLMUsage` (kept here to avoid circular deps)
- Methods: `NewStateManager`, `statePath`, `lockPath`
- Methods: `Load`, `Save`, `loadWithLock`, `saveWithLock`
- Methods: `loadWithoutLock`, `saveWithoutLock`
- Function: `ValidateVersionString`

### state_tool.go (~95 lines)
- Methods: `UpdateTool`, `RemoveTool`
- Methods: `AddRequiredBy`, `RemoveRequiredBy`
- Methods: `GetToolState`
- Methods: `migrateToMultiVersion` (on State type)

### state_lib.go (~90 lines)
- Methods: `UpdateLibrary`
- Methods: `AddLibraryUsedBy`, `RemoveLibraryUsedBy`
- Methods: `RemoveLibraryVersion`
- Methods: `GetLibraryState`

### state_llm.go (~130 lines)
- Methods: `RecordGeneration`
- Methods: `CanGenerate`
- Methods: `DailySpent`
- Methods: `RecentGenerationCount`
