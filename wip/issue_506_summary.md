# Issue 506 Summary

## What Was Implemented

Added `loadPlanFromSource()` and `validateExternalPlan()` utilities for loading and validating installation plans from files or stdin. These form the foundation for the `--plan` flag on the install command.

## Changes Made

- `cmd/tsuku/plan_utils.go`: New file with two functions:
  - `loadPlanFromSource()`: Reads plan JSON from file path or stdin (when path is "-")
  - `loadPlanFromSourceWithReader()`: Internal implementation for testing
  - `validateExternalPlan()`: Validates plan structure, platform compatibility, and tool name

- `cmd/tsuku/plan_utils_test.go`: Comprehensive test suite with 11 tests:
  - File loading: valid file, not found, invalid JSON
  - Stdin loading: valid JSON, invalid JSON with hint
  - Validation: valid plan, OS mismatch, arch mismatch, tool mismatch, structural failures

## Key Decisions

- **Separate file from install.go**: Keeps install.go focused and makes utilities testable independently
- **Reader abstraction for stdin**: `loadPlanFromSourceWithReader()` accepts io.Reader for easy testing without actual stdin manipulation
- **Wrap ValidatePlan()**: External plan validation reuses existing structural validation then adds platform and tool checks

## Trade-offs Accepted

- **Functions in cmd package**: Could have been in internal/, but stdin handling is CLI-specific

## Test Coverage

- New tests added: 11
- All tests pass
- Coverage for both happy path and error scenarios

## Known Limitations

- None - implementation follows design document exactly

## Future Improvements

- Issue #507 will use these utilities to implement `--plan` flag
