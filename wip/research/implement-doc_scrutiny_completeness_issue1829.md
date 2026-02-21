# Scrutiny Review: Completeness -- Issue #1829

**Issue**: feat(registry): include satisfies data in registry manifest
**Focus**: completeness
**Date**: 2026-02-21

---

## AC Extraction from Issue Body

The issue body contains 9 acceptance criteria (checkboxes):

1. `scripts/generate-registry.py` includes a `satisfies` field in each recipe's JSON entry when the recipe has `[metadata.satisfies]` in its TOML
2. Recipes without `satisfies` entries omit the field (no empty objects in the JSON)
3. Schema version in the generated JSON is bumped to `1.2.0`
4. The `validate_metadata` function in the generate script validates `satisfies` structure: keys must be strings, values must be arrays of strings, package names must match the existing `NAME_PATTERN`
5. Cross-recipe duplicate detection: if two recipes declare the same package name in any `satisfies` entry, the script exits with error and lists the conflict
6. A `satisfies` entry that matches another recipe's canonical name is flagged as an error
7. The loader's `buildSatisfiesIndex()` populates the index from registry manifest data in addition to embedded recipes
8. Registry-only recipes with `satisfies` entries are resolvable through the satisfies fallback
9. Existing recipes without `satisfies` continue to work with no changes to their manifest entries

---

## AC-by-AC Verification

### AC 1: generate-registry.py includes satisfies field

**Mapping claim**: "implemented" -- evidence: "generate-registry.py satisfies field in recipe output"
**Diff verification**: `parse_recipe()` in `scripts/generate-registry.py` now reads `metadata.get("satisfies")` and includes it in the result dict when present (conditional `if satisfies: result["satisfies"] = satisfies`).
**Assessment**: PASS. Evidence confirmed in diff.

---

### AC 2: Recipes without satisfies omit the field

**Mapping claim**: No explicit entry in the requirements mapping.
**Diff verification**: Two mechanisms ensure omission:
1. Python: `if satisfies:` is falsy for `None` and `{}`, so absent/empty satisfies never enter the result dict.
2. Go: `ManifestRecipe.Satisfies` uses `json:"satisfies,omitempty"` tag.
3. Test `TestManifestRecipe_SatisfiesOmittedWhenEmpty` verifies JSON serialization omits the field when empty.

**Assessment**: PASS. Implemented but not mapped.
**Severity**: advisory (mapping omission for an implemented AC)

---

### AC 3: Schema version 1.2.0

**Mapping claim**: "implemented" -- evidence: "generate-registry.py SCHEMA_VERSION constant"
**Diff verification**: `SCHEMA_VERSION = "1.2.0"` replaces `"1.1.0"`.
**Assessment**: PASS. Evidence confirmed in diff.

---

### AC 4: validate_metadata validates satisfies structure

**Mapping claim**: No explicit entry in the requirements mapping.
**Diff verification**: ~30 lines added to `validate_metadata()`:
- `satisfies` must be a dict
- Ecosystem names must match new `ECOSYSTEM_PATTERN = re.compile(r"^[a-z][a-z0-9-]*$")`
- Values must be arrays of strings
- Package names must match existing `NAME_PATTERN`
**Assessment**: PASS. Implemented correctly.
**Severity**: advisory (mapping omission for an implemented AC)

---

### AC 5: Cross-recipe duplicate detection

**Mapping claim**: "implemented" -- evidence: "generate-registry.py validates no duplicate satisfies entries"
**Diff verification**: `main()` tracks `satisfies_claims: dict[str, str]` and emits `ValidationError` when `pkg_name in satisfies_claims` for a different recipe. The error message includes both conflicting recipe names and the ecosystem. `all_errors` causes exit code 1.
**Assessment**: PASS. Evidence confirmed in diff.

---

### AC 6: Canonical name collision flagged

**Mapping claim**: No explicit entry in the requirements mapping.
**Diff verification**: `if pkg_name in recipe_names and pkg_name != recipe["name"]` emits an error about conflicts with existing canonical names. The `!= recipe["name"]` guard permits tautological self-references while catching conflicts with other recipes.
**Assessment**: PASS. Implemented correctly. This validation also motivated the `libcurl.toml` data fix (see Additional Observations).
**Severity**: advisory (mapping omission for an implemented AC)

---

### AC 7: Loader buildSatisfiesIndex from manifest

**Mapping claim**: "implemented" -- evidence: "loader.go buildSatisfiesIndex registry integration"
**Diff verification**: The placeholder comment `// Registry manifest entries would be added here by #1829` is replaced with code that calls `l.registry.GetCachedManifest()` and iterates manifest entries, adding satisfies mappings where embedded entries don't already claim the package name.

