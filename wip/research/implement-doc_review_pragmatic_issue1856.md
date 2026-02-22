# Pragmatic Review: Issue #1856

**Issue**: #1856 (feat(cli): add subcategory to install error JSON output)
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Files changed**: `cmd/tsuku/install.go`, `cmd/tsuku/install_test.go`

## Findings

### Advisory 1: `ErrTypeRateLimit` falls through to wrong exit code

`cmd/tsuku/install.go:302-326` -- `classifyInstallError()` handles `ErrTypeNotFound`, `ErrTypeTimeout`, `ErrTypeDNS`, `ErrTypeTLS`, `ErrTypeConnection`, and `ErrTypeNetwork`, but not `ErrTypeRateLimit`. A `RegistryError` with `ErrTypeRateLimit` (returned by `internal/registry/registry.go:169` on HTTP 429) falls through to the default case, producing exit code 6 (`install_failed`) instead of exit code 5 (`network_error`).

This is a pre-existing issue -- the old `classifyInstallError` also didn't handle it because the old code used a collapsed multi-case branch for `ErrTypeTimeout`, `ErrTypeDNS`, `ErrTypeTLS`, `ErrTypeConnection`, and `ErrTypeNetwork` (which didn't include `ErrTypeRateLimit` either). The design doc lists `rate_limited` as a heuristic-sourced subcategory, not CLI-sourced, so it was intentionally left out of the CLI subcategory table. However, the exit code misclassification (6 instead of 5) exists regardless of subcategory.

Not blocking because: (a) this is pre-existing behavior not introduced by this PR, and (b) the design doc explicitly delegates `rate_limited` to heuristic sourcing. Worth filing as a follow-up.

### No blocking findings

The implementation is the simplest correct approach for the stated requirements:

- `classifyInstallError()` already had the `errors.As()` switch. Adding a second return value and splitting the collapsed case into individual cases is the minimal change.
- No new abstractions, no new types, no new files. The `Subcategory` field is added inline to the existing struct.
- `handleInstallError()` wires the subcategory through in one line.
- Tests cover all eight error type mappings and the omitempty serialization behavior in both directions.
- `categoryFromExitCode()` is correctly left unchanged per design doc.

No speculative generality, no dead code, no scope creep.
