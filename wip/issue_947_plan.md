# Issue 947 Implementation Plan

## Summary

Implement header validation module (Tier 1) for library verification as defined in `docs/designs/DESIGN-library-verify-header.md`.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/verify/types.go` | Data structures: HeaderInfo, ValidationError, ErrorCategory |
| `internal/verify/header.go` | Magic detection, format dispatch, ELF/Mach-O/Fat validation |
| `internal/verify/header_test.go` | Unit tests and benchmarks |

## Implementation Steps

### Step 1: Create types.go

Define core data structures:
- `HeaderInfo` struct with Format, Type, Architecture, Dependencies, SymbolCount, SourceArch
- `ValidationError` struct with Category, Path, Message, Err
- `ErrorCategory` constants (ErrUnreadable, ErrInvalidFormat, ErrNotSharedLib, ErrWrongArch, ErrTruncated, ErrCorrupted)
- String method for ErrorCategory

### Step 2: Create header.go

Main validation logic:
1. Magic number constants (ELF, Mach-O 32/64, Mach-O reversed, Fat, Ar archive)
2. `readMagic(path)` - read first 8 bytes
3. `detectFormat(magic)` - return format string
4. `ValidateHeader(path)` - main entry point with panic recovery
5. `validateELFPath(path)` - ELF shared object validation
6. `validateMachOPath(path)` - Mach-O dylib validation
7. `validateFatPath(path)` - Fat binary architecture extraction
8. Architecture mapping functions (mapGoArchToELF, mapGoArchToMachO, mapELFMachine, mapMachOCpu)
9. Error categorization functions

### Step 3: Create header_test.go

Tests for:
- Valid ELF shared object (use system libc.so.6 on Linux)
- Valid Mach-O dylib (use system libSystem.B.dylib on macOS if available)
- Error categories: truncated file, wrong magic, executable (not shared lib), wrong architecture
- Static library detection (.a archive)
- Benchmarks for performance validation

### Step 4: Integration verification

- Run `go build ./...` to verify compilation
- Run `go test ./internal/verify/...` to run new tests
- Run `go vet ./internal/verify/...` for static analysis

## Design Decisions (from design doc)

1. **Unified function with early magic detection** - Read 8 bytes, dispatch to format-specific validator
2. **Lazy symbol counting** - Return -1 by default, Tier 2 can request if needed
3. **Static library detection** - Detect `.a` archives with magic `!<arch>\n`
4. **Panic recovery** - Wrap validation with `defer recover()` for robustness
5. **Six error categories** - Enable actionable user messages

## Testing Strategy

- Use real system libraries for validation tests (platform-dependent)
- Create test fixtures for error cases (truncated, wrong magic)
- Target ~50us per file for header validation benchmark

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Platform differences in test fixtures | Use conditional tests based on runtime.GOOS |
| No system libraries available | Skip tests that require system libraries |
| Performance regression | Add benchmark and compare to 50us target |
