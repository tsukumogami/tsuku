# Validation Results: Issue #1881

**Date**: 2026-02-22
**Scenario**: scenario-11 (library_only subcategory recognized by extractSubcategory)

## Scenario 11: library_only subcategory recognized by extractSubcategory

**Status**: PASSED

### Test Command

The test plan specified:
```
go test -v -run 'TestExtractSubcategory.*library_only' ./internal/dashboard/...
```

This pattern returned "no tests to run" because the actual test is a subtest of `TestExtractSubcategory_bracketedTag` named `library_only_tag`. The correct pattern is:
```
go test -v -run 'TestExtractSubcategory_bracketedTag/library_only' ./internal/dashboard/...
```

### Verification Details

**1. knownSubcategories contains "library_only": true**

Verified in `internal/dashboard/failures.go` line 46:
```go
var knownSubcategories = map[string]bool{
    ...
    "library_only":    true,
    ...
}
```

**2. extractSubcategory recognizes [library_only] bracketed tag**

Test case in `internal/dashboard/failures_test.go` lines 56-60:
```go
{
    name:     "library_only tag",
    category: "complex_archive",
    message:  "[library_only] formula bdw-gc detected as library but recipe generation failed",
    want:     "library_only",
},
```

Test output:
```
=== RUN   TestExtractSubcategory_bracketedTag/library_only_tag
--- PASS: TestExtractSubcategory_bracketedTag/library_only_tag (0.00s)
```

**3. Existing subcategory extraction for other tags continues to work**

All 7 subtests in `TestExtractSubcategory_bracketedTag` pass:
- api_error_tag: PASS
- no_bottles_tag: PASS
- complex_archive_tag: PASS
- unknown_bracketed_tag_ignored: PASS
- verify_failed_tag: PASS
- install_failed_tag: PASS
- library_only_tag: PASS

All regex pattern tests pass (17 subtests in `TestExtractSubcategory_regexPatterns`).
All exit code fallback tests pass (5 subtests in `TestExtractSubcategory_exitCodeFallback`).
Priority and fallback edge cases pass (`bracketPriorityOverRegex`, `noFallbackWithMessage`).

**4. Full dashboard package regression check**

All tests in `./internal/dashboard/...` pass (80+ tests, 0 failures).

### Note on Test Plan Command

The test plan command `go test -v -run 'TestExtractSubcategory.*library_only' ./internal/dashboard/...` does not match any test. The `.*` pattern would need to match `_bracketedTag/` but Go's `-run` flag treats `/` as a subtest separator and `.*` does not cross that boundary when matching from the top-level test name. The working pattern is `TestExtractSubcategory_bracketedTag/library_only`.
