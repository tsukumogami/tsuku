# Scrutiny Review: Intent -- Issue #1856

**Issue**: #1856 feat(cli): add subcategory to install error JSON output
**Focus**: intent (design intent alignment + cross-issue enablement)
**Reviewer assessment date**: 2026-02-21

## Sub-check 1: Design Intent Alignment

### Design doc description of Phase 1

The design doc (DESIGN-structured-error-subcategories.md) describes Phase 1 as:

> Modify `classifyInstallError()` to return `(exitCode int, subcategory string)`. Update `handleInstallError()` to include the subcategory in the `installError` JSON struct. Add tests for the new subcategory values. The CLI's `categoryFromExitCode()` is unchanged -- it continues producing user-facing category names.

The design doc's Solution Architecture section specifies:

- `classifyInstallError()` returns subcategory alongside exit code
- `ErrTypeTimeout` -> `"timeout"`, `ErrTypeDNS` -> `"dns_error"`, `ErrTypeTLS` -> `"tls_error"`, `ErrTypeConnection` -> `"connection_error"`
- `ErrTypeNotFound`, `ErrTypeNetwork` (generic), dependency wrapper, catch-all -> `""` (empty)
- `installError` struct gains `Subcategory string` with `json:"subcategory,omitempty"`
- `categoryFromExitCode()` is unchanged
- Human-readable stderr output is unchanged
- Exit codes are unchanged

### Implementation vs. design intent

**Subcategory mapping**: The implementation at `cmd/tsuku/install.go:302-326` matches the design doc's subcategory taxonomy table exactly. Each error type maps to the correct subcategory string. The previously collapsed multi-case branch for network error subtypes was split into individual cases, which is the correct approach to return distinct subcategory strings. No deviation.

**Struct update**: `installError` at line 330-337 has the `Subcategory` field with the exact tag `json:"subcategory,omitempty"` as specified. No deviation.

**handleInstallError**: At line 367-383, `code, subcategory := classifyInstallError(err)` and `Subcategory: subcategory` in the struct literal. The subcategory flows into JSON output only in the `--json` path. The non-JSON path (stderr) is unchanged. No deviation.

**categoryFromExitCode**: Lines 349-360 are unchanged in the diff. Confirmed.

**Exit codes**: Not modified. Confirmed by examining the diff -- only subcategory-related changes are present.

**Assessment**: The implementation fully captures the design doc's intent for Phase 1. The function signature, subcategory values, struct field, JSON tag, and omitempty behavior all match the described architecture.

## Sub-check 2: Cross-Issue Enablement

### Downstream issue: #1857

Issue #1857 (feat(batch): normalize pipeline categories and add subcategory passthrough) depends on this issue and needs:

1. **`classifyInstallError()` returns subcategory string**: Confirmed. The function now returns `(int, string)` at line 302. The subcategory is a string with values matching the design's taxonomy.

2. **`installError` struct includes Subcategory in JSON output**: Confirmed. The `Subcategory` field at line 333 has `json:"subcategory,omitempty"`, so when non-empty, it appears as `"subcategory":"timeout"` in the marshaled JSON that `parseInstallJSON()` will parse.

3. **Subcategory values documented and tested**: Confirmed.
   - The subcategory mapping table is documented in the issue body and matches the design doc.
   - `TestClassifyInstallError` tests all error types with expected subcategory values: `"timeout"`, `"dns_error"`, `"tls_error"`, `"connection_error"`, and `""` for cases that don't produce subcategories.
   - `TestInstallErrorJSON` has two subtests: "without subcategory" verifies `omitempty` omits the field, "with subcategory" verifies the field appears with value `"timeout"`.

4. **Key AC from #1857: `parseInstallJSON()` extracts subcategory from CLI JSON**: This requires the CLI to emit `"subcategory"` in its JSON output, which this implementation enables. The `installError` struct serialization is the contract that #1857's `parseInstallJSON()` will deserialize. The field name, JSON key, and omitempty behavior are all correct for downstream consumption.

5. **Key AC from #1857: `installResult` struct includes Subcategory field for deserialization**: This is #1857's own work, but it depends on the CLI producing the field. This implementation produces it correctly.

**Assessment**: The foundation provided by this implementation is sufficient for #1857. The subcategory values are a closed, documented set. The JSON serialization format is standard Go `json:"subcategory,omitempty"` which deserializes naturally on the other side. No fields are missing, no values are absent, and no interface is too thin for what #1857 needs.

## Backward Coherence

This is the first issue in the sequence (no previous summary). Skipped.

## Findings

### Blocking: None

### Advisory: None

The implementation is a clean, direct mapping of the design doc's Phase 1 specification. The code changes are minimal and focused. The subcategory values match the taxonomy tables exactly. The test coverage is thorough -- every error type path is tested for both exit code and subcategory. The `omitempty` behavior is explicitly tested in both directions (field present when set, field absent when empty). The downstream issue has a solid foundation to build on.
