# Architect Review: Issue #1856

**Issue**: #1856 feat(cli): add subcategory to install error JSON output
**Focus**: architecture (design patterns, separation of concerns)
**Files changed**: `cmd/tsuku/install.go`, `cmd/tsuku/install_test.go`

## Architecture Assessment

### Design Alignment

The implementation follows the design doc's Phase 1 architecture precisely. The key structural decision -- that the CLI classifies subcategories using `errors.As()` typed error information while leaving pipeline category ownership to the orchestrator -- is correctly implemented. `classifyInstallError()` returns `(exitCode, subcategory)` and `categoryFromExitCode()` is unchanged, maintaining the separation of concerns between user-facing categories and pipeline categories.

### Pattern Consistency

**Error classification flow**: The implementation extends the existing `classifyInstallError()` function rather than introducing a parallel classification mechanism. The subcategory is derived from the same `errors.As()` + switch pattern already in use for exit code classification. The switch cases that previously grouped multiple network error types into a single `ExitNetwork` return are now split to return distinct subcategory strings. This is a natural extension, not a pattern break.

**Dual `categoryFromExitCode` functions**: The CLI's `categoryFromExitCode()` at `cmd/tsuku/install.go:349-360` is unchanged. The orchestrator's `categoryFromExitCode()` at `internal/batch/orchestrator.go:483` is unchanged. The intentional divergence between these two functions (documented in my memory and in both functions' comments) is respected. The new subcategory field flows through a different channel (the `installError` JSON struct) rather than trying to unify the category functions.

**JSON contract surface**: The `installError` struct gains one field with `omitempty`. The `installResult` struct in `internal/batch/orchestrator.go:18` (which the orchestrator uses to deserialize CLI JSON) does not yet include a `Subcategory` field -- that's #1857's work. Go's `json.Unmarshal` silently ignores unknown fields, so the CLI can start producing the field without breaking the orchestrator. This is correct forward compatibility.

### Dependency Direction

No new imports introduced. The change stays entirely within `cmd/tsuku/`, importing from `internal/registry` (lower level). Dependencies flow downward correctly.

### Extensibility for Downstream Issues

The `installResult` struct in the orchestrator (`internal/batch/orchestrator.go:18`) currently only has `Category` and `MissingRecipes`. Issue #1857 will add `Subcategory` to this struct and to `FailureRecord` (`internal/batch/results.go:130`). The CLI's change provides the right foundation: the subcategory is in the JSON output under a stable key (`"subcategory"`), using a closed set of hardcoded string values. The orchestrator can opt into reading it when ready.

### Potential Concern: `ErrTypeRateLimit` Gap

`registry.ErrTypeRateLimit` exists as a typed error (registry/errors.go:26) but is not handled in the `classifyInstallError()` switch. It falls through to the default case, returning `ExitInstallFailed, ""`. The design doc's subcategory taxonomy lists `rate_limited` as "heuristic" source rather than "CLI typed error," so this omission is consistent with the design. However, architecturally, if `ErrTypeRateLimit` is ever produced in an install path, it would be misclassified as `install_failed` rather than `network_error`. This is an advisory note for future work, not a blocking issue for this change.

## Findings

### Blocking: None

### Advisory: 1

1. **`cmd/tsuku/install.go:302-326` -- `ErrTypeRateLimit` not handled in switch**. The `classifyInstallError()` switch covers `ErrTypeNotFound`, `ErrTypeTimeout`, `ErrTypeDNS`, `ErrTypeTLS`, `ErrTypeConnection`, and `ErrTypeNetwork`, but not `ErrTypeRateLimit`. If a rate-limited error reaches this function, it falls to the default `ExitInstallFailed` instead of `ExitNetwork`. The design doc marks `rate_limited` as heuristic-only, so this is consistent with the current design intent. But it's worth noting that the typed error exists and could be handled here for completeness in a future issue. **Advisory.**

## Summary

The implementation fits the existing architecture cleanly. The subcategory is produced at the right layer (CLI, where typed error info is available), using the existing classification pattern (switch on `errors.As` result), and delivered through the existing JSON contract surface (`installError` struct with `omitempty`). No new patterns are introduced. The dual `categoryFromExitCode` functions remain untouched. Dependency direction is correct. The change provides the exact foundation that #1857 needs to add subcategory passthrough to the orchestrator.
