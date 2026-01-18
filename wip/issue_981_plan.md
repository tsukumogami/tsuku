# Issue 981 Implementation Plan

## Summary

Create `internal/verify/abi.go` with a `ValidateABI(path string) error` function that checks ELF binaries for PT_INTERP segment validity, returning nil for static binaries or when the interpreter exists, and `ErrABIMismatch` when the interpreter is missing.

## Approach

Follow the existing patterns in `header.go` for ELF validation: use `debug/elf` package, implement panic recovery, and return categorized `*ValidationError` errors. The function iterates over ELF program headers to find PT_INTERP, extracts the interpreter path, and checks if it exists on the filesystem.

### Alternatives Considered

- **Use external tools (readelf, objdump)**: Not chosen because the design document explicitly requires using Go's `debug/elf` package only, with no external tool dependencies.
- **Integrate PT_INTERP check into existing ValidateHeader function**: Not chosen because the design treats ABI validation as a separate step (step 2a) consumed by recursive validation (#989), and keeping it modular allows callers to selectively apply validation.

## Files to Modify

- `internal/verify/types.go` - Add `ErrABIMismatch ErrorCategory = 10` constant and update `String()` method

## Files to Create

- `internal/verify/abi.go` - New file containing `ValidateABI(path string) error` function
- `internal/verify/abi_test.go` - Unit tests following patterns from `header_test.go`

## Implementation Steps

- [x] Add `ErrABIMismatch ErrorCategory = 10` to `types.go` with comment noting Tier 2 error categories
- [x] Update `ErrorCategory.String()` to handle `ErrABIMismatch` returning "ABI mismatch"
- [x] Create `abi.go` with package declaration and imports (`debug/elf`, `os`, `runtime`)
- [x] Implement `ValidateABI(path string) error` with:
  - Runtime check for non-Linux (return nil immediately for macOS)
  - Panic recovery defer block (matching `header.go` pattern)
  - Open file with `elf.Open(path)`
  - Return nil for non-ELF files (graceful handling)
  - Iterate `f.Progs` looking for `elf.PT_INTERP` type
  - If no PT_INTERP found, return nil (static binary)
  - If PT_INTERP found, read interpreter path from segment data
  - Check if interpreter path exists with `os.Stat()`
  - Return nil if interpreter exists
  - Return `*ValidationError{Category: ErrABIMismatch, ...}` if interpreter missing
- [x] Create `abi_test.go` with test cases:
  - Test with system shared library (should pass - has valid PT_INTERP)
  - Test with static binary (should pass - no PT_INTERP)
  - Test skip on non-Linux systems
  - Test non-ELF file returns nil (graceful handling)

## Testing Strategy

- **Unit tests**: Cover all code paths in `ValidateABI`:
  - Valid ELF with existing interpreter (e.g., system libc) returns nil
  - Static binary (no PT_INTERP) returns nil
  - Non-Linux platforms return nil immediately
  - Non-ELF files return nil (handled gracefully)
  - Construct synthetic test for missing interpreter scenario using a test helper that patches the interpreter path check

- **Integration tests**: Not required for this issue; #989 (recursive validation) will integrate this function.

- **Manual verification**: Run tests on Linux to confirm system library validation works:
  ```bash
  go test -v ./internal/verify/... -run TestValidateABI
  ```

## Risks and Mitigations

- **Risk**: PT_INTERP segment data may include null terminators or extra padding
  - **Mitigation**: Use `bytes.TrimRight(data, "\x00")` to strip null bytes when reading interpreter path

- **Risk**: Some exotic ELF files may have multiple PT_INTERP segments
  - **Mitigation**: Use first PT_INTERP found (standard behavior), document this choice in code comment

- **Risk**: Interpreter path could be relative or use symlinks
  - **Mitigation**: Use `os.Stat()` directly on the path as returned; kernel resolves these at runtime anyway

## Success Criteria

- [ ] `go test ./internal/verify/...` passes with new tests
- [ ] `go build ./...` succeeds without errors
- [ ] `ErrABIMismatch` has explicit value 10 in `types.go`
- [ ] `ValidateABI` returns nil for static binaries
- [ ] `ValidateABI` returns nil for macOS (no-op)
- [ ] `ValidateABI` returns `ErrABIMismatch` when interpreter is missing
- [ ] Code follows existing patterns from `header.go` (panic recovery, error categorization)

## Open Questions

None - all implementation details are clear from the design document and introspection analysis.
