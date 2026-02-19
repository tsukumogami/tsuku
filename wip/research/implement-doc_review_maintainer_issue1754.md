# Maintainer Review: Issue #1754

## Findings

### 1. Baseline key is an implicit contract between test runner and baseline file - BLOCKING

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:318`

The subtest name (which becomes the baseline key) is constructed as `testID + "_" + tc.Tool` (e.g., `"llm_github_stern_baseline_stern"`). This format is not documented anywhere and is consumed by three separate concerns: `compareBaseline`, `writeBaseline`, and the committed JSON baseline files.

The next developer who renames a test ID in `llm-test-matrix.json` or changes the tool name will silently break baseline matching. `compareBaseline` (line 183) silently skips any key present in the baseline but missing from results -- so a renamed test looks like "no regressions" when actually the old test disappeared and the new one has no baseline. A previously-failing test that gets renamed shows as clean.

Either extract the key construction into a named function (e.g., `baselineKey(testID, tc)`) with a comment explaining why the format matters, or at minimum add a warning when a baseline key has no corresponding result (the inverse of the current skip on line 183-185).

### 2. Magic strings "pass" and "fail" used as status values without constants - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:137,186,189,379,381`

The strings `"pass"` and `"fail"` appear in 6 locations across two files and are also serialized into the JSON baseline files. A typo like `"passed"` or `"PASS"` would silently break comparison logic -- `compareBaseline` only checks exact `"pass"` and `"fail"`, so any other value is silently treated as neither regression nor improvement.

With only two valid values this is a bounded risk, but named constants (`statusPass`, `statusFail`) would make the contract explicit and let the compiler catch typos.

### 3. Dead code: TSUKU_BASELINE_DIR env var saved and restored but never read - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/baseline_test.go:12-17`

`TestWriteBaseline_MinimumPassRate` saves and restores the `TSUKU_BASELINE_DIR` environment variable, but `baselineDir()` in `llm_integration_test.go` (line 87-101) never reads this env var -- it uses hardcoded relative path candidates. The save/restore code is dead and misleading: the next developer will think `TSUKU_BASELINE_DIR` controls the baseline directory and try to use it elsewhere, then be confused when it has no effect.

The comment on line 19-21 correctly says "We call writeBaselineToDir directly", which makes the env var handling unnecessary. Remove the `origDir` save/restore block.

### 4. containsFeature does prefix matching but looks like equality check - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:550-557`

`containsFeature(features, "patches:")` uses prefix matching (line 553: `f[:len(prefix)] == prefix`), meaning `containsFeature(features, "patches:url")` would also match a feature called `"patches:url_encoded"` if one were added later. The function name `containsFeature` suggests membership checking, not prefix matching. The callers happen to use it correctly because the current feature names don't overlap, but the next developer adding a feature like `"patches:urls"` alongside `"patches:url"` would get a false match.

Rename to `hasFeaturePrefix` or use `strings.HasPrefix` to make the intent obvious. Alternatively, since all current callers use exact colon-delimited prefixes, document that the matching is prefix-based.

### 5. Hand-rolled substring search instead of strings.Contains - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/baseline_test.go:315-326`

`containsSubstring` and `searchSubstring` re-implement `strings.Contains` from the standard library. This is unusual in Go and will make the next developer wonder if there's a reason (Unicode edge case? performance?). There isn't -- `strings.Contains` handles the same cases. Replace with `strings.Contains` to remove the "why is this different?" question.

### 6. baselineDir silently swallows filepath.Abs error - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:94-95,99-100`

`baselineDir()` calls `filepath.Abs(c)` and discards the error with `abs, _ := filepath.Abs(c)`. If `Abs` fails (unlikely but possible with unusual working directories), the function silently returns an empty or garbage path, and the subsequent `loadBaseline`/`writeBaseline` calls will fail with a confusing "file not found" error that doesn't point to the real problem. Since this is test code and `Abs` rarely fails, this is low-risk, but logging the error or returning it would save debugging time.

### 7. Hardcoded test ID ordering duplicates matrix keys - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:298-306`

The `testIDs` slice hardcodes every test ID from the matrix JSON. When someone adds a test to `llm-test-matrix.json`, they must also add it here -- and there's no compile-time or runtime check that the two are in sync. A test added to the JSON but missing from `testIDs` will silently not run.

The comment at line 297 ("Run tests in order") explains why: deterministic ordering. Consider iterating `matrix.Tests` with sorted keys and documenting the ordering guarantee, or at minimum add a check that `len(testIDs) == len(matrix.Tests)` so a mismatch between the list and the JSON produces a clear failure.

## Summary

Blocking: 1, Advisory: 6

The overall structure is well-organized. The provider detection flow (priority chain with clear comments), the separation of `compareBaseline` from `reportRegressions`, and the `*ToDir` variants for testability are all solid patterns that a new contributor can follow. The blocking finding is about the invisible coupling between test key format and baseline file format -- a renamed test silently passes regression checks, which is exactly the kind of mistake this test infrastructure is meant to catch.
