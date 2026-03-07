# Validation Report: Issue 1 (Registry Schema Versioning)

## Summary

All 5 scenarios passed.

---

## Scenario 1: Integer schema version parses correctly

**ID**: scenario-1
**Status**: PASSED

**Command**: `go test ./internal/registry/... -run TestParseManifest -v`

**Verification**:
- `TestParseManifest_ValidIntegerVersion` explicitly tests `"schema_version": 1` (integer) and asserts `manifest.SchemaVersion == 1`.
- `TestParseManifest_WithSatisfies` also uses integer `"schema_version": 1` and passes.
- `Manifest.SchemaVersion` is declared as `int` in `manifest.go` (line 34).
- No string-based schema version (`"1.2.0"`) remains in any source or test file.

**Output**: PASS (0.002s)

---

## Scenario 2: Above-range schema version returns RegistryError with ErrTypeSchemaVersion

**ID**: scenario-2
**Status**: PASSED

**Command**: `go test ./internal/registry/... -run TestParseManifest_AboveMaxVersion -v`

**Verification**:
- Test uses `MaxManifestSchemaVersion+1` (which is 2, since max is 1).
- Asserts error is `*RegistryError` via `errors.As`.
- Asserts `regErr.Type == ErrTypeSchemaVersion`.
- Asserts error message contains the version number, `"tsuku update-registry"`, and `"upgrade tsuku"`.
- The error message format is: `"unsupported manifest schema version 2 (supported range: 1-1); run 'tsuku update-registry' or upgrade tsuku"`.

**Output**: PASS (0.002s)

---

## Scenario 3: Zero schema version is handled

**ID**: scenario-3
**Status**: PASSED

**Command**: `go test ./internal/registry/... -run TestParseManifest_ZeroVersion -v`

**Verification**:
- Test uses `"schema_version": 0`.
- Asserts `parseManifest()` returns a `*RegistryError` with `Type == ErrTypeSchemaVersion`.
- The validation in `parseManifest()` checks `manifest.SchemaVersion < MinManifestSchemaVersion` (which is 1), so 0 is correctly rejected.
- A missing `schema_version` field defaults to Go's zero value for int (0), which would also be rejected by the same check.

**Output**: PASS (0.002s)

---

## Scenario 4: Incompatible manifest is not cached on fetch

**ID**: scenario-4
**Status**: PASSED

**Command**: `go test ./internal/registry/... -run TestFetchManifest -v` + additional targeted test

**Verification**:
- Created and ran a dedicated test (`TestFetchManifest_IncompatibleSchemaNotCached`) that:
  1. Serves a manifest with `"schema_version": 99` from a test HTTP server.
  2. Pre-populates a valid cached manifest on disk.
  3. Calls `FetchManifest()` and verifies it returns an error.
  4. Verifies the error is a `*RegistryError` with appropriate message.
  5. Reads the cache file and confirms the pre-existing valid manifest was NOT overwritten.
- The code path in `FetchManifest()` (manifest.go lines 131-134) calls `parseManifest(data)` before `CacheManifest(data)`, so an incompatible version causes early return with error before caching.
- Error message: `"registry: unsupported manifest schema version 99 (supported range: 1-1); run 'tsuku update-registry' or upgrade tsuku"`

**Output**: PASS (0.003s)

---

## Scenario 5: Satisfies tests pass with integer schema version

**ID**: scenario-5
**Status**: PASSED

**Command**: `go test ./internal/recipe/... -run TestSatisfies -v`

**Verification**:
- All 3 manifest JSON literals in `satisfies_test.go` use `"schema_version": 1` (integer):
  - Line 581: `TestSatisfies_BuildIndex_IncludesManifestData`
  - Line 641: `TestSatisfies_BuildIndex_EmbeddedOverManifest`
  - Line 720: `TestSatisfies_ManifestRecipeResolvable`
- No `"schema_version": "1.2.0"` string values remain anywhere in the codebase source or test files.
- All 24 satisfies-related tests pass.

**Output**: PASS (0.019s)
