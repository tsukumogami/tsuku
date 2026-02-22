# Architect Review: Issue #1859

**Issue**: feat(dashboard): read structured subcategories with category remap fallback
**Review Focus**: architecture (design patterns, separation of concerns)
**Files Changed**: `internal/dashboard/dashboard.go`, `internal/dashboard/dashboard_test.go`, `internal/dashboard/failures.go`, `internal/dashboard/failures_test.go`

## Design Alignment

The implementation matches the design doc's Phase 4 intent precisely. The data flow is:

1. JSONL records are deserialized into `FailureRecord` / `PackageFailure` structs (which now include `Subcategory`)
2. `remapCategory()` translates old category strings at load time
3. `extractSubcategory()` is called conditionally -- only when the structured subcategory field is absent

This follows the design doc's stated flow and maintains the separation of concerns: the dashboard is a consumer of classification data, not a producer. It reads what the CLI and orchestrator wrote, remaps legacy names for consistency, and falls back on heuristics only for old records.

## Structural Assessment

### Dual FailureRecord sync: Correct

The `batch.FailureRecord` (producer side, `internal/batch/results.go:130`) has `Subcategory string` with `json:"subcategory,omitempty"`. The `dashboard.FailureRecord` (consumer side, `internal/dashboard/dashboard.go:154`) and `dashboard.PackageFailure` (consumer side, `internal/dashboard/dashboard.go:163`) both now have `Subcategory string` with `json:"subcategory,omitempty"`. All three structs are in sync. The JSON schema (`data/schemas/failure-record.schema.json:47-49`) includes `subcategory` as an optional string property. No schema drift.

### Dependency direction: Correct

`internal/dashboard` imports `internal/batch` for queue types. No reverse dependency. The dashboard package remains a leaf consumer.

### No parallel patterns introduced

`remapCategory()` is a single function with a single map. It's applied at all four load points (legacy batch and per-recipe paths in both `loadFailures()` and `loadFailureDetailsFromFile()`). There's no second remap mechanism or alternative classification path. The existing `extractSubcategory()` function is reused unchanged -- just called conditionally now.

### Placement of `remapCategory()`: Appropriate

Placed in `failures.go` alongside `extractSubcategory()`. Both functions deal with failure classification at the dashboard level. This keeps classification logic in one file rather than splitting it across `dashboard.go` and `failures.go`.

### Category remap table completeness

The `categoryRemap` map in `failures.go:54-61` covers all six old-to-new mappings from the design doc's Category Remap Table. Categories already in the canonical taxonomy (`missing_dep`, `network_error`, `install_failed`, `generation_failed`, `recipe_not_found`, `verify_failed`) pass through unchanged. This is a closed, explicit mapping -- no regex, no inference.

### Remap in both `loadFailures()` and `loadFailureDetailsFromFile()`

This is worth noting: the design doc's Phase 4 description mentions `loadFailureDetailsFromFile()` specifically, but the implementation also applies remap in `loadFailures()` (the summary-level loader). This is correct -- `loadFailures()` produces the `Failures` map (category -> count) shown on the dashboard. Without remap there, summary counts would use old names while detail records use canonical names. The issue AC explicitly requires this.

### Conditional `extractSubcategory()`: Clean implementation

The conditional logic in `loadFailureDetailRecords()` (failures.go:156-163) is straightforward:

```go
for i := range allDetails {
    if allDetails[i].Subcategory == "" {
        allDetails[i].Subcategory = extractSubcategory(...)
    }
}
```

This runs after all records are loaded but before ID generation and sorting. The positioning is correct -- subcategory needs to be populated before records are deduplicated or sorted. The subcategory is read from JSONL during `loadFailureDetailsFromFile()` and checked here at the aggregation level. No concern about ordering.

## Findings

### No blocking findings

The implementation follows the established patterns:
- Struct field additions are consistent across producer and consumer types
- No new package dependencies or import direction violations
- No bypass of existing classification flow (heuristic is still called for old records)
- No parallel pattern (one remap mechanism, one subcategory extraction flow)
- JSON schema includes the new field

### No advisory findings

The code is structurally clean. The remap function is simple, the conditional logic is clear, and the test coverage exercises both the structured passthrough and the heuristic fallback paths.

## Summary

This is a leaf-node change at the consumer end of the error classification pipeline. It adds deserialization support for fields that upstream components (#1856, #1857) already produce, applies a compatibility remap for legacy data, and makes the existing heuristic extraction conditional. The architecture of the dashboard as a read-only consumer of JSONL data is preserved. No new patterns, no dependency direction violations, no state contract drift.
