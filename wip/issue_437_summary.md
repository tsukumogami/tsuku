# Issue 437 Summary

## What Was Implemented

Implemented `DecomposeToPrimitives()` function that recursively decomposes composite actions until only primitive actions remain, with cycle detection to prevent infinite recursion.

## Changes Made

- `internal/actions/decomposable.go`: Added `DecomposeToPrimitives()`, `decomposeToPrimitivesInternal()`, and `computeStepHash()` functions
- `internal/actions/decomposable_test.go`: Added 8 test cases covering all scenarios

## Key Decisions

- **Reuse existing `Step` struct**: The design document suggested `PrimitiveStep` but `Step` already has all required fields (Action, Params, Checksum, Size), so we reuse it
- **Hash-based cycle detection**: Using SHA256 hash of action name + JSON-serialized params provides reliable cycle detection without complex graph tracking
- **Internal recursive function pattern**: Separated public `DecomposeToPrimitives()` from internal `decomposeToPrimitivesInternal()` to manage visited set cleanly

## Trade-offs Accepted

- **Params serialization for hashing**: JSON marshaling for cycle detection has overhead, but it's only called once per action in the decomposition chain
- **Short hash (8 bytes)**: Using first 8 bytes of SHA256 for readability; collision probability is negligible for practical decomposition depths

## Test Coverage

- New tests added: 8 test cases
- Coverage: Tests cover primitive passthrough, single-level decomposition, recursive decomposition, cycle detection, checksum propagation, and error handling

## Known Limitations

- Cycle detection is based on exact params match; actions with different params but same semantic meaning are treated as distinct
- No depth limit implemented (not needed for expected decomposition depths)

## Future Improvements

- The function will be called by plan generator (#440) after composite actions implement Decompose() (#438, #439)
