# Architect Review: Issue #1829

**Issue**: feat(registry): include satisfies data in registry manifest
**Focus**: architecture (design patterns, separation of concerns)
**Date**: 2026-02-21

---

## Files Changed

- `scripts/generate-registry.py` -- satisfies emission, cross-recipe validation, schema version bump
- `scripts/test_generate_registry.py` -- Python tests for cross-recipe validation
- `internal/registry/manifest.go` (new) -- Manifest/ManifestRecipe types, FetchManifest, GetCachedManifest
- `internal/registry/manifest_test.go` (new) -- 8 test functions for manifest parsing and caching
- `internal/recipe/loader.go` -- buildSatisfiesIndex reads manifest data
- `internal/recipe/satisfies_test.go` -- 4 new integration tests for manifest path
- `recipes/l/libcurl.toml` -- removed satisfies.homebrew = ["curl"], added explanatory comment

---

## Finding 1: manifest.go fits the registry package correctly (no issue)

The new `manifest.go` is placed in `internal/registry/`, the same package that handles `FetchRecipe`, `GetCached`, `CacheRecipe`, and discovery entries. The manifest is another registry artifact (the aggregated JSON at `tsuku.dev/recipes.json`), so placing fetch/parse/cache logic on the existing `Registry` struct is the right home.

Key structural points:
- `FetchManifest` reuses `r.client` (`manifest.go:66`), the security-hardened HTTP client from `newRegistryHTTPClient()` in `registry.go:38-51`. No new HTTP client instantiated.
- `GetCachedManifest` follows the same cache-read pattern as `GetCached`: read from `CacheDir`, return nil/nil on miss. Consistent behavior.
- `manifest.go` imports only standard library packages -- no upward dependency from `internal/registry` to `internal/recipe` or `cmd/`.
- The `ManifestRecipe.Satisfies` field uses `map[string][]string` with `json:"satisfies,omitempty"`, matching the Go-side `MetadataSection.Satisfies` declaration in `internal/recipe/types.go:162`.

No parallel pattern introduced. No dependency direction violation.

## Finding 2: Loader reads manifest through registry abstraction (no issue)

`buildSatisfiesIndex()` at `loader.go:402` calls `l.registry.GetCachedManifest()` to access manifest data. The loader doesn't fetch or parse JSON itself -- it delegates to the registry package. This preserves the dependency direction: `internal/recipe` depends on `internal/registry` (downward), not the reverse.

The design doc said "Update the loader's `fetchFromRegistry` path" but the implementation updates `buildSatisfiesIndex` instead. This is architecturally better. `fetchFromRegistry` loads individual recipes by name; building the satisfies index requires scanning all recipes. Putting the scan in `buildSatisfiesIndex` keeps concerns separated.

Priority is correctly enforced: embedded recipes scanned first (lines 374-396), manifest entries only added when the package name isn't already indexed (line 408). This matches the existing tier ordering (embedded > registry).

## Finding 3: FetchManifest error handling diverges from FetchRecipe pattern (advisory)

`FetchRecipe` in `registry.go` returns typed `RegistryError` values with `ErrType*` classifications (not found, rate limit, network, validation, parsing). `FetchManifest` in `manifest.go` returns plain `fmt.Errorf` wrapped errors without type classification.

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/internal/registry/manifest.go:61-74`

This means callers can't distinguish manifest fetch failures by type (e.g., rate limit vs. not found vs. network). Currently the only caller is `buildSatisfiesIndex`, which treats all errors the same (silently skips manifest data). But if a future caller (e.g., `update-registry` command) wants to handle manifest errors differently, typed errors would need to be retrofitted.

**Severity**: Advisory. The single current caller doesn't need typed errors, and adding them later is a contained change within `manifest.go`. Doesn't compound -- no other code will copy this untyped pattern because `FetchRecipe` already models the typed pattern.

## Finding 4: Cross-recipe validation correctly separated between CI and runtime (no issue)

The design doc's Uncertainties section said: "Recipe validation (CI) should hard-error on duplicate satisfies entries to catch the problem at PR time. At runtime, the loader should warn and prefer the first match found."

Implementation matches:
- `generate-registry.py` lines 322-345: hard errors on (a) duplicate satisfies claims across recipes and (b) satisfies entries that collide with canonical recipe names. Runs in CI. Script exits non-zero.
- `loader.go` lines 388-391: runtime warning for duplicate embedded entries, first-match preference. No hard error. Manifest duplicates silently skipped since CI already caught them (line 412 comment).

This is the right separation of concerns. Expensive cross-recipe validation at CI time; cheap best-effort at runtime.

## Finding 5: Python test file duplicates validation logic (advisory)

`scripts/test_generate_registry.py:38-71` re-implements the cross-recipe validation from `generate-registry.py:322-345` rather than calling the production code. The `_run_cross_recipe_validation` method is a near-copy of the production logic.

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/scripts/test_generate_registry.py:38-71`

If the production validation changes (e.g., adding a new check, changing error messages), the test's copy won't reflect the change. The production validation in `main()` is tightly integrated with the file-parsing pipeline, which explains why extracting a testable function would require refactoring.

**Severity**: Advisory. The duplication is contained to a single test file with a single test class. No other code will copy this pattern. The test exercises the logic shape (duplicate detection, canonical name collision), which is useful even if the exact code path diverges later.

## Finding 6: Schema version bump and backward compatibility (no issue)

`SCHEMA_VERSION` bumped from `1.1.0` to `1.2.0` in `generate-registry.py:23`. The `satisfies` field is optional (`omitempty` in Go, conditional emission in Python), so old clients reading a new manifest will simply ignore the unknown field. Minor version bump is semantically correct.

No existing Go code reads `schema_version` to gate behavior (verified by grep). The field is informational.

## Finding 7: libcurl.toml change is a correct canonical name collision fix (no issue)

The removal of `[metadata.satisfies] homebrew = ["curl"]` from `recipes/l/libcurl.toml` was added by #1828 during dep-mapping migration. The new canonical name collision validation (AC 6 of this issue) correctly flags it: `curl` exists as its own canonical recipe at `recipes/c/curl.toml`, so `libcurl` claiming to satisfy `curl` would create ambiguity.

The comment left in `libcurl.toml` (lines 7-9) documents why the field was removed. The design doc's Uncertainties section explicitly described this scenario: "A satisfies entry that matches another recipe's canonical name should be rejected by validation."

This is a data correction, not a structural concern.

---

## Overall Assessment

The implementation fits the existing architecture cleanly. The new `manifest.go` extends `internal/registry` following established patterns (reuses HTTP client, same cache conventions, correct dependency direction). The loader consumes manifest data through the registry abstraction. CI-time validation is properly separated from runtime behavior. No parallel patterns introduced, no dependency direction violations, no action dispatch bypass.

Two advisory findings: (1) `FetchManifest` uses plain errors instead of the typed `RegistryError` used by `FetchRecipe` -- contained, doesn't compound; (2) Python test duplicates validation logic -- contained to one test file.

No blocking findings.

---

## Summary Table

| # | Finding | Severity |
|---|---------|----------|
| 1 | manifest.go fits registry package correctly | No issue |
| 2 | Loader reads manifest through registry abstraction | No issue |
| 3 | FetchManifest uses plain errors instead of typed RegistryError | Advisory |
| 4 | Cross-recipe validation correctly separated between CI and runtime | No issue |
| 5 | Python test duplicates validation logic | Advisory |
| 6 | Schema version bump and backward compatibility | No issue |
| 7 | libcurl.toml canonical name collision fix | No issue |
