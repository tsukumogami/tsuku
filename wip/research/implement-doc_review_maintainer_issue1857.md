# Maintainer Review: #1857 feat(batch): normalize pipeline categories and add subcategory passthrough

## Files Changed

- `internal/batch/orchestrator.go`
- `internal/batch/orchestrator_test.go`
- `internal/batch/results.go`
- `data/schemas/failure-record.schema.json`

---

## Finding 1: Stale comment in CLI's categoryFromExitCode references old orchestrator category names

**File**: `cmd/tsuku/install.go:343-348`
**Severity**: Advisory

The NOTE comment in the CLI's `categoryFromExitCode()` still says the orchestrator version maps to `"api_error"`, `"validation_failed"`. Those names were just renamed in this PR to `"network_error"`, `"install_failed"`, `"verify_failed"`, and `"generation_failed"`. The comment now lies about what the other function returns.

```go
// NOTE: A separate categoryFromExitCode() exists in internal/batch/orchestrator.go
// with different category strings. That version maps exit codes to pipeline/dashboard
// categories (e.g., "api_error", "validation_failed") used for batch queue
```

The next developer reading this comment will think the orchestrator uses `api_error` and `validation_failed`, then be confused when they find `network_error` and `install_failed` instead.

**Suggestion**: Update the comment examples to match the current canonical taxonomy. The orchestrator's version already has a correct cross-reference comment pointing back to the CLI, so both sides should agree.

---

## Finding 2: Hardcoded "network_error" in generate() post-retry fallback bypasses categoryFromExitCode

**File**: `internal/batch/orchestrator.go:373`
**Severity**: Advisory

In `generate()`, the fallback after exhausting retries hardcodes `Category: "network_error"` instead of calling `categoryFromExitCode(ExitNetwork)`. The non-retry path at line 352 correctly calls `categoryFromExitCode(exitCode)`. If the canonical name for exit code 5 ever changes, the non-retry path would update automatically but the retry-exhausted path would not.

```go
return generateResult{
    Err: lastErr,
    Failure: FailureRecord{
        PackageID: pkg.Source,
        Category:  "network_error",  // hardcoded, not via categoryFromExitCode
```

This is functionally correct today because the only way to reach this code is when `exitCode == ExitNetwork`, and `categoryFromExitCode(5)` returns `"network_error"`. But it's a maintenance trap: the contract says "categoryFromExitCode is the single authority" and then this spot sidesteps it.

**Suggestion**: Replace with `categoryFromExitCode(ExitNetwork)` for consistency. The intent stays clear and the function lives up to its "single source of truth" claim.

---

## Finding 3: Test uses legacy category name "api_error" in internal/batch package

**File**: `internal/batch/results_test.go:66`
**Severity**: Advisory

`TestWriteFailures_createsTimestampedFiles` uses `Category: "api_error"` as test data. While this test is exercising file creation (not category logic), using a deprecated category name in the same package where the canonical taxonomy was just defined creates confusion. The next person reading this test will wonder if `api_error` is still a valid category.

This is a pre-existing line, not introduced in this PR. Noting it because the rename in this PR makes it newly misleading -- it was accurate before the rename.

**Suggestion**: Could be updated to use a canonical name in this PR or left for #1859. Not blocking since the test isn't validating categories.

---

## Finding 4: categoryFromExitCode comment quality is excellent

**File**: `internal/batch/orchestrator.go:473-508`
**Severity**: N/A (positive)

The godoc on the orchestrator's `categoryFromExitCode()` is well done. It documents the full taxonomy, explains the exit code mappings, notes the relationship to retry/circuit breaker logic, and has a clear cross-reference to the CLI's version. The inline exit code comments (`// ExitRecipeNotFound (from cmd/tsuku/exitcodes.go)`) are a good touch for a function that uses raw integers.

---

## Finding 5: parseInstallJSON signature and comment are clear about ownership

**File**: `internal/batch/orchestrator.go:439-451`
**Severity**: N/A (positive)

The function comment explicitly states that the pipeline category is "always derived from categoryFromExitCode() rather than trusting the CLI's category string, which uses a separate user-facing taxonomy." This is exactly the kind of comment that prevents the next developer from "fixing" it to use the CLI's category.

The three-value return `(category, subcategory, blockedBy)` is at the reasonable limit for Go. The test `TestParseInstallJSON` covers the key case where CLI category differs from exit code mapping (the "CLI category ignored when it differs" test case at line 693), which demonstrates the design intent clearly.

---

## Finding 6: Schema enum and Go code are in sync

**File**: `data/schemas/failure-record.schema.json:37-44`, `internal/batch/orchestrator.go:491-507`
**Severity**: N/A (positive)

The six canonical categories in the JSON schema enum match exactly what `categoryFromExitCode()` can produce. No old names remain in the schema. The `subcategory` field is correctly optional (not in `required`), and `additionalProperties: false` ensures no unexpected fields sneak in.

---

## Finding 7: generate() and validate() have intentional structural differences that are documented

**File**: `internal/batch/orchestrator.go:321-437`
**Severity**: N/A (positive)

`generate()` and `validate()` look similar at a glance but differ in meaningful ways: generate uses `CombinedOutput()` while validate splits `Stdout`/`Stderr`; generate calls `extractBlockedByFromOutput()` on raw output while validate calls `parseInstallJSON()` on structured JSON. The code comments explain why: generate doesn't get CLI JSON (it runs `tsuku create`, not `tsuku install --json`), so it falls back to regex extraction. The validate path gets structured JSON and uses `parseInstallJSON()` to extract subcategory. These differences are justified and clear.

---

## Summary

The implementation is clean and well-documented. The new canonical taxonomy is consistently applied through `categoryFromExitCode()`, and the cross-reference comments between the CLI and orchestrator versions of this function make the intentional divergence legible.

Two advisory items worth acting on:
1. The CLI's `categoryFromExitCode` comment at `cmd/tsuku/install.go:343-348` still references the old orchestrator names (`api_error`, `validation_failed`). This is technically outside the diff but was made stale by this change. Update the examples in the NOTE to match the new names.
2. The hardcoded `"network_error"` at `orchestrator.go:373` could use `categoryFromExitCode(ExitNetwork)` for consistency with the "single authority" principle.

Neither is blocking. The code reads well and the test coverage captures the critical design decisions (especially the "CLI category ignored when it differs" test case).
