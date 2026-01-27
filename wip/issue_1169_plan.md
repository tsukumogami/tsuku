# Issue 1169 Implementation Plan

## Summary

Add two additional edge case tests (symlink chains and broken symlinks) to strengthen integrity verification coverage beyond the core acceptance criteria already met by existing tests.

## Approach

The existing test suite created in issue #1168 already covers all 7 acceptance criteria items with dedicated tests:
- `TestVerifyIntegrity_AllMatch` - Normal file verification
- `TestVerifyIntegrity_Mismatch` - Mismatch detection
- `TestVerifyIntegrity_MissingFile` - Missing file handling
- `TestVerifyIntegrity_EmptyChecksums` - Empty checksums map
- `TestVerifyIntegrity_NilChecksums` - Nil checksums map
- `TestVerifyIntegrity_Symlink` - Single-hop symlink resolution
- `TestVerifyIntegrity_Mixed` - Combined scenarios

However, the design document (DESIGN-library-verify-integrity.md) explicitly mentions **symlink chains** as a real-world pattern (lines 216-227):
```
libstdc++.so -> libstdc++.so.6
libstdc++.so.6 -> libstdc++.so.6.0.33
libstdc++.so.6.0.33 (actual file)
```

The existing `TestVerifyIntegrity_Symlink` only tests a single-hop symlink. Adding multi-hop chain and broken symlink tests strengthens coverage for real-world library directory structures.

### Alternatives Considered

- **Do nothing (issue already complete)**: While all AC items are technically covered, the design doc emphasizes symlink chains as a key scenario. Adding explicit tests improves confidence.
- **Add tests in a follow-up issue**: Would fragment test coverage and create unnecessary overhead. Better to include them now.

## Files to Modify

- `internal/verify/integrity_test.go` - Add two new test functions

## Files to Create

None.

## Implementation Steps

- [ ] Add `TestVerifyIntegrity_SymlinkChain` - Test multi-hop symlink resolution (a -> b -> c pattern)
- [ ] Add `TestVerifyIntegrity_BrokenSymlink` - Test symlink pointing to non-existent target (should report as missing)
- [ ] Run `go test ./internal/verify/...` to verify all tests pass
- [ ] Verify existing E2E flow still works (run full test suite)

## Testing Strategy

- **Unit tests**: The new tests are themselves unit tests for edge cases
- **Regression**: Run full `go test ./...` to ensure no breakage
- **Manual verification**: Not required - tests are self-contained with temp directories

## Risks and Mitigations

- **Platform differences**: Symlink behavior is consistent across Linux/macOS. Windows testing uses `t.TempDir()` which handles platform differences.
- **Test complexity**: Keep tests simple and follow existing patterns in the file.

## Success Criteria

- [ ] `TestVerifyIntegrity_SymlinkChain` passes with 3-level symlink chain (a -> b -> c)
- [ ] `TestVerifyIntegrity_BrokenSymlink` passes with symlink to non-existent target
- [ ] All existing tests continue to pass
- [ ] `go test ./...` passes

## Open Questions

None. The implementation approach is clear and follows established patterns.
