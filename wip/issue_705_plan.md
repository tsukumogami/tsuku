# Issue 705 Implementation Plan

## Summary

Extend the `tsuku info` command with two new flags: `--recipe <path>` for loading local recipe files (following the pattern from `eval` and `install` commands) and `--metadata-only` for skipping dependency resolution to enable fast static queries. The existing JSON output schema will be expanded additively with static recipe properties while maintaining full backward compatibility.

## Approach

This implementation follows the established patterns in `eval.go` and `install.go` for the `--recipe` flag, and builds on the existing `info.go` structure by adding conditional logic paths. The approach prioritizes backward compatibility by making the new flags opt-in and ensuring the default behavior remains unchanged.

The key insight is that dependency resolution (via `actions.ResolveDependencies()` and `actions.ResolveTransitive()`) is expensive and unnecessary for queries that only need static recipe metadata. By making this optional via `--metadata-only`, we enable fast automation workflows (e.g., golden plan testing) that need to query hundreds of recipes without network calls or graph traversal.

### Alternatives Considered

**Alternative 1: Create a new `tsuku metadata` command**
- Pro: Clean separation of concerns, no flag complexity
- Con: Breaks symmetry with other commands, adds new command to learn
- Con: Duplicates recipe loading logic, increases maintenance burden
- Rejected because extending existing commands is the established pattern (see `--recipe` in eval/install)

**Alternative 2: Add only `--metadata-only` flag without `--recipe` support**
- Pro: Simpler implementation, fewer flag combinations
- Con: Cannot test uncommitted local recipe files (primary use case from issue)
- Con: Breaks symmetry with eval/install which support `--recipe`
- Rejected because golden plan testing specifically requires local recipe file support

**Alternative 3: Always perform dependency resolution but add new static fields**
- Pro: Simpler code path (no conditional logic)
- Con: Doesn't solve performance problem (dependency resolution remains slow)
- Con: Requires network access even for static queries
- Rejected because it fails to address the core performance motivation

## Files to Modify

- `/Users/danielgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/info.go` - Add flags, modify command logic to support both local file and registry loading, add conditional dependency resolution, expand JSON output schema with static metadata fields

## Files to Create

None (extending existing command only)

## Implementation Steps

- [ ] Add flag definitions to `info.go` init function: `--recipe string` and `--metadata-only bool`
- [ ] Add mutual exclusivity validation at start of Run function: tool name XOR `--recipe`
- [ ] Add conditional recipe loading logic: if `--recipe` use `loadLocalRecipe()`, else use `loader.Get()`
- [ ] Wrap dependency resolution in conditional: skip if `--metadata-only` flag is set
- [ ] Wrap installation state check in conditional: skip if `--metadata-only` flag is set
- [ ] Expand JSON output struct with new static fields: `version_source`, `version_format`, `platform_constraints`, `supported_platforms`, `tier`, `type`, `verification`, `steps`
- [ ] Add platform computation: call `r.GetSupportedPlatforms()` and include in output
- [ ] Update JSON output field tags to use `omitempty` for fields that may not be present in `--metadata-only` mode
- [ ] Add human-readable output formatting for new static metadata fields
- [ ] Update command usage documentation: reflect new flags and optional tool name argument

## Testing Strategy

**Unit tests:**
- Test flag validation (mutual exclusivity of tool name and `--recipe`)
- Test JSON schema marshaling with new static fields
- Test `--metadata-only` mode skips dependency resolution
- Test backward compatibility (existing `tsuku info <tool>` behavior unchanged)

**Integration tests:**
- Test `--recipe` with local file (success case)
- Test `--recipe` with missing file (error handling)
- Test `--recipe` with invalid TOML (parse error handling)
- Test `--metadata-only` with registry tool (fast path)
- Test `--metadata-only` with `--recipe` (combined flags)
- Test JSON output contains all expected fields
- Test human-readable output formatting
- Test platform computation edge cases (empty platforms array)

**Manual verification:**
- Compare JSON output before/after changes (confirm additive only)
- Verify performance improvement with `--metadata-only` (time command)
- Test with real recipe files from registry
- Verify existing scripts using `tsuku info <tool> --json` still work

## Risks and Mitigations

**Risk: Breaking existing JSON consumers**
- Mitigation: Use additive-only schema expansion (new fields don't break existing parsers)
- Mitigation: Add backward compatibility tests to CI
- Mitigation: Test with existing automation scripts if available

**Risk: Flag interaction complexity (--recipe, --metadata-only, --json)**
- Mitigation: Clear validation at function entry with helpful error messages
- Mitigation: Document all flag combinations in help text
- Mitigation: Comprehensive test matrix covering all combinations

**Risk: Platform computation errors with malformed recipes**
- Mitigation: `GetSupportedPlatforms()` already has test coverage (platform_test.go)
- Mitigation: Handle edge case of empty platform array (valid output, not error)
- Mitigation: Recipe validation during parse catches most malformed constraints

**Risk: Inconsistent behavior between registry and `--recipe` modes**
- Mitigation: Use same Recipe struct in both paths
- Mitigation: Only difference is loading mechanism (ParseFile vs loader.Get)
- Mitigation: Integration tests verify both paths produce equivalent output

**Risk: `--metadata-only` output differs too much from default mode**
- Mitigation: Keep schema identical, only omit installation state fields
- Mitigation: Static metadata fields always present (regardless of flag)
- Mitigation: Document field presence rules clearly

## Success Criteria

- [ ] `tsuku info <tool>` works exactly as before (backward compatibility)
- [ ] `tsuku info --recipe <path>` loads and displays metadata from local file
- [ ] `tsuku info <tool> --metadata-only` skips dependency resolution (measurably faster)
- [ ] JSON output includes all new static fields (version_source, platforms, steps, etc.)
- [ ] `--recipe` and `--metadata-only` can be combined
- [ ] Platform computation produces correct "os/arch" tuple arrays
- [ ] Error messages are clear for invalid flag combinations
- [ ] All existing `info` tests pass without modification
- [ ] New integration tests cover both registry and file modes
- [ ] Go vet, go test, and golangci-lint pass
- [ ] Documentation updated (command help text reflects new flags)

## Open Questions

None - design document (DESIGN-info-enhancements.md) provides complete specification for all implementation details.
