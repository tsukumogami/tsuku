# Pragmatic Review: Issue #1857

## Issue

feat(batch): normalize pipeline categories and add subcategory passthrough

## Review Focus

pragmatic (simplicity, YAGNI, KISS)

## Files Reviewed

- `internal/batch/orchestrator.go`
- `internal/batch/orchestrator_test.go`
- `internal/batch/results.go`
- `data/schemas/failure-record.schema.json`

## Findings

### Blocking: 0

None.

### Advisory: 0

None.

## Analysis

**Single-caller abstraction**: No new helper functions introduced. `parseInstallJSON` was already a function; it gained a return value. No wrappers, no new layers.

**Speculative generality**: The `Subcategory` field on `FailureRecord` and `installResult` serves the immediate purpose (passthrough from CLI to JSONL). No config options, no feature flags, no unused parameters.

**Impossible-case handling**: None added. The JSON unmarshal error path in `parseInstallJSON` is correct -- it falls back to exit-code-only classification, which is the right behavior for malformed stdout.

**Backwards-compatibility shims**: None. The old category strings are simply replaced. The schema enum is updated to the new set. No aliases, no dual-write.

**Scope creep**: The diff touches exactly the files the issue specified. The `generate()` retry-exhaustion fallback update (`"api_error"` -> `"network_error"`) and the `"install_failed"` condition update (was `"validation_failed"`) are mechanical consequences of the taxonomy change, not scope additions.

**Gold-plated validation**: None. The schema's `subcategory` field is an unconstrained string, not an enum -- correct given that subcategory values come from the CLI and will grow over time without requiring schema changes.

The implementation is the simplest correct approach. `parseInstallJSON` grew from 2 to 3 return values and 1 line of logic. `categoryFromExitCode` is a 6-case switch. `FailureRecord` added one field. No new types, interfaces, or abstractions.
