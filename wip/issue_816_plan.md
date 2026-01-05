# Issue 816 Implementation Plan

## Summary

Fix macOS test failures by canonicalizing the tool directory path in `isWithinDir` before comparing with the symlink-resolved binary path, ensuring `/var/folders/...` and `/private/var/folders/...` are recognized as equivalent.

## Approach

The fix is applied in the production code (`checksum.go`) rather than the tests. Before comparing the resolved binary path against the tool directory, we canonicalize the tool directory using `filepath.EvalSymlinks`. This ensures both paths are in their fully resolved form, making the comparison work correctly on macOS where `/var` is a symlink to `/private/var`.

### Alternatives Considered

1. **Canonicalize in the test setup**: Could use `filepath.EvalSymlinks(t.TempDir())` in tests to get the real path upfront.
   - **Why not chosen**: This only fixes tests, not the actual production issue. A user on macOS with `TSUKU_HOME` in a symlinked directory would hit the same problem.

2. **Canonicalize only the tool directory in `isWithinDir`**: Add `filepath.EvalSymlinks(dir)` inside `isWithinDir`.
   - **Why not chosen**: `isWithinDir` expects already-clean paths per its documentation. Adding EvalSymlinks there could mask other issues and has different error semantics (EvalSymlinks can fail).

3. **Canonicalize both paths at call site in `ComputeBinaryChecksums`**: Resolve `toolDir` once before the loop, then pass both resolved paths to `isWithinDir`.
   - **Chosen**: Clean separation of concerns - path resolution happens explicitly in the calling code, `isWithinDir` remains a pure path comparison utility.

## Files to Modify

- `internal/install/checksum.go` - Canonicalize `toolDir` using `filepath.EvalSymlinks` before calling `isWithinDir`

## Files to Create

None

## Implementation Steps

- [ ] Canonicalize `toolDir` in `ComputeBinaryChecksums` before the loop using `filepath.EvalSymlinks`
- [ ] Add error handling for the case where `toolDir` cannot be resolved
- [ ] Run tests to verify the 4 failing tests now pass on macOS
- [ ] Run full test suite to ensure no regressions

## Testing Strategy

- **Unit tests**: Run `go test -v ./internal/install/...` on macOS to verify the 4 failing tests pass:
  - `TestComputeBinaryChecksums`
  - `TestComputeBinaryChecksums_WithSymlink`
  - `TestVerifyBinaryChecksums_AllMatch`
  - `TestVerifyBinaryChecksums_Mismatch`
- **Manual verification**: The existing tests already cover the symlink resolution scenarios; no new tests needed
- **Regression check**: Run `go test ./...` to ensure no other tests break

## Risks and Mitigations

- **Risk**: `filepath.EvalSymlinks(toolDir)` could fail if `toolDir` doesn't exist yet
  - **Mitigation**: This is called after the binary paths are resolved, so `toolDir` must exist. If it fails, the error is propagated appropriately.

- **Risk**: Performance impact from additional syscall
  - **Mitigation**: Single EvalSymlinks call per ComputeBinaryChecksums invocation (outside the loop), negligible impact.

## Success Criteria

- [ ] All 4 tests from issue #816 pass on macOS
- [ ] Full test suite passes (`go test ./...`)
- [ ] Build succeeds (`go build ./cmd/tsuku`)
- [ ] `go vet ./...` passes

## Open Questions

None - the approach is straightforward and aligned with the suggested fix in the issue.
