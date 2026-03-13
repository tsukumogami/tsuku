# Maintainer Review: Test Files

Reviewer perspective: can someone who didn't write this understand it and change it with confidence?

## Scope

Reviewed all test files in `internal/batch/`, `internal/seed/`, `internal/blocker/`, `cmd/batch-generate/`, `cmd/seed-queue/`, and `cmd/remediate-blockers/`. Cross-referenced against established test patterns in `internal/actions/` and `cmd/tsuku/`.

---

## Findings

### 1. Custom `contains`/`containsStr` reimplemented instead of `strings.Contains`

**Files:**
- `internal/batch/queue_entry_test.go:491-502` -- `contains()` and `searchSubstring()`
- `internal/batch/bootstrap_test.go:1039-1050` -- `containsStr()` and `searchStr()`
- `internal/builders/repair_loop_test.go:69-80` -- `containsSubstr()`

Three separate test files each reimplement substring search instead of using `strings.Contains`. The next developer will wonder if these custom implementations behave differently from `strings.Contains` (they don't). They'll also wonder whether the three copies are intentional divergences or accidental duplication.

The existing tests in `internal/actions/` and `cmd/tsuku/` use `strings.Contains` directly (e.g., `download_test.go`, `coverage_gap10_test.go`). These custom helpers break consistency with the rest of the codebase.

**Advisory.** Won't cause a bug, but the next person will waste time reading three helper implementations to confirm they do nothing special.

---

### 2. Stdout capture via `os.Pipe()` without `t.Cleanup()` restore

**Files:**
- `cmd/seed-queue/main_test.go:228-238` (TestExitCode_FatalOnBadQueueFile)
- `cmd/seed-queue/main_test.go:297-309` (TestSummaryJSON_WrittenToStdout)
- `cmd/seed-queue/main_test.go:404-412` (TestDryRun_NoFileWrites)
- `cmd/seed-queue/main_test.go:457-469` (TestDryRun_StillProducesSummaryJSON)

These tests mutate the global `os.Stdout` to capture output. The restore pattern (`w.Close()` then `os.Stdout = oldStdout`) doesn't use `t.Cleanup()`, so if a `t.Fatal` fires between the swap and the restore, stdout stays redirected for the rest of the test run. None of these tests call `t.Parallel()`, which is correct given the global mutation -- but there's no comment explaining why `t.Parallel()` is absent, so a contributor may add it later and introduce a race.

**Advisory.** The tests work as-is and correctly avoid `t.Parallel()`, but the missing `t.Cleanup()` restore is a latent bug. Add `t.Cleanup(func() { os.Stdout = oldStdout })` immediately after the swap, and add a comment explaining why `t.Parallel()` must not be used.

---

### 3. `TestExitCode_SuccessWithEmptyQueue` accepts two exit codes

**File:** `cmd/seed-queue/main_test.go:196-204`

```go
if code != 0 && code != 2 {
    t.Errorf("exit code = %d, want 0 or 2", code)
}
```

The comment says "may vary" but doesn't explain under what conditions each exit code applies. This pattern repeats in `TestDryRun_StillProducesSummaryJSON` (line 481) and `TestDryRun_CorrectExitCode` (line 519). A test that accepts multiple outcomes without explaining which conditions produce which result doesn't test behavior -- it tests "doesn't crash." The next developer can't tell if exit code 2 from this path is expected or a regression.

**Blocking.** A future change to the exit code logic could introduce exit code 2 as a new error path, and these tests would still pass, masking the regression. Either pin the expected exit code or document what determines which code is returned.

---

### 4. `computeExitCode` helper duplicates production logic

**File:** `cmd/seed-queue/main_test.go:279-288`

```go
// computeExitCode mirrors the exit code logic from execute().
func computeExitCode(summary *seed.SeedingSummary) int {
```

This function re-implements the exit code logic rather than calling the production code. If the production logic changes and someone forgets to update this mirror, the test for `TestExitCode_SelectionLogic` will silently test stale logic. The comment even says "mirrors" -- this is the kind of divergent twin that causes the next developer to fix a bug in one place and miss the other.

**Blocking.** The test is testing its own reimplementation, not the actual production code. Either extract the exit code computation into a named function in production code and call it from both places, or call `execute()` directly with controlled inputs.

---

### 5. `time.Sleep(1100ms)` in `TestWriteFailures_createsTimestampedFiles`

**File:** `internal/batch/results_test.go:76`

```go
time.Sleep(1100 * time.Millisecond)
```

The test sleeps to get different second-precision timestamps, then hedges with "collisions are possible and acceptable" (line 82). The assertion `len(files) >= 1 && len(files) <= 2` is effectively "file was created at all," which `TestWriteFailures_createsFile` already validates.

**Advisory.** This test adds 1.1 seconds to the test suite for minimal additional coverage. Consider injecting a clock or removing the test.

---

### 6. Missing `t.Parallel()` across all batch/seed test files

**Observation across:** `internal/batch/bootstrap_test.go`, `internal/batch/queue_entry_test.go`, `internal/batch/orchestrator_test.go`, `internal/batch/disambiguation_test.go`, `internal/batch/results_test.go`, `cmd/seed-queue/main_test.go`, `cmd/remediate-blockers/main_test.go`, `cmd/batch-generate/main_test.go`, `internal/seed/*.go`

None of the batch/seed tests use `t.Parallel()`. The established tests in `internal/actions/` (e.g., `download_test.go`, `util_test.go`, `coverage_gap10_test.go`) consistently call `t.Parallel()`. Most of the batch/seed tests are pure (no global state, no filesystem outside `t.TempDir()`), so they could run in parallel. The exceptions are the `cmd/seed-queue` tests that mutate `os.Stdout`.

**Advisory.** Inconsistency with established patterns. Not a correctness issue, but signals a different author convention that may confuse contributors about whether `t.Parallel()` is expected in this codebase.

---

### 7. `TestDisambiguationRecord_Fields` tests struct field assignment

**File:** `internal/batch/disambiguation_test.go:11-40`

This test creates a struct literal, then asserts that each field contains the value that was just assigned. This tests Go struct literals, not application logic. It has zero value -- if this test ever fails, Go's type system is broken.

**Advisory.** Delete this test. `TestDisambiguationRecord_JSONMarshal` already verifies the struct round-trips through JSON, which is the actual contract that matters.

---

### 8. Magic strings for circuit breaker and failure category states

**Files:**
- `cmd/batch-generate/main_test.go`: `"open"`, `"half-open"`, `"closed"` as raw strings throughout
- `cmd/remediate-blockers/main_test.go:88`: `"missing_dep"`, `"recipe_not_found"`, `"validation_failed"` as raw strings

The queue entry tests in `internal/batch/queue_entry_test.go` correctly use constants (`StatusPending`, `StatusFailed`, `ConfidenceAuto`, etc.). But the circuit breaker tests and remediation tests use raw string literals. If someone renames a state value, the production code changes but these tests silently pass with stale values.

**Advisory.** Define constants for circuit breaker states and failure categories, or at minimum use the existing constants where they exist and add local constants where they don't.

---

### 9. `TestRemediateQueue_noChanges` relies on mod-time comparison

**File:** `cmd/remediate-blockers/main_test.go:443-444`

```go
info2, _ := os.Stat(queuePath)
if info2.ModTime() != info1.ModTime() {
```

File modification time has platform-dependent precision (1 second on many Linux filesystems with ext4). A no-op write-same-bytes could still update the mod time on some filesystems, or a fast rewrite could keep the same mod time if within the same second. The assertion doesn't reliably prove "file was not rewritten."

**Advisory.** Compare file contents byte-for-byte instead, or verify via a write-counting wrapper.

---

## What's Good

- **Table-driven tests are well-structured.** `TestExtractDeps`, `TestIsValidDependencyName`, `TestSourceFromStep_AllActions`, `TestQueueEntry_Validate_Valid`, and `TestQueueEntry_Validate_InvalidPriority` all follow idiomatic Go table-driven patterns with clear case names.

- **Test helpers use `t.Helper()`.** The `writeFile`, `readQueue`, `writeQueue`, and `entryByName` helpers all correctly call `t.Helper()`, so failure messages point to the right call site.

- **Bootstrap integration test is excellent.** `TestBootstrap_FullMigration` sets up the full three-source pipeline with intentional duplicates and verifies precedence rules. The comments explain why each assertion matters ("from recipe, not curated"). A new contributor can read this single test and understand the merge semantics.

- **Idempotency test is valuable.** `TestRemediateFile_idempotent` runs remediation twice and verifies the second run makes no changes. This catches real regressions.

- **Security boundary tests exist.** Path traversal tests in both `TestIsValidDependencyName` and `TestQueueEntry_Validate_pathTraversalEcosystem` verify that hostile input is rejected. These are well-named and make the security contract explicit.

- **Test naming is consistent.** Most tests follow `TestFunctionName_scenario` or `TestType_Method_Scenario` naming. Subtest names in table-driven tests are descriptive and lowercase.

---

## Summary

| Severity | Count | Key items |
|----------|-------|-----------|
| Blocking | 2 | Ambiguous exit code assertions mask regressions (#3); duplicated exit code logic tests itself instead of production code (#4) |
| Advisory | 7 | Reimplemented `strings.Contains` (#1); unsafe stdout capture (#2); sleep-based test (#5); missing `t.Parallel()` (#6); dead struct-field test (#7); magic strings (#8); unreliable mod-time assertion (#9) |
