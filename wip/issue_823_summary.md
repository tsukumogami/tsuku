# Issue 823 Summary

## What Was Implemented

Extended WhenClause with LinuxFamily and Arch fields for explicit platform targeting, and implemented MergeWhenClause() to combine implicit action constraints with explicit when clause constraints while detecting conflicts.

## Changes Made

- `internal/recipe/types.go`:
  - Added `Arch` and `LinuxFamily` fields to WhenClause struct
  - Updated `IsEmpty()` to include new fields in empty check
  - Updated `UnmarshalTOML` to parse `arch` and `linux_family` from when clause data
  - Updated `ToMap` to serialize new fields when present
  - Added `MergeWhenClause()` function with conflict detection for all constraint dimensions

- `internal/recipe/types_test.go`:
  - Added 15 unit tests for WhenClause and MergeWhenClause functionality

## Key Decisions

- **Exported function name**: Named the function `MergeWhenClause` (exported) rather than `mergeWhenClause` since it will be called by downstream code in issue #824.
- **Conflict detection order**: Check platform array first, then OS, then LinuxFamily, then Arch. This follows the order of specificity (platform is most specific).
- **Multi-OS behavior**: When implicit has no OS and when clause has multiple OSes, leave result.OS empty (step runs on multiple OSes).

## Trade-offs Accepted

- **No arch extraction from Platform array**: When when.Platform is set, we don't extract arch information from the tuples (e.g., "linux/amd64" -> arch="amd64"). This keeps the logic simpler and platform array is primarily for exact matching.

## Test Coverage

- New tests added: 15
- All acceptance criteria test cases implemented:
  - WhenClause.IsEmpty with new fields (2 tests)
  - MergeWhenClause nil cases (3 tests)
  - Conflict detection (4 tests)
  - Extension/compatibility (6 tests)

## Known Limitations

- Platform array and single Arch field are somewhat redundant - a recipe author could specify both. The implementation allows this but doesn't extract arch from platform tuples.

## Future Improvements

- Issue #824 will use MergeWhenClause in computeAnalysis to produce StepAnalysis
- Issue #827 will add Matchable interface that uses the new LinuxFamily field
