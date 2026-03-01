# Issue 1967 Summary

## What Was Implemented

Fixed three fallback code paths that created broken symlinks in `$TSUKU_HOME/tools/current/` when `Binaries` was empty. The fallbacks now use `bin/<name>` instead of just `<name>`, matching the already-correct wrapper fallback behavior.

## Changes Made

- `internal/install/manager.go`: Fixed `createSymlink` and `Activate` fallbacks to use `filepath.Join("bin", name)`
- `internal/install/remove.go`: Fixed `RemoveVersion` fallback to use `filepath.Join("bin", name)`
- `internal/install/manager_test.go`: Updated `TestActivate_FallbackToBinaryName` to use bin/ layout, strengthened `TestInstallWithOptions_NoBinariesFallback` with target verification
- `internal/install/remove_test.go`: Added `TestRemoveVersion_EmptyBinariesFallback`

## Key Decisions

- Fixed the fallback paths rather than eliminating them. The fallbacks exist for backward compatibility with tools that have empty `Binaries` in state.json, and removing them would break version switching for those tools.
- Matched the existing correct pattern from `createWrappersForBinaries` (line 343) which already uses `filepath.Join("bin", toolName)`.

## Test Coverage

- 1 new test added (`TestRemoveVersion_EmptyBinariesFallback`)
- 2 existing tests updated with stronger assertions (exact target verification + resolution check)
- All tests verify symlinks both resolve correctly and point to the expected path

## Requirements Mapping

| AC | Status | Evidence |
|----|--------|----------|
| Fix createSymlink fallback | Implemented | manager.go:295 |
| Fix Activate fallback | Implemented | manager.go:258 |
| Fix RemoveVersion fallback | Implemented | remove.go:124 |
