# Scrutiny Review: Intent -- Issue #1829

**Focus**: intent (design alignment + cross-issue enablement)
**Issue**: #1829 (feat(registry): include satisfies data in registry manifest)

---

## Sub-check 1: Design Intent Alignment

### Design Phase 4 Description

The design document (DESIGN-ecosystem-name-resolution.md, "Phase 4: Registry Integration") says:

> 1. Update `scripts/generate-registry.py` to include `satisfies` data in the registry manifest
> 2. Update the loader's `fetchFromRegistry` path to populate the satisfies index from manifest data
> 3. Update recipe validation to warn on duplicate `satisfies` entries across recipes

The issue's ACs elaborate these into specific criteria around schema versioning, validation structure, cross-recipe duplicate detection, canonical name collision flagging, loader index population from manifest, and registry-only recipe resolvability.

### Assessment: Registry Generation

Fully aligned. `generate-registry.py` extracts `satisfies` from TOML metadata and includes it in JSON output. Schema version bumped from 1.1.0 to 1.2.0. Recipes without satisfies omit the field (`if satisfies:` guard ensures only truthy values are included). This matches the design's intent that the manifest carries satisfies data so the loader doesn't need to download every recipe.

### Assessment: Loader Integration

Fully aligned. `buildSatisfiesIndex()` has a new loop reading `manifest.Recipes` from the cached manifest via `l.registry.GetCachedManifest()`. The design says "scan all embedded recipes and (if available) the registry manifest" -- that's what the implementation does. Priority is correct: embedded entries scanned first, manifest entries skipped if the package name is already indexed. The design's Solution Architecture says the index is "keyed by bare package name (not prefixed by ecosystem), because callers don't know which ecosystem a dependency comes from" -- matched.

The implementation also creates `internal/registry/manifest.go` with `Manifest` and `ManifestRecipe` types, `FetchManifest()`, and `GetCachedManifest()`. The design doc listed the manifest format under Uncertainties ("the exact format of this inclusion hasn't been designed"), so these types are new design decisions. They follow Go conventions and model the JSON schema correctly with `json:"satisfies,omitempty"`.

Note: The design says "Update the loader's `fetchFromRegistry` path" but the implementation updates `buildSatisfiesIndex()` instead. This is a better integration point since `buildSatisfiesIndex` is where the index is built. The design's phrasing was likely imprecise about the exact code location. The intent -- populate the index from manifest data -- is satisfied.

### Assessment: Cross-recipe Validation

Fully aligned. The generate script validates two kinds of collisions:
1. Duplicate satisfies entries across recipes (same `pkg_name` claimed by two different recipes)
2. Canonical name collision (a satisfies entry matching another recipe's canonical name)

The design Phase 4 says "warn on duplicate satisfies entries." The implementation makes them hard errors (script exits non-zero). The issue's AC explicitly says "the script exits with error," which is stricter than the design's "warn." This is the correct behavior for CI -- the design's Uncertainties section says "Recipe validation (CI) should hard-error on duplicate satisfies entries to catch the problem at PR time."

### Assessment: Satisfies Structure Validation

The `validate_metadata` function validates satisfies entries: keys must match `ECOSYSTEM_PATTERN` (lowercase alphanumeric with hyphens), values must be arrays of strings matching `NAME_PATTERN`. This is well-aligned with the issue's AC4 and the design's Phase 1 note about field validation.

### Finding 1 (advisory): Removal of satisfies from libcurl.toml contradicts #1828 and design intent

The diff removes `[metadata.satisfies]` with `homebrew = ["curl"]` from `recipes/l/libcurl.toml`. Issue #1828 explicitly migrated this mapping from `dep-mapping.json` to `libcurl.toml`'s `[metadata.satisfies]` section. The #1828 commit message says: "Migrate non-trivial entries from dep-mapping.json to per-recipe satisfies fields: gcc -> gcc-libs, python-standalone, sqlite3 -> sqlite, curl -> libcurl, nghttp2 -> libnghttp2."

`libcurl` is a registry-only recipe (no file exists at `internal/recipe/recipes/libcurl.toml`). With `satisfies` removed from the TOML, the `curl -> libcurl` mapping depends entirely on the manifest being cached locally. The design doc says the index is built from "embedded recipes and (if available) the registry manifest." The manifest is explicitly optional ("if available"), meaning a missing manifest silently drops the mapping.

The design's core rationale is "co-locating the mapping with the recipe means no separate file to maintain and no indirection to forget." Removing satisfies from the recipe TOML and relying solely on manifest derivation inverts this relationship -- the mapping now lives only in the derived artifact, not the source file.

This is advisory rather than blocking because: (1) the generate script reads the TOML files to produce the manifest, so for CI validation the data source is still the TOML -- but the TOML no longer contains satisfies data; (2) the scenario where this matters is narrow (fresh install without cached manifest trying to resolve `curl` to `libcurl`); (3) the removal doesn't violate any explicit AC of #1829. However, it undoes #1828's migration work without explanation.

### Finding 2 (no issue): New registry/manifest.go is reasonable scope growth

Creating `internal/registry/manifest.go` with `FetchManifest`, `GetCachedManifest`, and manifest types goes beyond the minimum ACs but provides necessary plumbing for the loader to access manifest data. The `Registry` struct already handles recipe fetching; adding manifest fetching is consistent.

---

## Sub-check 2: Cross-Issue Enablement

**Downstream issues**: none (terminal issue). Sub-check skipped.

---

## Backward Coherence

**Previous summary**: "Files changed: scripts/generate-registry.py, internal/recipe/loader.go, internal/recipe/satisfies_test.go, internal/registry/manifest.go, internal/registry/manifest_test.go, recipes/l/libcurl.toml. Key decisions: New internal/registry package for manifest fetching/parsing. Schema version bumped to 1.2.0 in generate-registry.py with satisfies data and cross-recipe duplicate detection. Loader buildSatisfiesIndex reads registry manifest when available."

The previous issues established these conventions:
- #1826: `satisfiesIndex`, `buildSatisfiesIndex()`, and `satisfiesOnce` patterns. This issue extends them correctly.
- #1828: `satisfies` fields migrated from dep-mapping.json into recipe TOML files, including libcurl.toml. This issue removes the field from libcurl.toml, contradicting #1828's migration.

### Finding 3 (advisory): libcurl.toml change contradicts #1828 backward coherence

Issue #1828 established a pattern: satisfies data lives in recipe TOML files, co-located with the recipe. This issue removes satisfies from a recipe TOML without adding it to an embedded recipe equivalent. The mapping `curl -> libcurl` now only exists in the derived manifest file. This is a convention change from what #1828 established.

---

## Findings Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | Removal of satisfies from libcurl.toml contradicts #1828 migration and design's co-location rationale | Advisory |
| 2 | New registry/manifest.go reasonable scope growth | No issue |
| 3 | libcurl.toml change contradicts #1828 backward coherence | Advisory |
