# Scrutiny Review: Completeness -- Issue #1856

**Issue**: #1856 (feat(cli): add subcategory to install error JSON output)
**Focus**: completeness
**Files changed**: `cmd/tsuku/install.go`, `cmd/tsuku/install_test.go`

## AC Coverage

The issue body defines five AC groups. The coder's mapping contains four entries covering three of them. Two AC groups have no explicit mapping entry but are satisfied in the implementation.

### AC Group 1: Signature change

**Issue AC**: `classifyInstallError()` returns `(exitCode int, subcategory string)` instead of `int`; all call sites updated.

**Mapping entry**: "classifyInstallError returns (int, string)" -- status: implemented

**Assessment**: CONFIRMED. Diff shows signature change at `classifyInstallError(err error) (int, string)` (install.go line 302). The sole call site in `handleInstallError()` (line 368) is updated to `code, subcategory := classifyInstallError(err)`.

### AC Group 2: Subcategory mapping

**Issue AC**: Eight error type mappings specified in a table (ErrTypeTimeout -> "timeout", ErrTypeDNS -> "dns_error", ErrTypeTLS -> "tls_error", ErrTypeConnection -> "connection_error", ErrTypeNotFound -> "", ErrTypeNetwork -> "", dependency wrapper -> "", catch-all -> "").

**Mapping entry**: No explicit entry for this AC group.

**Assessment**: IMPLEMENTED but not mapped. The diff shows all eight mappings correctly implemented:
- `ErrTypeNotFound` -> `""` (line 313)
- `ErrTypeTimeout` -> `"timeout"` (line 315)
- `ErrTypeDNS` -> `"dns_error"` (line 317)
- `ErrTypeTLS` -> `"tls_error"` (line 319)
- `ErrTypeConnection` -> `"connection_error"` (line 321)
- `ErrTypeNetwork` -> `""` (line 323)
- dependency wrapper -> `""` (line 307)
- catch-all -> `""` (line 326)

All eight are tested in `TestClassifyInstallError` with correct expected values.

**Severity**: advisory -- the code satisfies the AC; only the mapping documentation is incomplete.

### AC Group 3: Struct update

**Issue AC**: `installError` struct has `Subcategory string` with tag `json:"subcategory,omitempty"`; `handleInstallError()` populates the field.

**Mapping entries**: "Subcategory field added to installError struct" (implemented), "handleInstallError populates Subcategory" (implemented)

**Assessment**: CONFIRMED. Diff shows field added at line 333 with correct tag. `handleInstallError()` at line 373 sets `Subcategory: subcategory`.

### AC Group 4: JSON output behavior

**Issue AC**: Non-empty subcategory appears in JSON; empty subcategory omitted via omitempty; existing fields unchanged.

**Mapping entry**: "omitempty behavior correct" -- status: implemented

**Assessment**: CONFIRMED. `TestInstallErrorJSON` split into two subtests:
- "without subcategory": creates installError with no Subcategory, verifies key is absent from marshaled JSON
- "with subcategory": creates installError with Subcategory "timeout", verifies it appears as `"subcategory":"timeout"`

Existing fields (status, category, message, missing_recipes, exit_code) have unchanged tags and are tested in both subtests.

### AC Group 5: No changes to non-JSON output

**Issue AC**: `categoryFromExitCode()` unchanged; human-readable stderr unchanged; exit codes unchanged.

**Mapping entry**: No explicit entry for this AC group.

**Assessment**: CONFIRMED by diff. `categoryFromExitCode()` has no diff. The else branch in `handleInstallError()` (stderr path) is untouched. Exit code values are the same; only the return tuple changed.

**Severity**: advisory -- absence-of-change ACs are reasonable to omit from a mapping, but noting for completeness.

## Phantom AC Check

All four mapping entries correspond to real ACs in the issue body:
- "classifyInstallError returns (int, string)" -> AC Group 1
- "Subcategory field added to installError struct" -> AC Group 3
- "handleInstallError populates Subcategory" -> AC Group 3
- "omitempty behavior correct" -> AC Group 4

No phantom ACs detected.

## Evidence Verification

| Mapping claim | Cited evidence | Diff verification |
|---|---|---|
| classifyInstallError returns (int, string) | install.go:classifyInstallError() line 302 | Confirmed: signature changed at line 302 |
| Subcategory field added | install.go line 333 | Confirmed: `Subcategory string \`json:"subcategory,omitempty"\`` at line 333 |
| handleInstallError populates | install.go:handleInstallError() line 373 | Confirmed: `Subcategory: subcategory` at line 373 |
| omitempty behavior correct | install_test.go TestInstallErrorJSON subtests | Confirmed: two subtests verify presence/absence |

All cited file locations and function names exist in the diff and match the claimed behavior.

## Summary

The implementation fully satisfies all five AC groups from issue #1856. The mapping has two advisory gaps (AC groups 2 and 5 not explicitly listed) but no blocking issues. All "implemented" claims are verified against the diff. The subcategory values match the design doc's taxonomy table. The downstream dependency (#1857) has what it needs: the function returns a subcategory, the struct includes it in JSON, and the values are tested.
