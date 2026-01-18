# Issue 990 Implementation Plan

## Summary

Add Tier 2 dependency validation to the verify command by integrating `ValidateDependenciesSimple()` into `verifyTool()` and `verifyLibrary()`, with a new `displayDependencyResults()` helper for consistent output formatting.

## Approach

Use the simplified wrapper `ValidateDependenciesSimple(binaryPath, state, tsukuHome)` from #989 to minimize integration complexity. For tools, iterate over installed binaries; for libraries, reuse `findLibraryFiles()` results. Display results using the categorized format specified in the issue (tsuku:name@version, system, external).

### Alternatives Considered

- **Full ValidateDependencies with recipe loader**: Would enable EXTERNALLY_MANAGED refinement but adds complexity. The simplified wrapper with nil loader is sufficient for initial integration and treats all tsuku-managed deps consistently.
- **Single call per install directory**: Would require ValidateDependencies to iterate files internally, breaking its current single-binary contract. Iterating in the command layer maintains clean separation.

## Files to Modify

- `cmd/tsuku/verify.go` - Add `displayDependencyResults()` helper, update `verifyVisibleTool()` to add Step 5, update `verifyWithAbsolutePath()` to add dependency check, update `verifyLibrary()` to add Tier 2 after Tier 1

## Files to Create

None.

## Implementation Steps

- [ ] Add `displayDependencyResults(results []verify.DepResult)` helper function in `cmd/tsuku/verify.go`
  - Format tsuku-managed deps as "OK (tsuku:recipe@version)"
  - Format externally-managed deps as "OK (tsuku:recipe@version, external)"
  - Format system deps as "OK (system)"
  - Handle failed/unknown deps with error messages
  - Print summary line with count

- [ ] Add `findToolBinaries(installDir string, toolState *install.ToolState, versionState *install.VersionState) []string` helper
  - Return absolute paths to actual binary files in install directory
  - Use `toolState.Binaries` or `versionState.Binaries` to locate files
  - Handle bin/ subdirectory layout

- [ ] Update `verifyVisibleTool()` to add Step 5 for dependency validation
  - Add after Step 4 (binary integrity)
  - Iterate over tool binaries and call `ValidateDependenciesSimple()` for each
  - Collect all results and pass to `displayDependencyResults()`
  - Exit with `ExitVerifyFailed` if any dependency fails
  - Handle static binaries (empty results) with "No dynamic dependencies" message

- [ ] Update `verifyWithAbsolutePath()` to add dependency validation for hidden tools
  - Add after binary integrity check
  - Same logic as visible tools but without step numbering

- [ ] Update `verifyLibrary()` to add Tier 2 after Tier 1 header validation
  - Add between Tier 1 and Tier 4 (integrity)
  - Reuse `libFiles` from `findLibraryFiles()` call
  - For each library file, call `ValidateDependenciesSimple()`
  - Collect results and pass to `displayDependencyResults()`
  - Return error if any dependency fails

- [ ] Add import for `config` package (if not already imported for tsukuHome access)

- [ ] Run `go test ./cmd/tsuku/...` to verify no regressions

- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` before commit

## Testing Strategy

- **Unit tests**: Not required for this issue since we're integrating existing tested functionality (`ValidateDependenciesSimple` from #989)
- **Manual verification**:
  - `tsuku verify <tool>` on a tool with known dependencies - verify output format matches spec
  - `tsuku verify <library>` on a library with dependencies - verify Tier 2 section appears
  - Verify static binaries report "No dynamic dependencies"
  - Verify tools with missing deps would fail (if such a state can be induced)
- **Integration tests**: Covered by #991 (follow-on issue)

## Risks and Mitigations

- **Performance with many binaries**: Each binary triggers a full dependency walk. Mitigation: The visited map in ValidateDependencies provides cycle detection and avoids redundant work for shared transitive deps.
- **Output format for failed deps**: Issue spec only shows passing deps. Mitigation: Include error message inline with FAIL status label.
- **Empty binary list edge case**: Some tools might not have recorded binaries. Mitigation: Fall back to tool name as binary, same as Step 3 currently does.

## Success Criteria

- [ ] `verifyTool()` calls `ValidateDependenciesSimple()` after Tier 1 completes (both visible and hidden tools)
- [ ] `verifyLibrary()` calls `ValidateDependenciesSimple()` after Tier 1 header validation
- [ ] Output matches the specified format with category labels (tsuku, external, system)
- [ ] Recipe name and version displayed for tsuku-managed dependencies (using `.Recipe` field)
- [ ] Summary line shows total count of validated dependencies
- [ ] Validation failures result in `ExitVerifyFailed` exit code
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run --timeout=5m ./...` passes

## Open Questions

None. The introspection identified all field naming differences (.Recipe vs .Provider) and the correct function wrapper to use.
