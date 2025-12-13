# Issue 438 Implementation Plan

## Summary

Implement `Decompose()` method for `GitHubArchiveAction` to resolve assets and return primitive steps at evaluation time, enabling deterministic plan generation.

## Approach

Add a `Downloader` field to `EvalContext` for checksum computation, then implement `Decompose()` on `GitHubArchiveAction` that:
1. Resolves wildcard asset patterns via GitHub API
2. Constructs the download URL
3. Downloads the file to compute checksum
4. Returns primitives: download, extract, chmod, install_binaries

### Alternatives Considered

- **Lazy checksum computation**: Defer checksum to installation time. Rejected because it defeats determinism.
- **Separate decomposition context**: Create a new context type. Rejected - extending `EvalContext` is cleaner.

## Files to Modify

- `internal/actions/decomposable.go` - Add `Downloader` field to `EvalContext`
- `internal/actions/composites.go` - Add `Decompose()` method to `GitHubArchiveAction`
- `internal/actions/decomposable_test.go` - Add test for `EvalContext.Downloader`
- `internal/actions/composites_test.go` - Add tests for `GitHubArchiveAction.Decompose()`

## Files to Create

None

## Implementation Steps

- [ ] Add `Downloader` field to `EvalContext` in `decomposable.go`
- [ ] Add `EvalContext.Downloader` test in `decomposable_test.go`
- [ ] Implement `Decompose()` method on `GitHubArchiveAction`
- [ ] Add unit tests for `GitHubArchiveAction.Decompose()`
- [ ] Run verification (go vet, go test, go build)

## Testing Strategy

- Unit tests: Test decomposition with mock resolver and downloader
- Test wildcard pattern resolution
- Test that returned steps are primitives (download, extract, chmod, install_binaries)
- Test checksum and size are captured correctly
- Test error cases (missing params, API errors)

## Risks and Mitigations

- **Import cycle with validate package**: EvalContext needs PreDownloader from validate. Check for cycles. If needed, define an interface instead of concrete type.

## Success Criteria

- [ ] `GitHubArchiveAction` implements `Decomposable` interface
- [ ] Asset pattern resolution happens during decomposition
- [ ] Checksum computed via pre-download during decomposition
- [ ] Returns primitives: download, extract, chmod, install_binaries
- [ ] All tests pass, no lint errors

## Open Questions

None.
