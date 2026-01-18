# Implementation Context for Issue #986

## Design Reference
Design: `docs/designs/DESIGN-library-verify-deps.md`
Section: Solution Architecture - SonameIndex and Classification Flow

## Issue Summary
Implement `SonameIndex` data structure and `ClassifyDependency` function for Tier 2 dependency validation. This provides O(1) reverse lookups from soname to recipe and categorizes dependencies.

## Key Design Decisions

### What This Enables
The classification determines how each dependency is validated:
- **PURE SYSTEM**: Inherently OS-provided (libc, libpthread) - verify accessible, skip recursion
- **TSUKU-MANAGED**: Built/managed by tsuku - verify provides expected soname, recurse
- **EXTERNALLY-MANAGED**: Tsuku recipe delegating to pkg manager - verify, skip recursion
- **UNKNOWN**: Unclassified - FAIL (pre-GA, helps find corner cases)

### Classification Order (Critical)
**Check soname index FIRST, then system patterns, else UNKNOWN.**

A soname like `libssl.so.3` should be identified as TSUKU-managed when we have an installed recipe, not matched against system patterns.

### Error Category Design Decision
Per design decision #2: Use explicit error constant values, not iota.
```go
ErrUnknownDependency ErrorCategory = 11 // Explicit value
```

## Dependencies
- #978 (Add Sonames field) - sonames must be stored for index building - **DONE**
- #980 (System library patterns) - classification uses SystemLibraryRegistry - **DONE** (PR #996)

## Downstream Dependencies
- #989 (Recursive dependency validation) - needs SonameIndex and ClassifyDependency

## Integration Points

### With State
`BuildSonameIndex(state *State)` iterates `state.Libs` to build reverse index from sonames to recipes.

### With System Library Registry
`ClassifyDependency` uses `SystemLibraryRegistry.IsSystemLibrary()` from issue #980.

## Files to Create/Modify
- `internal/verify/index.go` (NEW) - SonameIndex struct and BuildSonameIndex
- `internal/verify/classify.go` (NEW) - DepCategory type and ClassifyDependency
- `internal/verify/index_test.go` (NEW) - Tests for index building
- `internal/verify/classify_test.go` (NEW) - Tests for classification
