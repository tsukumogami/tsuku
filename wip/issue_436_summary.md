# Issue 436 Summary

## What Was Implemented

Defined the foundational types for decomposable actions: the `Decomposable` interface, `Step` struct, `EvalContext` struct, and a primitive action registry with `IsPrimitive()` function.

## Changes Made
- `internal/actions/decomposable.go`: New file containing:
  - `Decomposable` interface with `Decompose()` method signature
  - `Step` struct for decomposition results
  - `EvalContext` struct for decomposition context
  - Primitive registry (map) with 8 Tier 1 primitives
  - `IsPrimitive()`, `RegisterPrimitive()`, and `Primitives()` functions
- `internal/actions/decomposable_test.go`: New file with unit tests for all exported functions and structs

## Key Decisions
- **Separate file**: Created `decomposable.go` rather than adding to `action.go` for better separation of concerns
- **Map-based registry**: Used `map[string]bool` for O(1) primitive lookup
- **No PreDownloader field**: Omitted from EvalContext to avoid validate package dependency; will be added in issue #437

## Trade-offs Accepted
- **EvalContext missing Downloader**: The design document shows a `Downloader *validate.PreDownloader` field, but including it would create an import cycle. Issue #437 (recursive decomposition) will address this when the field is actually needed.

## Test Coverage
- New tests added: 5 test functions
- Tests cover: `IsPrimitive()`, `Primitives()`, `RegisterPrimitive()`, `Step` struct, `EvalContext` struct

## Known Limitations
- EvalContext does not yet include the PreDownloader field for checksum computation
- No integration with plan generator yet (that's issue #440)

## Future Improvements
- Issue #437 will implement `DecomposeToPrimitives()` recursive decomposition
- Issues #438-#439 will implement `Decompose()` on composite actions
