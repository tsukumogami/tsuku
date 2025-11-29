# Issue 47 Implementation Plan

## Summary

Split `cmd/tsuku/main.go` (1102 lines) into separate files per command, keeping shared helpers in a dedicated file and main.go minimal.

## Approach

Organize by responsibility: each command gets its own file containing the cobra.Command definition and any command-specific helper functions. Shared utilities and the global `loader` variable stay in common files.

### Alternatives Considered

- **Group related commands**: Could group install/update/remove together. Rejected because issue requests one file per command, and single-purpose files are easier to navigate.
- **Internal package**: Could move commands to internal/cli/. Rejected because this adds complexity and the issue just asks for file splitting within cmd/tsuku/.

## Files to Modify

- `cmd/tsuku/main.go` - Keep only root command, init, and main()

## Files to Create

- `cmd/tsuku/install.go` - install command + installWithDependencies, ensurePackageManagersForRecipe, runInstall
- `cmd/tsuku/list.go` - list command
- `cmd/tsuku/update.go` - update command
- `cmd/tsuku/versions.go` - versions command
- `cmd/tsuku/search.go` - search command
- `cmd/tsuku/info.go` - info command
- `cmd/tsuku/outdated.go` - outdated command
- `cmd/tsuku/remove.go` - remove command + cleanupOrphans
- `cmd/tsuku/recipes.go` - recipes command
- `cmd/tsuku/update_registry.go` - update-registry command
- `cmd/tsuku/verify.go` - verify command + verifyWithAbsolutePath, verifyVisibleTool
- `cmd/tsuku/helpers.go` - shared loader variable and initialization

## Implementation Steps

- [x] Create helpers.go with loader variable and GetLoader function
- [x] Create install.go with install command and related functions
- [x] Create list.go with list command
- [x] Create update.go with update command
- [x] Create versions.go with versions command
- [x] Create search.go with search command
- [x] Create info.go with info command
- [x] Create outdated.go with outdated command
- [x] Create remove.go with remove command and cleanupOrphans
- [x] Create recipes.go with recipes command
- [x] Create update_registry.go with update-registry command
- [x] Create verify.go with verify command and helpers
- [x] Refactor main.go to ~100 lines (root command, init, main)
- [x] Run tests and verify build

## Testing Strategy

- Unit tests: Existing `cmd/tsuku/main_test.go` tests should pass unchanged
- Build verification: `go build ./...` must succeed
- Manual verification: Run `tsuku --help` and a few commands to verify functionality

## Risks and Mitigations

- **Import cycle**: Could occur if files depend on each other incorrectly. Mitigation: Keep dependencies one-way (commands import helpers, not vice versa).
- **Init order**: Go's init() in multiple files has undefined order. Mitigation: Use explicit initialization in a single init() in main.go.

## Success Criteria

- [x] main.go is ~100 lines or less (58 lines)
- [x] Each command is in its own file
- [x] All existing tests pass
- [x] Build succeeds
- [x] No functional changes

## Open Questions

None - straightforward refactoring task.
