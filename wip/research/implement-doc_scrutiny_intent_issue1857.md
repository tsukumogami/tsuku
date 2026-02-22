# Scrutiny Review: Intent -- Issue #1857

## Issue

feat(batch): normalize pipeline categories and add subcategory passthrough

## Scrutiny Focus

intent

## Files Changed

- `internal/batch/orchestrator.go`
- `internal/batch/orchestrator_test.go`
- `internal/batch/results.go`
- `data/schemas/failure-record.schema.json`
- `docs/designs/DESIGN-structured-error-subcategories.md` (status update only)

---

## Sub-check 1: Design Intent Alignment

### Design Doc Section: "Solution Architecture > Key Changes > `internal/batch/orchestrator.go`"

The design specifies:
1. `categoryFromExitCode()` updated to use the canonical taxonomy.
2. `parseInstallJSON()` extracts subcategory from CLI JSON but uses `categoryFromExitCode(result.ExitCode)` for pipeline category instead of trusting the CLI's category string.
3. The `generate()` path already uses `categoryFromExitCode()` -- it gets updated names.

**Assessment: Aligned.** The diff shows:
- `categoryFromExitCode()` updated: exit 5 returns `network_error` (was `api_error`), exit 6 returns `install_failed` (was `validation_failed`), exit 7 returns `verify_failed` (was `validation_failed`), exit 9 returns `generation_failed` (was `deterministic_insufficient`), default returns `generation_failed` (was `validation_failed`). Matches the canonical taxonomy table in the design exactly.
- `parseInstallJSON()` now always calls `categoryFromExitCode(exitCode)` first, ignoring `result.Category`. The old code used `result.Category` from CLI JSON when available. This is the core fix the design describes.
- `generate()` retry-exhaustion fallback at line 372 changed from `"api_error"` to `"network_error"`.
- `generate()` blocked-by override condition updated from `category == "validation_failed"` to `category == "install_failed"` -- correct since exit 6 now maps to `install_failed`.

### Design Doc Section: "Solution Architecture > Key Changes > `internal/batch/results.go`"

The design specifies: `FailureRecord` gains a `Subcategory string` field.

**Assessment: Aligned.** The diff adds `Subcategory string \`json:"subcategory,omitempty"\`` to `FailureRecord`. Tag uses `omitempty` as specified in the design.

### Design Doc Section: "Solution Architecture > Key Changes > `data/schemas/failure-record.schema.json`"

The design specifies: Add `subcategory` as an optional property (the schema uses `additionalProperties: false`).

**Assessment: Aligned.** The diff updates the `category` enum to the six canonical values and adds `subcategory` as an optional string property. The schema uses `additionalProperties: false`, so adding the property was necessary for the field to pass validation.

### Design Doc Section: "Data Flow"

The design describes:
```
Orchestrator (validate path):
  parseInstallJSON() -> extracts subcategory + blockedBy from CLI JSON
  categoryFromExitCode(exitCode) -> canonical pipeline category
```

**Assessment: Aligned.** The `validate()` method passes stdout and exitCode to `parseInstallJSON()`, which returns `(category, subcategory, blockedBy)`. Both the non-retry early exit and the retry-exhaustion path populate `Subcategory` in the `FailureRecord`.

### Design Doc Section: "Data Flow" -- generate path

The design describes:
```
Orchestrator (generate path):
  categoryFromExitCode(exitCode) -> canonical pipeline category
  subcategory left empty (dashboard heuristic handles it)
```

**Assessment: Aligned.** The `generate()` method does not call `parseInstallJSON()`. It uses `categoryFromExitCode(exitCode)` directly. The `FailureRecord` it constructs does not set `Subcategory`, so it remains empty. The retry-exhaustion fallback hardcodes `Category: "network_error"` without a subcategory.

### `installResult` struct

The design mentions `installResult` adding a `Subcategory` field so `json.Unmarshal` can extract it from CLI JSON. The diff adds `Subcategory string \`json:"subcategory"\`` to `installResult`. Aligned.

### Comment update in `categoryFromExitCode()`

The design's canonical taxonomy table lists the six categories with their exit codes and meanings. The implementation's comment block mirrors this table. The old comment references `api_error` and `validation_failed` are updated. Aligned.

---

## Sub-check 2: Cross-Issue Enablement

### Downstream #1858: fix(ci): align batch workflow category names with canonical taxonomy

**What #1858 needs from #1857:** The canonical taxonomy must be defined and in use by the orchestrator. #1858 updates inline jq in `batch-generate.yml` to match.

**Assessment: Sufficient foundation.** #1857 establishes the canonical taxonomy in `categoryFromExitCode()` and the JSON schema. The six canonical category names (`recipe_not_found`, `network_error`, `install_failed`, `verify_failed`, `missing_dep`, `generation_failed`) are now the single source of truth. The schema enum enforces them. #1858 only needs to update jq strings to match -- no structural changes are needed from #1857 that are missing.

### Downstream #1859: feat(dashboard): read structured subcategories with category remap fallback

**What #1859 needs from #1857:**
1. `FailureRecord` in `results.go` must have a `Subcategory` field -- **present**.
2. Canonical category names must be in use so the dashboard knows what to remap old names to -- **present** (the six canonical names are defined and used).
3. The `subcategory` field must appear in JSONL output -- **present** (`FailureRecord` has `json:"subcategory,omitempty"` which means `WriteFailures()` will include it when non-empty).
4. The JSON schema must accept the `subcategory` field -- **present** (schema updated with the property).

**Assessment: Sufficient foundation.** All four prerequisites for #1859 are met. The dashboard issue (#1859) needs `FailureRecord` and `PackageFailure` structs in `internal/dashboard/dashboard.go` to add `Subcategory` -- those are dashboard-side structs, not batch-side, so they're correctly scoped to #1859. The `remapCategory()` function is also correctly scoped to #1859 since it's dashboard logic. Nothing is missing from #1857 that would block #1859.

---

## Backward Coherence Check

**Previous issue #1856 summary:** "Files changed: cmd/tsuku/install.go, cmd/tsuku/install_test.go. Key decisions: Split the multi-case network error branch into individual cases per error type to return distinct subcategory strings."

**Assessment: No contradictions.** #1856 added the `subcategory` field to the CLI's `installError` struct and made `classifyInstallError()` return subcategory strings. #1857 consumes that field through `parseInstallJSON()` via the `installResult` struct. The two implementations are complementary: #1856 produces the data, #1857 extracts and passes it through. No conventions were changed or renamed between issues. The approach is consistent.

---

## Findings

### Blocking: 0

None.

### Advisory: 0

None.

---

## Overall Assessment

The implementation faithfully captures the design's intent across all dimensions. The canonical taxonomy matches the design doc table exactly. The separation of concerns -- orchestrator owns pipeline categories via `categoryFromExitCode()`, CLI owns subcategories via typed error classification -- is implemented as designed. The `parseInstallJSON()` change from trusting CLI category strings to always deriving from exit codes is the core behavioral fix, and it's correctly implemented. Both downstream issues (#1858, #1859) have what they need to proceed. The implementation is backward-coherent with #1856.
