# Architect Review: Issue #1857

**Issue**: feat(batch): normalize pipeline categories and add subcategory passthrough
**Focus**: architecture (design patterns, separation of concerns)
**Files changed**: `internal/batch/orchestrator.go`, `internal/batch/orchestrator_test.go`, `internal/batch/results.go`, `data/schemas/failure-record.schema.json`

---

## Finding 1: Stale cross-reference comment in CLI's categoryFromExitCode

**File**: `cmd/tsuku/install.go:343-348`
**Severity**: Advisory

The CLI's `categoryFromExitCode()` comment at line 343-348 says:

```go
// NOTE: A separate categoryFromExitCode() exists in internal/batch/orchestrator.go
// with different category strings. That version maps exit codes to pipeline/dashboard
// categories (e.g., "api_error", "validation_failed") used for batch queue
// classification and the pipeline dashboard.
```

Issue #1857 renamed the orchestrator's categories: `api_error` is now `network_error`, and `validation_failed` is now `install_failed`. The CLI comment still cites the old names. This was flagged as an advisory risk in the maintainer review of #1856, with the expectation that #1857 would update it.

The orchestrator's symmetric NOTE (lines 487-490) was updated correctly -- it no longer cites specific CLI category names, making it durable against future renames.

**Suggestion**: Update the CLI's NOTE to either (a) use the new names, or (b) follow the orchestrator's approach and remove specific example strings entirely, just saying "different category strings optimized for pipeline operations." Option (b) is more durable.

This is advisory because it's a stale comment in a file outside the diff scope (`cmd/tsuku/install.go` was not changed by #1857). The comment won't cause a bug, but the next person who reads it will go looking for `"api_error"` in the orchestrator and not find it.

---

## Structural Assessment

### Design alignment: Strong

The implementation follows the design doc's architecture precisely:

- **Orchestrator owns pipeline categories**: `categoryFromExitCode()` is the single source of truth. `parseInstallJSON()` no longer trusts the CLI's category string -- it calls `categoryFromExitCode(exitCode)` unconditionally. This is the core behavioral fix the design requires.

- **CLI owns subcategories**: The orchestrator extracts `result.Subcategory` from CLI JSON and passes it through to `FailureRecord`. It does not interpret, validate, or transform the subcategory value. Clean boundary.

- **Generate path has no subcategory**: The `generate()` method doesn't call `parseInstallJSON()` and doesn't set `Subcategory` on `FailureRecord`. This matches the design's data flow: generate-path subcategories come from dashboard heuristics, not CLI output.

### Pattern consistency: Good

- The `parseInstallJSON()` function signature change from `(string, []string)` to `(string, string, []string)` follows Go conventions for multi-return functions. All three callers within `validate()` are updated.

- `FailureRecord.Subcategory` uses `json:"subcategory,omitempty"` matching the existing pattern for optional fields (`BlockedBy` uses `omitempty` too).

- The canonical taxonomy is defined in one place (`categoryFromExitCode`) and enforced at the schema level (JSON schema enum). No parallel definition.

### Dual categoryFromExitCode: Intentionally maintained

The CLI's `categoryFromExitCode()` (`cmd/tsuku/install.go:349`) and the orchestrator's `categoryFromExitCode()` (`internal/batch/orchestrator.go:491`) remain separate functions with different return values. This is by design (my memory notes confirm: "Intentionally divergent. Do NOT flag as duplication"). The CLI produces user-facing categories (`network_error`, `install_failed`, `recipe_not_found`, `missing_dep`); the orchestrator produces pipeline categories with additional granularity (`verify_failed`, `generation_failed`). The design doc explicitly addresses and rejects unifying them (Decision 1, "Alternatives Considered").

### Schema contract: Correctly maintained

The JSON schema (`data/schemas/failure-record.schema.json`) uses `additionalProperties: false`, which means adding `subcategory` to `FailureRecord` without also adding it to the schema would cause validation failures. The implementation adds both: the Go struct field (line 133 of `results.go`) and the schema property (lines 47-49 of the schema). The `category` enum is updated to the six canonical values. The schema and Go struct are in sync.

### Dashboard struct drift: Expected, scoped to #1859

The dashboard's `FailureRecord` (`internal/dashboard/dashboard.go:143`) and `PackageFailure` (`internal/dashboard/dashboard.go:159`) do not yet have `Subcategory` fields. This is by design -- the dashboard update is #1859's scope. Go's `json.Unmarshal` silently ignores unknown fields, so JSONL records with `subcategory` will deserialize without error until #1859 adds the field.

### Dependency direction: No violations

`internal/batch/orchestrator.go` imports only standard library packages. No upward dependency on `cmd/` or cross-package import issues.

### No parallel patterns introduced

The change doesn't introduce any new dispatch mechanism, parser, or classification approach. It modifies the existing `categoryFromExitCode()` and `parseInstallJSON()` functions in place. The subcategory passthrough is a data field addition, not a new behavioral pattern.

---

## Summary

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 1 |

The implementation structurally matches the design doc's intent. The separation of concerns -- orchestrator owns pipeline categories, CLI owns subcategories -- is clean. The single source of truth for pipeline categories (`categoryFromExitCode`) is established and enforced at the schema level. No dependency violations, no parallel patterns, no schema drift. The one advisory finding is a stale cross-reference comment in `cmd/tsuku/install.go` (outside the diff) that still cites the old orchestrator category names.
