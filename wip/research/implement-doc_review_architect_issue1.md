# Architect Review: Issue 1 - Integer Schema Version Validation

## Review Scope

Files reviewed (based on Issue 1 acceptance criteria):
- `internal/registry/manifest.go` -- struct change, constants, validation logic
- `internal/registry/errors.go` -- new `ErrTypeSchemaVersion` error type
- `internal/registry/manifest_test.go` -- new and updated tests

## Findings

### No blocking findings.

### Advisory: Validation approach diverges slightly from `internal/discover/registry.go`

**File:** `internal/registry/manifest.go:171-181` vs `internal/discover/registry.go:52-53`

The manifest validation uses a `[Min, Max]` range check with `RegistryError` and `ErrTypeSchemaVersion`, while `discover/registry.go:52` uses a simple equality check (`!= 1`) returning a plain `fmt.Errorf`. Both are schema version validation for JSON files consumed by the CLI.

The manifest approach is strictly better (range-based, typed error, actionable message), but it creates a divergence where discovery registry validation will likely need to be upgraded to match. This isn't blocking because: (1) they're in different packages with different consumers, (2) the manifest pattern is the one the design doc establishes as canonical, and (3) upgrading discovery later won't require touching manifest code.

### Positive observations

1. **Consistent with existing patterns.** The `SchemaVersion int` type matches every other internal package that serializes schema versions (`dashboard`, `discover`, `blocker`, `batch`, `seed`, `markfailures`). The telemetry package uses `string` for its own `SchemaVersion`, but that's a distinct domain (event schema vs registry schema) -- no conflict.

2. **Error type extends existing registry.** `ErrTypeSchemaVersion` is added to the existing `ErrorType` iota in `errors.go` with a corresponding `Suggestion()` case. This follows the established pattern exactly -- no parallel error mechanism introduced.

3. **Validation is in the right place.** `parseManifest()` is the single entry point for all manifest parsing (both `GetCachedManifest` and `FetchManifest` route through it). The version check runs after unmarshal but before returning, which means no caller can obtain an unsupported-version manifest. No dispatch bypass.

4. **No dependency direction issues.** The changes are entirely within `internal/registry/` with no new imports. The package remains at the same architectural layer.

5. **Constants are well-placed for Issue 2.** `MinManifestSchemaVersion` and `MaxManifestSchemaVersion` as package-level constants in `manifest.go` will be directly accessible to the deprecation logic (Issue 2) without needing to move or re-export them.
