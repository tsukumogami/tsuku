# Maintainer Review: Issue #2133

**Focus**: Clarity, readability, duplication -- can someone who didn't write this understand it and change it with confidence?

## Files Reviewed

- `internal/discover/chain_coverage_test.go`
- `internal/verify/coverage2_test.go`
- `internal/verify/deps_coverage_test.go`
- `internal/verify/dltest_coverage_test.go`
- `internal/verify/external_coverage_test.go`
- `internal/verify/header_coverage_test.go`

---

## Findings

### 1. Divergent twins: `TestStageMiss_TwoRemaining` and `TestStageMiss_OneRemaining` are identical setups with different assertions

**File**: `internal/discover/chain_coverage_test.go:95-137`
**Severity**: Advisory

These two tests create the exact same 3-resolver chain (miss, miss, LLM hit) and differ only in which log string they check: `"probing package ecosystems"` vs `"web search"`. The names suggest they test different states (two remaining vs one remaining) but the setup is identical -- both have a 3-stage chain with 2 misses followed by a hit.

The next developer will look at these side-by-side and wonder: are these actually testing different chain positions, or did someone copy-paste and forget to change the setup? Both assertions pass because the single Resolve call produces log output containing *both* strings (first miss logs "probing package ecosystems", second miss logs "web search"). The tests aren't wrong, but the divergent names + identical setup is a trap.

**Suggestion**: Combine into a single test that asserts both log messages, or add a comment explaining that the same chain run produces both log lines and each test isolates one.

### 2. Reinvented `strings.Contains` as `containsStr` in `dltest_coverage_test.go`

**File**: `internal/verify/dltest_coverage_test.go:163-170`
**Severity**: Advisory

The file defines a `containsStr` helper that reimplements `strings.Contains` with a manual loop. The existing test file `dltest_test.go` already imports `strings` and uses `strings.Contains` extensively (lines 873, 879, 933, 961, 978, 991, 1036, 1208, 1211). A next developer adding tests to this package will see two different ways to check substring containment and wonder which to use.

**Suggestion**: Import `strings` and use `strings.Contains` like the rest of the package.

### 3. Magic index `e[:11]` and `e[:16]` for env var prefix matching

**File**: `internal/verify/dltest_coverage_test.go:83-91`
**Severity**: Advisory

The test checks for `LD_PRELOAD=` by slicing `e[:11]` and for `LD_LIBRARY_PATH=` by slicing `e[:16]`. These are correct (11 chars and 16 chars respectively), but the next developer has to count characters to verify them. The existing `dltest_test.go` uses `strings.Contains` for similar checks. Using `strings.HasPrefix(e, "LD_PRELOAD=")` would be self-documenting.

**Suggestion**: Use `strings.HasPrefix` instead of manual length checks.

### 4. Duplicate test coverage for `splitIntoBatches` and `sanitizeEnvForHelper`

**File**: `internal/verify/dltest_coverage_test.go:38-101`
**Severity**: Advisory

`TestSplitIntoBatches` in this file covers the same cases as 6 existing tests in `dltest_test.go:314-391` (empty, single batch, exact split, uneven, one-per-batch, zero batch size). The new table-driven version is actually more concise and adds the "negative batch size" case. However, having both means the next developer modifying `splitIntoBatches` needs to update tests in two files.

Similarly, `TestSanitizeEnvForHelper` duplicates coverage from `dltest_test.go:788-890` (4 existing tests covering dangerous vars, safe vars, library paths).

These aren't wrong -- both sets pass -- but the duplication means the next developer changing these functions will find tests in two places and may only update one.

**Suggestion**: If the intent is to supersede the old tests with the more concise table-driven versions, remove the old ones. If the intent is additive (covering edge cases the old tests missed), add only the missing cases (e.g., "negative batch size") and note which existing tests cover the baseline.

### 5. `coverage2_test.go` file name is opaque

**File**: `internal/verify/coverage2_test.go`
**Severity**: Advisory

The name `coverage2_test.go` gives no hint about what aspects of `verify` it covers. It contains tests for `ValidationError`, `VerifyIntegrity`, `ExpandPathVariables`, `ValidateHeader`, `validateSingleDependency`, `IsPathVariable`, `readMagicForRpath`, and `readMagicForSoname`. The "2" suffix suggests it was the second coverage file created, not what it tests. In contrast, the other new files (`deps_coverage_test.go`, `header_coverage_test.go`, `external_coverage_test.go`, `dltest_coverage_test.go`) have names that tell you what subsystem they exercise.

The next developer looking for where to add a new test for `ExpandPathVariables` won't think to look in `coverage2_test.go`.

**Suggestion**: Split or rename. The tests naturally group into: validation-error tests, header-validation tests (overlaps with `header_coverage_test.go`), path-expansion tests, and integrity tests. Even `verify_misc_coverage_test.go` would be more descriptive.

### 6. Test name `TestErrorCategory_String_AllTier2` references an unexplained "Tier2"

**File**: `internal/verify/header_coverage_test.go:294`
**Severity**: Advisory

The test is named `AllTier2` but there's no explanation of what "Tier 2" means in this context. Looking at the error categories being tested (`ErrABIMismatch`, `ErrUnknownDependency`, `ErrRpathLimitExceeded`, etc.), these appear to be the less common error categories not covered by other tests. But the name implies a classification system ("Tier 1" vs "Tier 2") that isn't documented anywhere in the test or the production code.

The next developer will wonder: is there a Tier 1 test? What's the classification? Is this important for error handling priority?

**Suggestion**: Rename to something like `TestErrorCategory_String_RemainingCategories` or add a comment explaining the grouping.

---

## Overall Assessment

The tests are structurally sound. They follow existing patterns (table-driven tests, proper use of `t.TempDir()`, `t.Skip()` for environment-dependent tests). The assertions are reasonable and the test names mostly describe what they verify.

The main risk for the next developer is the duplication in `dltest_coverage_test.go` where `splitIntoBatches` and `sanitizeEnvForHelper` have full test suites in both the new and existing files. This is the most likely source of confusion: someone changes the function, updates the tests they find in `dltest_test.go`, CI passes, and they never know about the parallel tests in `dltest_coverage_test.go` (or vice versa). Nothing here rises to blocking.
