# Maintainer Review: Issue #1856

**Issue**: #1856 (feat(cli): add subcategory to install error JSON output)
**Review focus**: maintainability (clarity, readability, duplication)
**Files in scope**: `cmd/tsuku/install.go`, `cmd/tsuku/install_test.go`

## Finding 1: Cross-reference NOTE will become stale after #1857

**File**: `cmd/tsuku/install.go:343-348`
**Severity**: Advisory

The NOTE comment on the CLI's `categoryFromExitCode()` references the orchestrator's current category strings:

```go
// NOTE: A separate categoryFromExitCode() exists in internal/batch/orchestrator.go
// with different category strings. That version maps exit codes to pipeline/dashboard
// categories (e.g., "api_error", "validation_failed") used for batch queue
// classification and the pipeline dashboard.
```

Issue #1857 (the next issue in this sequence) will rename the orchestrator's categories from `"api_error"` to `"network_error"` and `"validation_failed"` to `"install_failed"`. After that rename, the CLI's NOTE will cite category names that no longer exist in the orchestrator code. A symmetric NOTE exists on the orchestrator's `categoryFromExitCode()` at `internal/batch/orchestrator.go:477-482`.

This is not a bug, but it's a stale-comment trap. The next person who reads the CLI's NOTE will go look for `"api_error"` in the orchestrator code and not find it. The fix is either: (a) update the NOTE in #1857 when the rename happens, or (b) remove the specific example strings from the NOTE and just say "different category strings optimized for pipeline operations." Option (b) is more durable since it won't go stale with future renames.

## Finding 2: Subcategory strings are inline magic values

**File**: `cmd/tsuku/install.go:315-321`
**Severity**: Advisory

The subcategory strings (`"timeout"`, `"dns_error"`, `"tls_error"`, `"connection_error"`) appear as inline string literals in `classifyInstallError()` and are repeated in test assertions in `cmd/tsuku/install_test.go`. These same strings will appear again in #1857 (`parseInstallJSON` extraction) and in the dashboard's `extractSubcategory()` heuristic fallback (`internal/dashboard/failures.go` already has `"timeout"` as a known subcategory at line 48).

Currently there are 4 subcategory values in 2 files (source + test), totaling 8 occurrences. After #1857 and #1859 land, these strings will appear in at least 4 more files. A typo in any one location would silently break subcategory matching.

This doesn't block merge today because the scope is small (2 files, same package). But the design doc's stated intent is for these strings to become a machine-readable API contract. Consider defining them as constants (e.g., `SubcategoryTimeout = "timeout"`) in a shared location before #1857 introduces more consumers. The exit code constants (`ExitNetwork`, `ExitRecipeNotFound`) already follow this pattern.

## Finding 3: Test names accurately describe behavior

**File**: `cmd/tsuku/install_test.go`
**Severity**: N/A (positive observation)

`TestClassifyInstallError` tests both exit codes and subcategories for all 12 error scenarios. `TestInstallErrorJSON` is split into "without subcategory" and "with subcategory" subtests that clearly document the omitempty contract. Test case names like "DNS registry error" and "dependency failure wrapping RegistryError NotFound" describe the exact error scenario, not just what's being tested. The next developer can read the test names to understand the subcategory mapping without reading the implementation.

## Finding 4: classifyInstallError comment explains ordering constraint

**File**: `cmd/tsuku/install.go:296-301`
**Severity**: N/A (positive observation)

The godoc comment on `classifyInstallError()` explicitly explains why the dependency wrapper string check comes before the `errors.As()` type check:

```go
// It checks the dependency wrapper string before using typed error unwrapping,
// because a dependency failure wrapping a RegistryError should be classified
// as a dependency error (exit 8), not by the inner error's type (exit 3).
```

This is good. Without this comment, the next person would see `strings.Contains(err.Error(), "failed to install dependency")` before the typed error check and wonder if reordering would be cleaner. The comment preempts that question.

## Summary

No blocking findings. The implementation is clean, well-tested, and the function-level documentation is better than average -- particularly the ordering constraint comment and the cross-reference NOTEs.

Two advisory items:

1. The cross-reference NOTE on `categoryFromExitCode()` (line 343) cites orchestrator category names (`"api_error"`, `"validation_failed"`) that will be renamed in the very next issue (#1857). The #1857 implementer should update or generalize this comment to avoid a stale cross-reference.

2. The subcategory strings are inline literals. With only 2 files today this is fine, but the design calls for these strings to appear in 4+ more files across 3 packages. Consider extracting constants before #1857 adds more consumers.
