# Maintainer Review: Issue #1641 (test(llm): add quality benchmark suite)

**Review Focus**: Maintainability (clarity, readability, duplication)
**Files Changed**: `internal/builders/llm_integration_test.go`, `internal/builders/baseline_test.go`
**Reviewer Lens**: Can someone who didn't write this understand it and change it with confidence?

---

## Finding 1: `reportRegressions` name-behavior mismatch

**File**: `internal/builders/llm_integration_test.go:269-301`
**Severity**: Advisory

The function is named `reportRegressions` and its doc comment says "Returns true if regressions were found." But it also returns true for orphaned entries (line 300: `return len(diff.Regressions) > 0 || len(diff.Orphaned) > 0`). A developer reading the call site at line 585 (`reportRegressions(t, baseline, results)`) would think it only concerns regressions, but orphaned baseline entries also cause a true return and `t.Errorf` calls.

This is minor because the function's return value isn't actually used by the caller (TestLLMGroundTruth ignores it -- it relies on `t.Errorf` to fail the test). But the doc comment says one thing and the code does another, which could mislead someone trying to compose this function into other logic.

**Suggestion**: Either update the doc comment to say "Returns true if regressions or orphaned entries were found" or rename to `reportBaselineDiff`. Since the return value is currently unused at the call site, this is advisory.

---

## Finding 2: `testCaseMetrics` records zero `RepairAttempts` for failures, which is ambiguous

**File**: `internal/builders/llm_integration_test.go:503-509`
**Severity**: Advisory

When `session.Generate()` fails (error path), `metrics[key]` is recorded with `RepairAttempts: 0` (implicitly, since only `Latency` is set). This zero is indistinguishable from a successful generation that needed zero repairs. The `firstTryCount` counter at line 334 counts `RepairAttempts == 0` -- so failures contribute to the "first-try rate", making it look higher than it should.

In practice this likely doesn't cause a wrong decision because the test output also separately shows the pass/fail summary. But the next person reading the benchmark summary might trust the "first-try rate" metric and draw incorrect conclusions about model quality.

**Suggestion**: Either (a) skip failed cases in the first-try count calculation by gating on a success field, or (b) set `RepairAttempts: -1` for error cases as a sentinel, or (c) add a comment explaining that failed-generation cases inflate the first-try rate. Option (c) is the minimum fix.

---

## Finding 3: Hardcoded `testIDs` slice duplicates test matrix keys

**File**: `internal/builders/llm_integration_test.go:428-435`
**Severity**: Advisory

The `testIDs` slice is a manually ordered list of test IDs that must match the keys in `llm-test-matrix.json`. If someone adds a new test case to the JSON matrix but forgets to add it to this slice, the test silently ignores it. The comment at line 427 says "Run tests in order" but doesn't explain why ordering matters. If ordering doesn't matter, iterating over `matrix.Tests` directly would eliminate the synchronization burden. If ordering does matter (e.g., for reproducing GitHub API rate limit behavior), a comment should explain why.

The next developer adding a test case to the JSON will likely miss this second location. The gap is partly mitigated because orphaned baseline detection would catch a removed test, but a *new* test in the JSON would simply never run.

**Suggestion**: Either iterate `matrix.Tests` directly (with sorted keys for determinism) or add a validation step that checks `len(testIDs) == len(matrix.Tests)` and that all testIDs exist in the matrix. A comment explaining why order matters would also help.

---

## Finding 4: `baselineDir()` fallback silently returns a potentially non-existent path

**File**: `internal/builders/llm_integration_test.go:90-104`
**Severity**: Advisory

`baselineDir()` tries two candidate paths, then falls back to `../../testdata/llm-quality-baselines` even if it doesn't exist (line 102-103). Callers like `loadBaseline()` handle the missing-file case gracefully (returns nil), and `writeBaseline()` creates the directory with `os.MkdirAll`. So this doesn't break at runtime. However, the fallback path is duplicated with `candidates[0]`, which is slightly confusing -- the next person reading this might wonder if the fallback was intentionally different.

**Suggestion**: Minor. Consider just returning `candidates[0]` in the fallback to make the intent clear, or adding a comment that the fallback is intentional.

---

## Finding 5: Constants comment about migration is helpful but incomplete

**File**: `internal/builders/llm_integration_test.go:173-179`
**Severity**: Advisory (positive note)

The comment on `baselinePass`/`baselineFail` constants noting "Changing them requires migrating existing baseline files" is exactly the kind of comment the next developer needs. Similarly, the `baselineKey` doc comment at line 227-232 warns about the format being used as both subtest names and map keys. These comments prevent the most likely maintenance mistake.

---

## Finding 6: `TestProviderModel` assertions are weak

**File**: `internal/builders/baseline_test.go:299-318`
**Severity**: Advisory

The test checks that `providerModel()` doesn't return an empty string, but doesn't check actual values. This means the test can't catch a wrong mapping (e.g., if "claude" started returning the gemini model name). The test name and structure suggest it's testing correctness, but it only tests non-emptiness. For `"unknown"`, the expected return is the provider name itself, which the test doesn't verify.

This is minor because `providerModel` is only used for human-readable logging in baseline files, not for logic decisions.

**Suggestion**: Consider asserting expected values for known providers and checking that "unknown" returns "unknown".

---

## Overall Assessment

The code is well-structured and readable. Function names, comments, and the test organization follow a clear pattern. The baseline regression system (write/load/compare/report) is well-decomposed into testable units. The `baseline_test.go` file covers the important edge cases (minimum pass rate, invalid JSON, orphaned entries, mixed regressions/improvements).

The test matrix JSON approach is clean -- separating test definitions from test logic makes it easy to add cases. The metrics logging is diagnostic-only (not assertions), which is appropriate for a benchmark suite where absolute values depend on hardware.

The main maintainability risks are (1) the hardcoded `testIDs` slice that must stay in sync with the JSON matrix, and (2) the ambiguous zero-repair-attempts count for failed cases. Neither is blocking -- they're traps for the next person to add test cases or interpret benchmark results, respectively.
