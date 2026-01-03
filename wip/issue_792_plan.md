# Issue 792 Implementation Plan

## Summary

Add path filtering to sandbox-tests.yml to skip sandbox tests on docs-only changes, following the same pattern used in test.yml.

## Approach

Copy the path filtering pattern from test.yml (lines 133-146) and adapt it for sandbox-tests.yml. The filter will check for changes to:
- Go source files (`**/*.go`)
- Go module files (`go.mod`, `go.sum`)
- Recipe files (`internal/recipe/recipes/**/*.toml`)
- Test matrix configuration (`test-matrix.json`)

When no code files change (docs-only), the sandbox tests will be skipped.

## Files to Modify

- `.github/workflows/sandbox-tests.yml` - Add path filter step and conditional to matrix job

## Implementation Steps

- [x] Add `dorny/paths-filter@v3` step to matrix job (after checkout)
- [x] Define filter for code paths matching test.yml pattern
- [x] Add conditional `if` expression to sandbox-tests job

## Testing Strategy

- Create a docs-only commit to verify sandbox tests are skipped
- Verify via PR check that tests show as "skipped" not "running"
- Ensure code changes still trigger sandbox tests normally
