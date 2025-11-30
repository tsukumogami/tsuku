# Issue 44 Implementation Plan

## Summary

Enhance recipe management UX by: (1) adding source indicators to `tsuku recipes`, (2) adding `--local` flag, (3) adding `--force` flag to `tsuku create`, and (4) improving error messages when a tool is not found.

## Approach

The implementation adds new methods to the recipe Loader to enumerate recipes from both local and registry sources, and modifies the CLI commands to expose this information. Error messages are enhanced with contextual suggestions.

### Alternatives Considered

- **Separate commands for local vs registry recipes**: Rejected as it fragments the UX; a single `tsuku recipes` with filtering is more discoverable.
- **Automatic ecosystem detection in error messages**: Deferred to future LLM integration; for now, list all available ecosystems.

## Files to Modify

- `internal/recipe/loader.go` - Add methods to list recipes with source information
- `cmd/tsuku/recipes.go` - Add `--local` flag and source indicator display
- `cmd/tsuku/create.go` - The `--force` flag already exists; verify functionality
- `cmd/tsuku/install.go` - Enhance error message when tool not found

## Files to Create

- `internal/recipe/loader_test.go` - Tests for new listing functionality (if not exists, extend)

## Implementation Steps

- [x] Add `ListAllWithSource()` method to Loader that returns recipes from both local and registry with source indicator
- [x] Add `ListLocal()` method to Loader for local-only listing
- [x] Update `tsuku recipes` command to show source indicator (local/registry)
- [x] Add `--local` flag to `tsuku recipes` command
- [x] Verify `--force` flag works correctly on `tsuku create`
- [x] Enhance error message in `tsuku install` when tool not found
- [x] Add tests for new loader methods
- [x] Add tests for CLI flag behavior

## Testing Strategy

- Unit tests: Test `ListAllWithSource()` and `ListLocal()` methods with mock filesystem
- Integration tests: Test CLI commands with actual recipe files
- Manual verification: Run commands and verify output format

## Risks and Mitigations

- **Performance with large recipe directories**: Mitigate by lazy loading and not parsing TOML unless needed
- **Registry directory structure varies**: Handle both flat and letter-prefixed structures

## Success Criteria

- [x] `tsuku recipes` shows source indicator (local/registry) for each recipe
- [x] `tsuku recipes --local` shows only recipes in `$TSUKU_HOME/recipes/`
- [x] `tsuku create bat --from crates.io --force` overwrites existing recipe
- [x] Error when tool not found shows: "To create a recipe: tsuku create <tool> --from <ecosystem>"
- [x] Error lists available ecosystems (crates.io, rubygems, pypi, npm)

## Open Questions

None - requirements are clear from issue and design doc.
