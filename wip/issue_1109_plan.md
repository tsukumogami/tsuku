# Issue 1109 Implementation Plan

## Summary

Add libc detection to the platform package with a new `internal/platform/libc.go` file containing `DetectLibc()`, update the `Target` struct with a `libc` field, and extend the `Matchable` interface with a `Libc()` method to enable downstream recipe filtering.

## Approach

The approach follows the design document closely: detect musl by checking for the presence of `/lib/ld-musl-*.so.1` (the musl dynamic linker). This is a reliable indicator because musl-based systems always have this linker present, while glibc systems never do. The detection is simple, fast, and doesn't require parsing files or executing commands.

For testing, we use filesystem-based fixtures similar to the existing os-release tests, creating mock directory structures that simulate musl and glibc environments.

### Alternatives Considered

- **Parse /proc/self/exe or ldd output**: More complex, requires process execution, and varies by system. Rejected for simplicity.
- **Check for /lib/libc.musl-*.so.1**: Less reliable - the exact filename varies more than the dynamic linker. Rejected.
- **Use cgo to call getconf GNU_LIBC_VERSION**: Requires cgo, breaks pure Go builds, and fails silently on musl. Rejected.

## Files to Modify

- `internal/platform/target.go` - Add `libc` field to `Target` struct, add `Libc()` method, update `NewTarget()` signature
- `internal/platform/family.go` - Update `DetectTarget()` to call `DetectLibc()` and populate the libc field
- `internal/platform/target_test.go` - Add tests for `Libc()` method on Target
- `internal/recipe/types.go` - Add `Libc()` method to `Matchable` interface and `MatchTarget` struct

## Files to Create

- `internal/platform/libc.go` - Contains `DetectLibc()` and `DetectLibcWithRoot()` functions
- `internal/platform/libc_test.go` - Unit tests for libc detection
- `internal/platform/testdata/libc/musl/lib/ld-musl-x86_64.so.1` - Empty file fixture for musl detection
- `internal/platform/testdata/libc/glibc/lib/.gitkeep` - Empty directory fixture for glibc (no musl linker)

## Implementation Steps

- [ ] Create `internal/platform/libc.go` with `DetectLibc()` and `DetectLibcWithRoot()` functions
- [ ] Create testdata fixtures for musl and glibc detection scenarios
- [ ] Create `internal/platform/libc_test.go` with unit tests using fixtures
- [ ] Add `libc` field to `Target` struct in `target.go`
- [ ] Add `Libc()` method to `Target` in `target.go`
- [ ] Update `NewTarget()` to accept libc parameter
- [ ] Update `DetectTarget()` in `family.go` to call `DetectLibc()`
- [ ] Add `Libc()` method to `Matchable` interface in `internal/recipe/types.go`
- [ ] Add `libc` field and `Libc()` method to `MatchTarget` struct
- [ ] Update `NewMatchTarget()` to accept libc parameter
- [ ] Update existing tests for modified function signatures
- [ ] Run `go test ./internal/platform/...` to verify all tests pass
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...`

## Testing Strategy

### Unit Tests

**libc_test.go:**
- `TestDetectLibc_Musl` - Uses fixture with `/lib/ld-musl-x86_64.so.1` present, expects "musl"
- `TestDetectLibc_MuslArm64` - Uses fixture with `/lib/ld-musl-aarch64.so.1`, expects "musl"
- `TestDetectLibc_Glibc` - Uses fixture with no musl linker, expects "glibc"
- `TestDetectLibc_EmptyRoot` - Uses fixture with no /lib directory, expects "glibc" (default)
- `TestDetectLibcWithRoot` - Verifies root parameter is used correctly

**target_test.go additions:**
- `TestTarget_Libc` - Verify `Libc()` returns the libc field value
- `TestNewTarget_WithLibc` - Verify `NewTarget()` accepts and stores libc parameter
- Update existing `TestTarget_LinuxFamily` to include libc field assertions

**family_test.go additions:**
- `TestDetectTarget_Linux_WithLibc` - Verify `DetectTarget()` populates libc field on Linux

### Integration Tests

- The validation script in the issue (`go test -v ./internal/platform/... -run TestLibc`) serves as integration verification
- Manual verification via `./tsuku debug-target | grep -q "libc:"` (note: debug-target command is out of scope for this issue)

## Risks and Mitigations

- **Risk**: Glob pattern might not match all musl architectures
  - **Mitigation**: Pattern `/lib/ld-musl-*.so.1` covers all standard musl architectures (x86_64, aarch64, armhf, etc.). The wildcard handles architecture variations.

- **Risk**: Tests run on glibc CI runners, can't test actual musl detection
  - **Mitigation**: Use filesystem fixtures with `DetectLibcWithRoot()` function that accepts a root path parameter, allowing tests to use testdata directories.

- **Risk**: Breaking existing callers of `NewTarget()` and `NewMatchTarget()`
  - **Mitigation**: Update all callsites. Search for existing usage and update signatures. The change is additive and callers can pass empty string for backwards compatibility.

## Success Criteria

- [ ] `DetectLibc()` function exists in `internal/platform/libc.go`
- [ ] Detection checks for `/lib/ld-musl-*.so.1` presence
- [ ] Returns "musl" if present, "glibc" otherwise
- [ ] `libc` field exists on `Target` struct
- [ ] `Libc()` method exists on `Target` (satisfies Matchable interface)
- [ ] `Libc()` method exists on `MatchTarget` (satisfies Matchable interface)
- [ ] `NewTarget()` and `DetectTarget()` populate libc field
- [ ] Unit tests cover both glibc and musl detection scenarios
- [ ] Tests use testdata fixtures (similar to existing os-release tests)
- [ ] All existing tests continue to pass
- [ ] `go test -v ./internal/platform/... -run TestLibc` passes

## Open Questions

None - all design decisions are clearly specified in the design document and implementation context.
