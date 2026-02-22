# Pragmatic Review: Issue #1859

**Issue**: feat(dashboard): read structured subcategories with category remap fallback
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Reviewer**: pragmatic-reviewer

## Diff Scope

Files changed:
- `internal/dashboard/dashboard.go` (struct fields, `loadFailures()` remap calls)
- `internal/dashboard/dashboard_test.go` (updated test expectations for remapped categories)
- `internal/dashboard/failures.go` (`categoryRemap`, `remapCategory()`, subcategory passthrough, conditional extraction)
- `internal/dashboard/failures_test.go` (5 new tests for remap + subcategory passthrough)

## Findings

### No blocking findings.

### No advisory findings.

## Analysis

**`remapCategory()` and `categoryRemap` map**: A 6-entry map and a 5-line function. This is the simplest correct way to handle backward compatibility with old category strings. No abstraction layers, no interface, no registry pattern. The map is a package-level `var` -- readable, grep-able, and trivially extensible if new old categories surface. No over-engineering here.

**Conditional `extractSubcategory()` call**: A single `if allDetails[i].Subcategory == ""` guard. This is the minimum viable change to prefer structured data over heuristic extraction. No new types, no strategy pattern, no configuration.

**Subcategory passthrough in `loadFailureDetailsFromFile()`**: Two lines added (one per format path): `Subcategory: f.Subcategory` and `Subcategory: record.Subcategory`. Minimal, correct.

**Remap applied in `loadFailures()` too**: This is necessary for consistency -- the summary counts (`Failures` map in dashboard.json) would show old names while detail records show new names if remap were only in `loadFailureDetailsFromFile()`. Not scope creep; it's required for correctness.

**Test additions**: 5 new tests are focused and necessary -- they test the new behavior (structured subcategory passthrough for both formats, heuristic fallback, remap function, end-to-end mixed records). Test expectations in existing tests were updated to reflect remapped categories (e.g., `api_error` -> `network_error`). No gold-plated test infrastructure.

**No speculative generality**: The `categoryRemap` map only contains entries for categories that exist in current data (confirmed in the design doc's analysis of JSONL contents). No "just in case" mappings.

**No unnecessary abstractions**: `remapCategory()` is called from 4 sites. It's not a single-caller helper. It's the right abstraction level for a lookup table.

The implementation is the simplest correct approach for the requirements.
