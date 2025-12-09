# Issue 300 Implementation Plan

## Summary

Update `tsuku list` to show all installed versions (one line per version) with an `(active)` indicator for the currently active version of each tool, sorted by tool name then version.

## Approach

Modify the list implementation to iterate over all installed versions from state.json's `Versions` map instead of showing one line per tool. This leverages the multi-version state schema introduced in #294.

### Alternatives Considered

- **Use directory scanning only**: Could scan tool directories for versions, but this would miss state metadata (like active version indicator). Not chosen because we need state data for active indicator.
- **Show grouped output**: Could group versions under each tool name, but the design doc specifies "one line per installed version" with a flat list format. Not chosen to follow design spec.

## Files to Modify

- `internal/install/list.go` - Update `ListWithOptions()` to iterate over all versions, add `IsActive` field to `InstalledTool`
- `cmd/tsuku/list.go` - Update output formatting to show `(active)` indicator

## Files to Create

- `internal/install/list_test.go` - Tests for multi-version list behavior

## Implementation Steps

- [x] Add `IsActive` field to `InstalledTool` struct
- [x] Update `ListWithOptions()` to iterate over all versions from state
- [x] Ensure output is sorted by tool name, then by version
- [x] Update `cmd/tsuku/list.go` to display `(active)` indicator
- [x] Update JSON output format to include `is_active` field
- [x] Write tests for multi-version list output

## Testing Strategy

- Unit tests: Test `ListWithOptions()` returns all versions with correct active indicators
- Unit tests: Test sorting (by tool name, then version)
- Unit tests: Test hidden tool filtering still works
- Manual verification: Run `tsuku list` with multi-version tools

## Risks and Mitigations

- **Backward compatibility**: The output format changes from one line per tool to one line per version. Users may have scripts parsing the output. Mitigation: The JSON output flag provides stable machine-readable format.

## Success Criteria

- [x] `tsuku list` shows one line per installed version
- [x] Active version is marked with `(active)` indicator
- [x] Output is sorted by tool name, then by version
- [x] JSON output includes `is_active` field
- [x] Tests pass for multi-version scenarios

## Open Questions

None - requirements are clear from design doc.
