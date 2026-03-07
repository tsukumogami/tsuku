# Pragmatic Review: Issue 2 - Deprecation Notice Parsing and Warning Display

## Summary

No blocking findings. The implementation is the simplest correct approach for the requirements.

## Advisory Findings

### 1. `resetDeprecationWarning()` exists solely for tests

**File**: `cmd/tsuku/helpers.go:72`
**Severity**: Advisory

`resetDeprecationWarning()` resets the `sync.Once` for testing. This is a common Go pattern for testing `sync.Once` behavior and is bounded in scope. No action needed unless the team prefers a different testing approach (e.g., passing the `sync.Once` as a parameter).

### 2. `sync.Once` has only one call site

**File**: `cmd/tsuku/helpers.go:69`
**Severity**: Advisory

`checkDeprecationWarning` is called from exactly one place (`refreshManifest` in `update_registry.go`), making the `sync.Once` dedup currently unnecessary. However, the AC explicitly requires it ("Warning fires at most once per CLI invocation via `sync.Once`"), and additional call sites are a reasonable expectation as more commands fetch manifests. Not blocking.

## Verification

- `DeprecationNotice` struct fields match the AC (SunsetDate, MinCLIVersion, Message, UpgradeURL).
- Pointer field on `Manifest` -- nil when absent, populated when present. Correct.
- `printWarning()` writes to stderr, respects `--quiet`. Verified in tests.
- `formatDeprecationWarning()` extracted as a pure function with `cliVersion` parameter for testability. Clean.
- Dev build detection covers `dev`, `dev-*`, `unknown`. Verified in `TestIsDevBuild`.
- Version comparison uses existing `version.CompareVersions()`. No new dependencies.
- Downgrade prevention: when CLI >= min, shows "already supports" instead of upgrade instruction. Tested.
- `upgrade_url` is displayed as text in the warning string, never auto-opened. Correct.
- Warning format matches AC: `Warning: Registry at <url> reports: <message>`.
- Test coverage: 20 tests covering parsing, nil cases, quiet suppression, sync.Once dedup, version comparison branches, dev builds, downgrade prevention.

## Conclusion

Implementation is correct, minimal, and well-tested. No over-engineering beyond the AC requirements.
