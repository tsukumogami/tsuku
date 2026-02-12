# Issue 1629 Introspection

## Context Reviewed

- **Design doc**: `docs/designs/DESIGN-local-llm-runtime.md` (status: Planned)
- **Sibling issues reviewed**: #1628 (walking skeleton, closed)
- **Prior patterns identified**:
  - `internal/llm/addon/manager.go` - stub created with `AddonPath()` and `IsInstalled()` functions
  - Addon stored at `$TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm` (issue spec) vs `$TSUKU_HOME/tools/tsuku-llm/tsuku-llm` (current implementation)
  - Platform detection pattern already established (runtime.GOOS, runtime.GOARCH)
  - Lock file and socket path patterns in `lifecycle.go`
  - Factory integration complete with `WithLocalEnabled()` option

## Gap Analysis

### Minor Gaps

1. **Version directory structure**: The issue specifies addon stored at `$TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm` but the skeleton implementation in `manager.go` uses `$TSUKU_HOME/tools/tsuku-llm/tsuku-llm` (no version subdirectory). This should be reconciled - the version subdirectory is likely the correct design to support version upgrades.

2. **AddonManager struct vs functions**: Issue specifies `AddonManager struct` with `EnsureAddon()` method, but skeleton implements standalone functions (`AddonPath()`, `IsInstalled()`). Need to refactor to struct-based approach to add download logic, checksum verification, and state management.

3. **Progress hook points**: Issue notes downstream dependency (#1642) needs "hook points for progress tracking" in `EnsureAddon()`. The implementation should include callback parameters for download progress reporting.

4. **Manifest location**: Issue mentions "manifest embedded in tsuku" but doesn't specify the embed mechanism or file format. Based on design doc, this should be a JSON manifest embedded via Go's `//go:embed` directive.

### Moderate Gaps

None identified. The issue spec is reasonably complete and aligns with the design document.

### Major Gaps

None identified. The walking skeleton established clean patterns that this issue can build on.

## Recommendation

**Proceed**

The issue spec is complete enough to implement. The minor gaps are resolvable from the design document and prior work without requiring user input:

1. Version subdirectory should be added per design doc
2. Convert to struct-based AddonManager to match issue spec
3. Include progress callback parameter for downstream compatibility
4. Use `//go:embed` for manifest per design doc patterns

## Proposed Amendments

No amendments needed - proceed with implementation using patterns from #1628 and clarifications from design doc.

## Implementation Notes

Key patterns from #1628 to follow:

- **Path functions**: Use `$TSUKU_HOME` environment variable with fallback to `~/.tsuku`
- **Platform detection**: Use `runtime.GOOS` and `runtime.GOARCH`
- **Package structure**: Keep addon logic in `internal/llm/addon/` subpackage
- **Test patterns**: Use `t.TempDir()` and `t.Setenv()` for isolation
- **Error handling**: Return wrapped errors with context (per `lifecycle.go` patterns)

Files to modify:
- `internal/llm/addon/manager.go` - Expand from stub to full implementation
- `internal/llm/addon/manager_test.go` - Add tests for download and verification
- New file: `internal/llm/addon/manifest.go` - Embedded manifest with platform URLs and checksums
