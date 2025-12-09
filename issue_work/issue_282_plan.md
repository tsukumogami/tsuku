# Issue 282 Implementation Plan

## Summary

Extend the `--from` flag to support `builder:sourceArg` format alongside existing ecosystem names, enabling the GitHub Release Builder to be invoked via `tsuku create gh --from github:cli/cli`.

## Approach

Modify the `--from` flag parsing to detect if the value contains a colon. If it does, parse as `builder:sourceArg`; otherwise, treat as an ecosystem name (existing behavior). This preserves backward compatibility with existing usage like `--from crates.io`.

### Alternatives Considered

- **Separate `--source-arg` flag**: More explicit but requires two flags for github builder. Rejected for usability reasons.
- **Only support new format**: Would break existing `--from crates.io` usage. Rejected for compatibility.

## Files to Modify

- `cmd/tsuku/create.go` - Add parsing logic for `builder:sourceArg` format, register GitHub builder, pass SourceArg to build request

## Files to Create

None - all functionality goes in existing files.

## Implementation Steps

- [x] Add parseFromFlag function to split `builder:sourceArg` format
- [x] Register GitHubReleaseBuilder in the builder registry
- [x] Update runCreate to handle both formats and pass SourceArg when present
- [x] Skip toolchain check for github builder (no toolchain required)
- [x] Skip CanBuild check for github builder (requires SourceArg instead of package name)
- [x] Update help text and examples to show new syntax
- [x] Add unit tests for parseFromFlag function
- [x] Integration tests deferred to #283 (ground truth validation)

## Testing Strategy

- Unit tests: Test `parseFromFlag` with various inputs (old format, new format, edge cases)
- Integration tests: Mock HTTP server for GitHub API and LLM client, verify correct recipe generation

## Risks and Mitigations

- **Risk**: Existing users may accidentally use colon in ecosystem name
  - **Mitigation**: Document format clearly, validate builder exists before proceeding
- **Risk**: GitHub builder requires ANTHROPIC_API_KEY which might not be set
  - **Mitigation**: Clear error message when key missing

## Success Criteria

- [ ] `tsuku create ripgrep --from crates.io` still works (backward compatibility)
- [ ] `tsuku create gh --from github:cli/cli` invokes GitHub builder with correct SourceArg
- [ ] Recipe TOML printed to stdout
- [ ] Cost/warnings printed to stderr
- [ ] Help text shows both formats
- [ ] Unit tests pass for new parsing logic

## Open Questions

None - the design doc and issue provide clear guidance.
