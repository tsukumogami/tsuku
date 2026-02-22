# Architect Review: #1881 feat(dashboard): add library_only failure subcategory (Round 2)

## Summary

Round 1 identified an advisory about dead code in the classifier: `classifyDeterministicFailure` had a case matching `"library recipe generation failed"` but no production code produced that error string. Commit 6300f70 fixed this by wrapping the `generateLibraryRecipe` error at `homebrew.go:2184` with `fmt.Errorf("library recipe generation failed: %w", err)`. The round 1 advisory is resolved.

No blocking findings. No new advisory findings.

## Files Changed

- `internal/dashboard/failures.go` - Added `"library_only": true` to `knownSubcategories` map (line 46)
- `internal/dashboard/failures_test.go` - Added test case for `library_only` bracket tag extraction (line 56-60)
- `internal/builders/homebrew.go` - Added `library recipe generation failed` case to `classifyDeterministicFailure` (line 503-505), added error wrapping at line 2184
- `internal/builders/homebrew_test.go` - Added classification test (line 2542-2545), added tag presence test (line 2587-2607)

## Round 1 Advisory Resolution

The error wrapping at `homebrew.go:2184`:
```go
return nil, fmt.Errorf("library recipe generation failed: %w", err)
```

...produces error strings like `"library recipe generation failed: no platform contents provided"`, which matches the classifier's `strings.Contains(msg, "library recipe generation failed")` at line 503. The classifier case is now reachable through a real production code path. The test at `homebrew_test.go:2593` now mirrors what production code actually produces. Resolved.

## Architectural Assessment

### Pattern Compliance

The implementation uses the established subcategory mechanism. Producer (`internal/builders/`) embeds `[library_only]` in the error message string. Consumer (`internal/dashboard/`) registers it in `knownSubcategories` and parses it with the existing `extractSubcategory()`. No new mechanism introduced.

### State Contract

`failure-record.schema.json` is unmodified. The `category` field uses the existing `complex_archive` enum value. The subcategory is carried in the free-form `message` field and extracted at read time. No schema drift.

### Dependency Direction

`internal/builders/` and `internal/dashboard/` communicate through serialized JSONL data, not Go imports. No new cross-package imports. Information flows: builders write tagged messages -> JSONL files -> dashboard reads and parses. Correct direction.

### No Parallel Patterns

No new error types, no new fields on `DeterministicFailedError`, no alternative classification path. One registration in `knownSubcategories`, one bracket tag in the classifier output. Consistent with all other subcategories.

## Verdict

The change adds one subcategory to an existing classification system using the established pattern at every layer. The round 1 advisory about unreachable classifier code has been fixed. No structural issues.
