# Issue 240 Implementation Plan

## Summary

Enhance `tsuku info <tool>` to display the dependency tree by reading from state.json for installed tools or resolving from recipe for uninstalled tools.

## Approach

Add dependency tree display to the existing `info.go` command. For installed tools, read `InstallDependencies` and `RuntimeDependencies` from state.json. For uninstalled tools, use `actions.ResolveDependencies()` to compute them from the recipe. Display with clear section headers and indentation for transitive deps.

### Alternatives Considered
- Separate command (`tsuku deps <tool>`): Not chosen because dependency info fits naturally in `info` and issue explicitly says "tsuku info shows..."
- Tree visualization library: Not chosen - simple text indentation is sufficient and avoids new dependencies

## Files to Modify
- `cmd/tsuku/info.go` - Add dependency tree display to both text and JSON output

## Files to Create
None - all changes fit in existing file

## Implementation Steps
- [x] Add dependency tree display for installed tools (from state.json)
- [x] Add dependency tree display for uninstalled tools (from recipe resolution)
- [x] Add transitive dependency resolution for uninstalled tools
- [x] Update JSON output to include dependencies
- [x] Tests pass via existing test infrastructure

## Testing Strategy
- Unit tests: Test helper functions for formatting dependency output
- Manual verification: `./tsuku info turbo` (npm tool with nodejs dep), `./tsuku info lazygit` (go tool with no runtime deps)

## Risks and Mitigations
- **Recipe loading for transitive deps may be slow**: Mitigated by only resolving for uninstalled tools where we already load the recipe
- **Stale state.json data**: Acceptable - state reflects what was recorded at install time

## Success Criteria
- [x] `tsuku info` shows install dependencies with indentation
- [x] `tsuku info` shows runtime dependencies separately
- [x] Transitive deps shown with additional indentation (for uninstalled tools)
- [x] Works for installed tools (reads state.json)
- [x] Works for uninstalled tools (resolves from recipe)

## Open Questions
None
