# Maintainer Review: Issue 1 -- Integer Schema Version Validation

## Files Changed

- `internal/registry/errors.go`
- `internal/registry/manifest.go`
- `internal/registry/manifest_test.go`
- `internal/recipe/satisfies_test.go`

## Findings

### 1. Divergent remediation text in error message and Suggestion() (Advisory)

**File:** `internal/registry/manifest.go:176-178` and `internal/registry/errors.go:90-91`

The `parseManifest()` error message embeds remediation inline:
```
"unsupported manifest schema version %d (supported range: %d-%d); run 'tsuku update-registry' or upgrade tsuku"
```

The `Suggestion()` method for `ErrTypeSchemaVersion` returns different wording:
```
"Run 'tsuku update-registry' to refresh the cache, or upgrade tsuku to the latest version"
```

The `errmsg.FormatError()` function appends `Suggestion()` output after the error message. Users will see both versions of the same advice in one output:

```
Error: registry: unsupported manifest schema version 2 (supported range: 1-1); run 'tsuku update-registry' or upgrade tsuku

Suggestion: Run 'tsuku update-registry' to refresh the cache, or upgrade tsuku to the latest version
```

The next developer will wonder if these are meant to say different things. They aren't -- it's the same advice written twice. The fix is to remove the inline suggestion from the error message in `parseManifest()` and let `Suggestion()` handle it. The error message should only state the problem: `"unsupported manifest schema version %d (supported range: %d-%d)"`.

This pattern is consistent with how other error types work in this file -- none of the other `Suggestion()` cases duplicate their text in the error message itself.

**Severity:** Advisory. The duplication is confusing but won't cause a bug. A future developer changing the suggestion text would need to update two locations, and might only find one.

### 2. Test names accurately describe behavior (No issue)

`TestParseManifest_ValidIntegerVersion`, `TestParseManifest_AboveMaxVersion`, `TestParseManifest_ZeroVersion`, and `TestParseManifest_SchemaVersionSuggestion` all test exactly what their names say. The test assertions match.

### 3. Constants are well-placed and named (No issue)

`MinManifestSchemaVersion` and `MaxManifestSchemaVersion` are defined next to the struct they validate, with clear godoc. The range check in `parseManifest()` references the constants rather than magic numbers.

### 4. Single validation point is clear (No issue)

`parseManifest()` is the only place schema version validation happens, and both `FetchManifest()` and `GetCachedManifest()` route through it. The next developer won't accidentally bypass validation.

### 5. Existing test updates in satisfies_test.go (No issue)

The `schema_version` values in `satisfies_test.go` manifest fixtures were updated from string `"1.0.0"` to integer `1`. These are consistent with the new type and pass validation.

## Summary

The implementation is clean and well-structured. The single validation point in `parseManifest()`, the named constants, and the typed error with `ErrTypeSchemaVersion` all make the code easy to understand and modify. One advisory finding: the remediation advice appears in both the error message string and the `Suggestion()` method with different wording -- the next developer who updates one might miss the other. Removing the inline suggestion from the error message would align with how other error types in the same file work.
