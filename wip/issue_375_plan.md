# Issue 375 Implementation Plan

## Summary

Implement a hybrid recipe preview flow in `cmd/tsuku/create.go` that displays a summary of the generated recipe with options to view full TOML, install, or cancel.

## Approach

Add preview functionality directly in `create.go` after recipe generation. The preview shows a concise summary of the recipe (downloads, actions, verification, cost) and provides an interactive prompt with three options: `v` to view full TOML, `i` to install, `c` to cancel.

### Alternatives Considered

1. **Preview as separate package**: Extract preview logic to `internal/preview/` package.
   - Not chosen: Adds unnecessary abstraction for UI code that's specific to the create command.

2. **Preview before generation**: Show preview of what will be generated.
   - Not chosen: We need to show the actual generated recipe, not a prediction.

## Files to Modify

- `cmd/tsuku/create.go` - Add preview flow after recipe generation for GitHub builder
- `internal/builders/builder.go` - Add `Cost` field to `BuildResult` for direct cost access

## Files to Create

None.

## Implementation Steps

- [x] 1. Add `Cost` field to `BuildResult` struct in `internal/builders/builder.go`
- [x] 2. Update `GitHubReleaseBuilder.Build()` to populate `Cost` field
- [x] 3. Add helper functions for preview display in `create.go`:
  - `previewRecipe()` - displays summary and handles prompt loop
  - `promptForApproval()` - handles user input (v/i/c)
  - `extractDownloadURLs()` - extracts download URLs from recipe
  - `describeStep()` - returns human-readable description of a recipe step
  - `formatRecipeTOML()` - formats recipe as TOML string for display
- [x] 4. Integrate preview flow into `runCreate()` for GitHub builder path
- [x] 5. Add unit tests for preview helper functions
- [x] 6. Run tests and lint checks

## Testing Strategy

- Unit tests: Test `describeStep()`, `extractDownloadURLs()` with various recipe configurations
- Manual verification: Run `tsuku create` with mock GitHub builder to test interactive flow

## Risks and Mitigations

- **Risk**: Interactive prompts don't work in non-TTY environments (CI, scripts)
  - **Mitigation**: This feature is for GitHub builder which requires user interaction anyway; future `--yes` flag (issue #374) will bypass preview

## Success Criteria

- [ ] Summary shows downloads, actions, verification command
- [ ] Summary shows LLM provider and cost
- [ ] Summary shows validation warnings if applicable
- [ ] `v` displays full TOML recipe
- [ ] `i` proceeds to installation (writes recipe to file and prints install instructions)
- [ ] `c` or Enter cancels
- [ ] Invalid input re-prompts
- [ ] All existing tests pass
- [ ] golangci-lint passes

## Open Questions

None - requirements are clear from the issue and design doc.
