# Issue 1817 Summary

## What Was Implemented

Added homepage URL scheme validation to the Go recipe validator's `validateMetadata()` function, closing the gap between the Go validator (used in CI) and the Python registry generator (used at deploy time).

## Changes Made
- `internal/recipe/validator.go`: Added homepage URL validation in `validateMetadata()` - checks for `https://` prefix and rejects dangerous schemes (`javascript:`, `data:`, `vbscript:`)
- `internal/recipe/validator_test.go`: Added 4 test functions covering valid HTTPS, HTTP rejection, dangerous schemes (table-driven with 4 cases including mixed case), and empty homepage

## Key Decisions
- Inline in `validateMetadata()`: Kept the check inline rather than extracting a separate function, matching how other metadata fields are validated in the same function
- Didn't reuse `validateURLParam()`: That function accepts both HTTP and HTTPS for step URLs. Homepage validation is stricter (HTTPS only) with a different policy

## Trade-offs Accepted
- Substring match for dangerous schemes: A URL like `https://example.com/javascript:foo` would be flagged. This matches the Python behavior and is conservative, which is acceptable for a security check.

## Test Coverage
- New tests added: 4 functions (7 test cases total)
- All existing tests continue to pass

## Requirements Mapping

| AC | Status | Evidence |
|----|--------|----------|
| `validateMetadata()` rejects non-HTTPS homepage | Implemented | `validator.go:141-143` |
| `validateMetadata()` rejects dangerous schemes | Implemented | `validator.go:145-151` |
| Clear error messages match existing style | Implemented | Uses `result.addError()` with `metadata.homepage` field |
| Test coverage for valid/invalid/dangerous URLs | Implemented | `validator_test.go` 4 new test functions |
| All existing recipes pass validation | Verified | Full test suite passes with no regressions |
