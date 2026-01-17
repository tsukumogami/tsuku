# Implementation Context: Issue #947

**Source Design**: docs/designs/DESIGN-library-verify-header.md (Accepted)

## Scope

Issue #947 was originally for creating the design document. The design has been created and accepted. We are now implementing the header validation module (Tier 1) as part of the same PR.

## Files to Create

1. `internal/verify/types.go` - Data structures (HeaderInfo, ValidationError, ErrorCategory)
2. `internal/verify/header.go` - Main validation logic with magic detection and format dispatch
3. `internal/verify/header_test.go` - Tests and benchmarks

## Key Design Decisions

- **Unified function with early magic detection**: `ValidateHeader(path)` reads 8-byte magic, dispatches to format-specific validator
- **Lazy symbol counting**: SymbolCount returns -1 by default for performance
- **Static library detection**: `.a` archives detected with clear error message
- **Panic recovery**: All validation functions wrapped with `defer recover()`
- **Six error categories**: ErrUnreadable, ErrInvalidFormat, ErrNotSharedLib, ErrWrongArch, ErrTruncated, ErrCorrupted

## Performance Target

~50 microseconds per file (assumes local storage with OS caching)

## Integration Point

`cmd/tsuku/verify.go` `verifyLibrary()` function will call `header.ValidateHeader()` for each library file
