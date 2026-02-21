# Scrutiny Review: Justification Focus -- Issue #1829

**Issue:** feat(registry): include satisfies data in registry manifest
**Scrutiny Focus:** justification
**Reviewer Date:** 2026-02-21

## Review Methodology

Read the diff (`git diff HEAD~1`) before examining the requirements mapping. Formed an independent understanding of the implementation by reading all changed files, then evaluated the mapping's deviation explanations from the justification lens.

## Diff Summary

Files changed: `scripts/generate-registry.py`, `internal/recipe/loader.go`, `internal/recipe/satisfies_test.go`, `internal/registry/manifest.go` (new), `internal/registry/manifest_test.go` (new), `recipes/l/libcurl.toml`.

The implementation:
1. Bumps schema version to 1.2.0.
2. Adds `satisfies` field emission in `parse_recipe()`, omitting when absent.
3. Adds `validate_metadata` logic for the `satisfies` structure (ecosystem pattern, package name pattern validation).
4. Adds cross-recipe duplicate detection and canonical name collision detection in `main()`.
5. Creates `internal/registry` package with `Manifest`/`ManifestRecipe` types, `FetchManifest()`, `GetCachedManifest()`, `parseManifest()`.
6. Updates `buildSatisfiesIndex()` in `loader.go` to read cached manifest data (embedded recipes take priority).
7. Adds comprehensive tests (8 manifest tests, 4 satisfies integration tests).
8. **Removes** `[metadata.satisfies] homebrew = ["curl"]` from `recipes/l/libcurl.toml`.

## Findings

### Finding 1: Undisclosed data change -- libcurl.toml satisfies removal (BLOCKING)

**Observation**: The diff removes `[metadata.satisfies] homebrew = ["curl"]` from `recipes/l/libcurl.toml`. This change appears nowhere in the requirements mapping. The mapping contains 4 "implemented" entries and zero deviations.

**Analysis**: The removal is a direct consequence of the new canonical name collision validation added in this commit. The `generate-registry.py` script now checks `if pkg_name in recipe_names and pkg_name != recipe["name"]` -- so `libcurl` declaring `satisfies.homebrew = ["curl"]` would fail because `curl` is a canonical recipe name (`recipes/c/curl.toml` exists).

The `satisfies.homebrew = ["curl"]` entry was added in #1828 as part of the `dep-mapping.json` migration. Removing it means `libcurl` can no longer be discovered through a satisfies lookup for the name "curl." This is a material change to the ecosystem name resolution system.

The coder likely has a valid rationale: `curl` (the CLI tool) and `libcurl` (the library) are genuinely different recipes, and having `libcurl` claim to satisfy "curl" could misdirect users who want the CLI tool. The canonical name collision rule is correct here. But this reasoning should have been stated as a deviation in the mapping, addressing:
- Was the #1828 migration wrong to add it?
- Is the collision rule correctly surfacing a pre-existing data quality issue?
- Are there downstream effects from this removal?

The avoidance pattern concern: the change resolves a validation conflict by silently removing data rather than explaining the conflict. This is the pattern the justification lens is specifically designed to catch -- not because the decision is wrong, but because the mapping presents zero trade-offs when at least one meaningful trade-off was made.

**Severity**: Blocking. The change is undisclosed, modifies recipe data outside the stated issue scope, and has consequences for name resolution behavior. The fix may be correct, but the mapping must acknowledge and justify it.

### Finding 2: No deviations declared despite 5 unmapped ACs (ADVISORY)

**Observation**: The mapping has 4 entries for 9 ACs. Five ACs are not explicitly mapped:
1. "Recipes without satisfies omit the field" -- implemented via `parse_recipe()` truthy check and `omitempty` JSON tag
2. "validate_metadata validates satisfies structure" -- implemented with full validation block (ecosystem pattern, array type checks, name pattern)
3. "Canonical name collision flagged" -- implemented in `main()` cross-recipe validation
4. "Registry-only recipes resolvable" -- verified by `TestSatisfies_ManifestRecipeResolvable`
5. "Existing recipes without satisfies continue working" -- verified by `TestSatisfies_BuildIndex_NoManifest` and `TestManifestRecipe_SatisfiesOmittedWhenEmpty`

**Analysis**: All 5 ACs are implemented in the diff. The mapping is compressed rather than incomplete. This is a documentation gap, not a code gap. No deviations exist for these ACs, so there are no justifications to evaluate.

**Severity**: Advisory. The code is there; the mapping just doesn't enumerate each AC individually.

### Finding 3: Proportionality -- no concerns

The implementation scope is proportionate to the issue. A new `internal/registry` package was created (reasonable separation of concerns), test coverage is thorough (12 new test functions), and all 9 ACs are addressed in the diff. No avoidance patterns like "too complex" or "out of scope" were found. The zero-deviation presentation is accurate for 8 of 9 ACs; the exception is the libcurl.toml change covered in Finding 1.

## Overall Assessment

One blocking finding: the removal of `libcurl.toml`'s `satisfies` field is undisclosed in the mapping. The canonical name collision rule correctly identifies a conflict, and removing the entry may be the right resolution, but the mapping must acknowledge this trade-off and explain why it was necessary. This is a justification gap, not necessarily a code quality gap.

One advisory finding: the mapping is compressed (4 entries for 9 ACs) but all ACs are implemented.