The new `internal/registry/manifest.go` provides:
- `Manifest` and `ManifestRecipe` types for JSON parsing
- `GetCachedManifest()` for offline manifest access (reads from `CacheDir/manifest.json`)
- `FetchManifest()` for network retrieval with local caching

Tests `TestSatisfies_BuildIndex_IncludesManifestData`, `TestSatisfies_BuildIndex_EmbeddedOverManifest`, and `TestSatisfies_BuildIndex_NoManifest` verify the integration.
**Assessment**: PASS. Evidence confirmed in diff.

---

### AC 8: Registry-only recipes resolvable through fallback

**Mapping claim**: No explicit entry, partially covered by mapping entry 4 ("Loader reads manifest satisfies").
**Diff verification**: Test `TestSatisfies_ManifestRecipeResolvable` demonstrates end-to-end resolution: creates a local recipe (simulating registry-only) and a cached manifest with matching satisfies data, then verifies `GetWithContext("test-alias@2", ...)` returns the recipe through the fallback path.
**Assessment**: PASS. The mechanism is implemented and tested.
**Severity**: advisory (the mapping's entry 4 is a reasonable umbrella but the issue has a distinct AC about end-to-end resolvability)

---

### AC 9: Existing recipes without satisfies continue working

**Mapping claim**: No explicit entry in the requirements mapping.
**Diff verification**: Backward compatibility is preserved:
1. `parse_recipe()` only adds `satisfies` when present.
2. `ManifestRecipe` uses `omitempty`.
3. Go range over nil map is a no-op, so `buildSatisfiesIndex()` handles absent satisfies gracefully.
4. Test `TestParseManifest_WithSatisfies` includes recipe `serve` without satisfies and confirms `len(serve.Satisfies) == 0`.
**Assessment**: PASS. Implicit coverage.
**Severity**: advisory (mapping omission)

---

## Phantom AC Check

The requirements mapping contains 4 entries:
1. "Registry manifest includes satisfies data" -- maps to AC 1
2. "Schema version 1.2.0" -- maps to AC 3
3. "Cross-recipe duplicate detection" -- maps to AC 5
4. "Loader reads manifest satisfies" -- maps to AC 7 (and partially AC 8)

All 4 mapping entries correspond to real ACs from the issue body. **No phantom ACs detected.**

---

## Additional Observations

### libcurl.toml satisfies removal (justified data fix)

The diff removes `[metadata.satisfies]` with `homebrew = ["curl"]` from `recipes/l/libcurl.toml`. This was added by #1828 during the dep-mapping migration.

The removal is justified: `recipes/c/curl.toml` exists as a canonical recipe, so `libcurl` claiming `satisfies.homebrew = ["curl"]` triggers the new canonical name collision validation (AC 6). The commit message explicitly documents this: "Remove satisfies.homebrew = ['curl'] since curl is a canonical recipe name; the cross-recipe validation correctly catches this conflict introduced by the dep-mapping migration in #1828."

This is a necessary consequence of AC 6's implementation catching a pre-existing data error. The other registry recipes modified by #1828 (`sqlite.toml` with `sqlite3`, `libnghttp2.toml` with `nghttp2`) don't have this problem because `sqlite3` and `nghttp2` are not canonical recipe names.

**Severity**: Not a finding (correctly identified and fixed a conflict from a prior issue).

### wip/ file cleanup

Three `wip/` files from the exploration phase are deleted. Expected housekeeping per project conventions.

**Severity**: Not a finding.

---

## Summary

| # | AC | Mapped? | Implemented? | Severity |
|---|-----|---------|-------------|----------|
| 1 | generate-registry.py includes satisfies | Yes | Yes | -- |
| 2 | Omit field when absent | No | Yes | advisory |
| 3 | Schema version 1.2.0 | Yes | Yes | -- |
| 4 | validate_metadata validates satisfies | No | Yes | advisory |
| 5 | Cross-recipe duplicate detection | Yes | Yes | -- |
| 6 | Canonical name collision detection | No | Yes | advisory |
| 7 | Loader buildSatisfiesIndex from manifest | Yes | Yes | -- |
| 8 | Registry-only resolvable through fallback | Partial | Yes | advisory |
| 9 | Existing recipes without satisfies work | No | Yes | advisory |

**Blocking findings: 0**
**Advisory findings: 5** (all are mapping omissions for ACs that are correctly implemented)
**Phantom ACs: 0**
