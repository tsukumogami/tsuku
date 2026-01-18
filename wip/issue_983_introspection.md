# Issue 983 Introspection

## Context Reviewed

- **Design doc:** `docs/designs/DESIGN-library-verify-deps.md`
- **Sibling issues reviewed:** #978, #979, #984
- **Prior patterns identified:**
  - `internal/verify/` package structure with `types.go` for error types and `header.go` for validation
  - `debug/elf` and `debug/macho` usage patterns in `header.go`
  - `LibraryVersionState.Sonames` field and `SetLibrarySonames()` helper now exist in state management
  - Panic recovery pattern for parser robustness

## Gap Analysis

### Minor Gaps

1. **File location confirmed**: Issue specifies `internal/verify/soname.go` which aligns with existing `internal/verify/header.go` pattern.

2. **Error handling pattern**: Issue doesn't mention using `ValidationError` type from `types.go`, but based on `header.go` patterns, soname extraction errors should likely follow the same categorization approach. However, the issue notes that errors for missing sonames should return empty string, not an error - which is correct for the use case.

3. **Fat binary handling already exists**: `header.go` has `validateFatPath()` for fat/universal binaries using `macho.OpenFat()`. Issue mentions "Support for fat/universal binaries on macOS" but doesn't detail the approach. The existing pattern in `header.go` (iterate `ff.Arches`, find matching architecture) should be followed.

4. **Test fixture creation**: Issue mentions `testdata/verify/` but this directory doesn't exist. The project uses `testdata/` at root level. Test fixtures should be created at `testdata/verify/` or the tests could create minimal binaries programmatically (common Go pattern for binary format tests).

### Moderate Gaps

None identified. The issue spec is complete and aligns with implemented prerequisites.

### Major Gaps

None identified. The blocking dependency (#978) is now closed and the required state infrastructure exists.

## Recommendation

**Proceed**

The issue specification is complete and implementation-ready. The blocking issue #978 has been merged (PR #995), providing:
- `LibraryVersionState.Sonames` field for storage
- `SetLibrarySonames()` helper for atomic updates

## Proposed Amendments

No amendments needed. The minor gaps can be incorporated into implementation without changing scope:

1. Follow `header.go` panic recovery pattern for parser robustness
2. For fat binaries, use `macho.OpenFat()` and iterate arches as in `validateFatPath()`
3. Create `testdata/verify/` directory for test fixtures (or use programmatic fixture creation)

## Implementation Notes

The design doc's Mach-O example has a minor issue - `macho.Dylib` doesn't expose `Cmd` directly. The implementation should iterate `f.Loads` and type-assert to find `*macho.Dylib` entries, checking if it's the library's own install name (typically the first one, or the one matching the file's path convention). The Go `debug/macho` package documentation and examples should be referenced for the exact approach.
