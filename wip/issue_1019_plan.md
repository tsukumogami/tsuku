# Issue 1019 Implementation Plan

## Overview

Add integration tests for dlopen verification (Level 3). After introspection, most acceptance criteria are already covered by existing tests from sibling issues. This plan focuses on filling remaining gaps.

## Existing Coverage Analysis

The 42 existing tests in `dltest_test.go` cover:
- JSON parsing (4 tests)
- EnsureDltest logic (4 tests)
- Batch splitting (6 tests)
- BatchError types (3 tests)
- InvokeDltest behavior (10 tests including timeout, crash retry, mock helpers)
- Environment sanitization (4 tests)
- Path validation (6 tests)
- RunDlopenVerification (3 tests)
- Error sentinels (2 tests)

## Remaining Gaps

### 1. Partial Batch Failure (Mixed Results)
**Status**: Partially covered by TestInvokeDltest_ExitCode1_NotCrash

Need explicit test for a batch where some libraries pass and some fail, verifying:
- All results returned (not just failures)
- OK status correctly reflects per-library outcome
- Error messages present only for failures

### 2. Multi-Batch Aggregation with Failures
**Status**: TestInvokeDltest_MockHelper_ManyPaths tests 75 paths but all succeed

Need test for >50 paths where some batches have failures, verifying results are correctly aggregated.

### 3. Exit Code 2 (Usage Error)
**Status**: Not tested

The helper spec says exit code 2 = usage error. Need test that this is handled appropriately.

## Implementation Plan

### 1. Add Test: Mixed Results in Single Batch

```go
func TestInvokeDltest_MockHelper_MixedResults(t *testing.T)
```
- Mock helper returns JSON with mix of ok:true and ok:false
- Verify all results returned
- Verify error messages only on failures

### 2. Add Test: Large Batch with Failures Across Batches

```go
func TestInvokeDltest_MockHelper_MultiBatchWithFailures(t *testing.T)
```
- Create 75 lib files (splits into 50+25)
- Mock helper fails some in each batch
- Verify total results = 75
- Verify failures correctly identified

### 3. Add Test: Exit Code 2 Handling

```go
func TestInvokeDltest_ExitCode2_UsageError(t *testing.T)
```
- Mock helper returns exit code 2
- Verify error returned (not treated as success)

### 4. Documentation

Add comment block at top of new tests explaining:
- These are integration tests for issue #1019
- They complement unit tests added in #1014-#1018
- Skip in short mode

## Files to Modify

- `internal/verify/dltest_test.go` - add new test functions

## Test Plan

1. Run `go test -v ./internal/verify/...` to verify new tests pass
2. Run `go test -test.short ./...` to verify short mode skips integration tests
3. Verify existing tests still pass
